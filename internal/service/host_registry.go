package service

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultHostScanInterval   = 10 * time.Second
	defaultHostScanPort       = 7000
	defaultHostScanSourceFPS  = 7
	defaultHostScanTimeout    = 2 * time.Second
	defaultHostScanWorkerSize = 32
	defaultHostScanUser       = "admin"
	defaultHostScanPW         = "000000"
)

type HostRegistrySnapshot struct {
	LocalIP      string            `json:"localIp"`
	SubnetPrefix string            `json:"subnetPrefix"`
	ScannedAt    string            `json:"scannedAt"`
	Hosts        map[string]string `json:"hosts"`
}

type hostRegistry struct {
	mu           sync.RWMutex
	localIP      string
	subnetPrefix string
	scannedAt    time.Time
	hosts        map[string]string
}

var globalHostRegistry = &hostRegistry{
	hosts: make(map[string]string),
}

var globalHostScanConfig = &hostScanConfigStore{
	cfg: loadHostScanConfig(),
}

var globalHostScanScheduler = &hostScanSchedulerStore{}

func ResolveTargetAddress(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return trimmed
	}
	if net.ParseIP(trimmed) != nil {
		return trimmed
	}
	return globalHostRegistry.LookupIPByHostName(trimmed)
}

func GetHostRegistrySnapshot() HostRegistrySnapshot {
	return globalHostRegistry.Snapshot()
}

func StartHostScanScheduler(parent context.Context) {
	globalHostScanScheduler.mu.Lock()
	globalHostScanScheduler.parent = parent
	globalHostScanScheduler.mu.Unlock()
	fmt.Println("[host-scan] scheduler initialized (default: off)")
}

type HostScanSchedulerStatus struct {
	Enabled bool `json:"enabled"`
}

func GetHostScanSchedulerStatus() HostScanSchedulerStatus {
	globalHostScanScheduler.mu.RLock()
	defer globalHostScanScheduler.mu.RUnlock()
	return HostScanSchedulerStatus{
		Enabled: globalHostScanScheduler.running,
	}
}

func SetHostScanSchedulerEnabled(enabled bool) HostScanSchedulerStatus {
	if enabled {
		startHostScanSchedulerLoop()
	} else {
		stopHostScanSchedulerLoop()
	}
	return GetHostScanSchedulerStatus()
}

func startHostScanSchedulerLoop() {
	globalHostScanScheduler.mu.Lock()
	if globalHostScanScheduler.running {
		globalHostScanScheduler.mu.Unlock()
		return
	}
	parent := globalHostScanScheduler.parent
	if parent == nil {
		parent = context.Background()
		globalHostScanScheduler.parent = parent
	}
	runCtx, cancel := context.WithCancel(parent)
	globalHostScanScheduler.nextRunID++
	runID := globalHostScanScheduler.nextRunID
	globalHostScanScheduler.running = true
	globalHostScanScheduler.cancel = cancel
	globalHostScanScheduler.runID = runID
	globalHostScanScheduler.mu.Unlock()

	cfg := getHostScanConfig()
	if cfg.Interval <= 0 {
		cfg.Interval = defaultHostScanInterval
	}

	go func() {
		runHostScanOnce(runCtx, getHostScanConfig())
		ticker := time.NewTicker(cfg.Interval)
		defer ticker.Stop()
		defer markHostScanSchedulerStopped(runID)

		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				runHostScanOnce(runCtx, getHostScanConfig())
			}
		}
	}()
}

func stopHostScanSchedulerLoop() {
	globalHostScanScheduler.mu.Lock()
	cancel := globalHostScanScheduler.cancel
	globalHostScanScheduler.cancel = nil
	globalHostScanScheduler.running = false
	globalHostScanScheduler.runID = 0
	globalHostScanScheduler.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func markHostScanSchedulerStopped(runID int64) {
	globalHostScanScheduler.mu.Lock()
	defer globalHostScanScheduler.mu.Unlock()
	if globalHostScanScheduler.runID == runID {
		globalHostScanScheduler.cancel = nil
		globalHostScanScheduler.running = false
		globalHostScanScheduler.runID = 0
	}
}

type hostScanConfig struct {
	Port      int
	SourceFPS int
	User      string
	PW        string
	Timeout   time.Duration
	Interval  time.Duration
	Workers   int
	LocalIP   string
}

type hostScanConfigStore struct {
	mu  sync.RWMutex
	cfg hostScanConfig
}

type hostScanSchedulerStore struct {
	mu        sync.RWMutex
	parent    context.Context
	cancel    context.CancelFunc
	running   bool
	runID     int64
	nextRunID int64
}

type HostScanCGIConfig struct {
	Port      int    `json:"port"`
	SourceFPS int    `json:"sourceFps"`
	User      string `json:"user"`
	PW        string `json:"pw"`
}

type DeviceCGIConfig struct {
	DevicePort   int    `json:"devicePort"`
	DeviceUserID string `json:"deviceUserId"`
	DeviceUserPW string `json:"deviceUserPw"`
	SourceFPS    int    `json:"sourceFps"`
}

func GetHostScanCGIConfig() HostScanCGIConfig {
	cfg := getHostScanConfig()
	return HostScanCGIConfig{
		Port:      cfg.Port,
		SourceFPS: cfg.SourceFPS,
		User:      cfg.User,
		PW:        cfg.PW,
	}
}

func GetDeviceCGIConfig() DeviceCGIConfig {
	cfg := GetHostScanCGIConfig()
	return DeviceCGIConfig{
		DevicePort:   cfg.Port,
		DeviceUserID: cfg.User,
		DeviceUserPW: cfg.PW,
		SourceFPS:    cfg.SourceFPS,
	}
}

func UpdateHostScanCGIConfig(port int, user, pw string) (HostScanCGIConfig, error) {
	if port < 1 || port > 65535 {
		return HostScanCGIConfig{}, fmt.Errorf("port must be between 1 and 65535")
	}
	user = strings.TrimSpace(user)
	pw = strings.TrimSpace(pw)
	if user == "" || pw == "" {
		return HostScanCGIConfig{}, fmt.Errorf("user and pw are required")
	}

	globalHostScanConfig.mu.Lock()
	cfg := globalHostScanConfig.cfg
	cfg.Port = port
	cfg.User = user
	cfg.PW = pw
	globalHostScanConfig.cfg = cfg
	globalHostScanConfig.mu.Unlock()

	return HostScanCGIConfig{
		Port:      cfg.Port,
		SourceFPS: cfg.SourceFPS,
		User:      cfg.User,
		PW:        cfg.PW,
	}, nil
}

func UpdateDeviceCGIConfig(devicePort int, deviceUserID, deviceUserPW string, sourceFPS int) (DeviceCGIConfig, error) {
	if sourceFPS < 1 || sourceFPS > 120 {
		return DeviceCGIConfig{}, fmt.Errorf("sourceFps must be between 1 and 120")
	}
	updated, err := UpdateHostScanCGIConfig(devicePort, deviceUserID, deviceUserPW)
	if err != nil {
		return DeviceCGIConfig{}, err
	}
	globalHostScanConfig.mu.Lock()
	cfg := globalHostScanConfig.cfg
	cfg.SourceFPS = sourceFPS
	globalHostScanConfig.cfg = cfg
	globalHostScanConfig.mu.Unlock()

	return DeviceCGIConfig{
		DevicePort:   updated.Port,
		DeviceUserID: updated.User,
		DeviceUserPW: updated.PW,
		SourceFPS:    sourceFPS,
	}, nil
}

func getHostScanConfig() hostScanConfig {
	globalHostScanConfig.mu.RLock()
	defer globalHostScanConfig.mu.RUnlock()
	return globalHostScanConfig.cfg
}

func loadHostScanConfig() hostScanConfig {
	cfg := hostScanConfig{
		Port:      defaultHostScanPort,
		SourceFPS: defaultHostScanSourceFPS,
		User:      defaultHostScanUser,
		PW:        defaultHostScanPW,
		Timeout:   defaultHostScanTimeout,
		Interval:  defaultHostScanInterval,
		Workers:   defaultHostScanWorkerSize,
		LocalIP:   strings.TrimSpace(os.Getenv("HOST_SCAN_LOCAL_IP")),
	}

	if userVal := strings.TrimSpace(os.Getenv("HOST_SCAN_USER")); userVal != "" {
		cfg.User = userVal
	}
	if pwVal := strings.TrimSpace(os.Getenv("HOST_SCAN_PW")); pwVal != "" {
		cfg.PW = pwVal
	}
	if portVal := strings.TrimSpace(os.Getenv("HOST_SCAN_PORT")); portVal != "" {
		if parsed, err := strconv.Atoi(portVal); err == nil && parsed >= 1 && parsed <= 65535 {
			cfg.Port = parsed
		}
	}
	if fpsVal := strings.TrimSpace(os.Getenv("HOST_SCAN_SOURCE_FPS")); fpsVal != "" {
		if parsed, err := strconv.Atoi(fpsVal); err == nil && parsed >= 1 && parsed <= 120 {
			cfg.SourceFPS = parsed
		}
	}
	if timeoutVal := strings.TrimSpace(os.Getenv("HOST_SCAN_TIMEOUT_SEC")); timeoutVal != "" {
		if parsed, err := strconv.Atoi(timeoutVal); err == nil && parsed > 0 {
			cfg.Timeout = time.Duration(parsed) * time.Second
		}
	}
	if intervalVal := strings.TrimSpace(os.Getenv("HOST_SCAN_INTERVAL_SEC")); intervalVal != "" {
		if parsed, err := strconv.Atoi(intervalVal); err == nil && parsed > 0 {
			cfg.Interval = time.Duration(parsed) * time.Second
		}
	}
	if workersVal := strings.TrimSpace(os.Getenv("HOST_SCAN_WORKERS")); workersVal != "" {
		if parsed, err := strconv.Atoi(workersVal); err == nil && parsed > 0 {
			cfg.Workers = parsed
		}
	}
	return cfg
}

func runHostScanOnce(parent context.Context, cfg hostScanConfig) {
	localIP := cfg.LocalIP
	if localIP == "" {
		localIP = detectLocalIPv4()
	}
	if net.ParseIP(localIP) == nil {
		return
	}

	prefix, ok := build24Prefix(localIP)
	if !ok {
		return
	}

	results := scanSubnet(parent, prefix, localIP, cfg)
	globalHostRegistry.Replace(localIP, prefix, results)
	printScanSummary(localIP, prefix, results)
}

func printScanSummary(localIP, prefix string, hosts map[string]string) {
	fmt.Println("========================================")
	fmt.Printf("[host-scan] completed at %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("[host-scan] localIp=%s subnet=%s.0/24 found=%d\n", localIP, prefix, len(hosts))
	if len(hosts) == 0 {
		fmt.Println("[host-scan] no hostname discovered")
		fmt.Println("========================================")
		return
	}

	hostNames := make([]string, 0, len(hosts))
	for hostName := range hosts {
		hostNames = append(hostNames, hostName)
	}
	sort.Strings(hostNames)

	for _, hostName := range hostNames {
		fmt.Printf("[host-scan] %s -> %s\n", hostName, hosts[hostName])
	}
	fmt.Println("========================================")
}

func scanSubnet(parent context.Context, prefix, localIP string, cfg hostScanConfig) map[string]string {
	type hit struct {
		hostName string
		ip       string
	}

	jobs := make(chan string, 64)
	hits := make(chan hit, 64)

	workers := cfg.Workers
	if workers <= 0 {
		workers = defaultHostScanWorkerSize
	}

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ip := range jobs {
				hostName, ok := probeInfoCGI(parent, ip, cfg)
				if !ok {
					continue
				}
				hits <- hit{hostName: hostName, ip: ip}
			}
		}()
	}

	go func() {
		for i := 1; i <= 254; i++ {
			ip := fmt.Sprintf("%s.%d", prefix, i)
			if sameIPv4(ip, localIP) {
				continue
			}
			select {
			case <-parent.Done():
				close(jobs)
				return
			case jobs <- ip:
			}
		}
		close(jobs)
	}()

	go func() {
		wg.Wait()
		close(hits)
	}()

	result := make(map[string]string)
	for h := range hits {
		if h.hostName == "" {
			continue
		}
		if sameIPv4(h.ip, localIP) {
			continue
		}
		result[h.hostName] = h.ip
	}
	return result
}

func probeInfoCGI(parent context.Context, ip string, cfg hostScanConfig) (string, bool) {
	ctx, cancel := context.WithTimeout(parent, cfg.Timeout)
	defer cancel()

	client := &http.Client{Timeout: cfg.Timeout}
	targetURL := fmt.Sprintf("http://%s/info.cgi", net.JoinHostPort(ip, strconv.Itoa(cfg.Port)))
	body, statusCode, err := doDigestRequest(ctx, client, http.MethodGet, targetURL, cfg.User, cfg.PW)
	if err != nil || statusCode < 200 || statusCode >= 300 || len(body) == 0 {
		return "", false
	}

	info := ParseInfoCGIResponse(body)
	hostName := pickHostName(info)
	if hostName == "" {
		return "", false
	}
	return hostName, true
}

func pickHostName(info map[string]any) string {
	if len(info) == 0 {
		return ""
	}

	candidates := map[string]struct{}{
		"hostname":   {},
		"host":       {},
		"hostnameid": {},
		"devicename": {},
		"device":     {},
	}

	for key, val := range info {
		normalized := normalizeInfoKey(key)
		if _, ok := candidates[normalized]; !ok {
			if !strings.Contains(normalized, "hostname") && !strings.Contains(normalized, "devicename") {
				continue
			}
		}
		s := strings.TrimSpace(fmt.Sprintf("%v", val))
		if s != "" {
			return s
		}
	}
	return ""
}

func normalizeInfoKey(key string) string {
	raw := strings.TrimSpace(strings.ToLower(key))
	if raw == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func sameIPv4(a, b string) bool {
	aIP := net.ParseIP(strings.TrimSpace(a))
	bIP := net.ParseIP(strings.TrimSpace(b))
	if aIP == nil || bIP == nil {
		return false
	}
	return aIP.Equal(bIP)
}

func build24Prefix(ip string) (string, bool) {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return "", false
	}
	return strings.Join(parts[:3], "."), true
}

func detectLocalIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil {
				continue
			}
			ip = ip.To4()
			if ip == nil || ip.IsLoopback() {
				continue
			}
			return ip.String()
		}
	}
	return ""
}

func (r *hostRegistry) Replace(localIP, prefix string, hosts map[string]string) {
	if hosts == nil {
		hosts = make(map[string]string)
	}
	cloned := make(map[string]string, len(hosts))
	for k, v := range hosts {
		cloned[k] = v
	}

	r.mu.Lock()
	r.localIP = localIP
	r.subnetPrefix = prefix
	r.hosts = cloned
	r.scannedAt = time.Now()
	r.mu.Unlock()
}

func (r *hostRegistry) LookupIPByHostName(hostName string) string {
	key := strings.TrimSpace(hostName)
	if key == "" {
		return hostName
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if ip, ok := r.hosts[key]; ok {
		return ip
	}
	for k, ip := range r.hosts {
		if strings.EqualFold(k, key) {
			return ip
		}
	}
	return hostName
}

func (r *hostRegistry) Snapshot() HostRegistrySnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cloned := make(map[string]string, len(r.hosts))
	for k, v := range r.hosts {
		cloned[k] = v
	}
	return HostRegistrySnapshot{
		LocalIP:      r.localIP,
		SubnetPrefix: r.subnetPrefix,
		ScannedAt:    r.scannedAt.Format(time.RFC3339),
		Hosts:        cloned,
	}
}
