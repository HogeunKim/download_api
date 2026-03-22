package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var (
	downloadJobMu      sync.Mutex
	activeDownloadByIP = make(map[string]int)
)

type DownloadNotifyPayload struct {
	Event      string `json:"event"`
	RequestID  int64  `json:"requestId"`
	DeviceIP   string `json:"deviceIp"`
	GoServerIP string `json:"goServerIp,omitempty"`
	Message    string `json:"message,omitempty"`
	Timestamp  string `json:"timestamp"`
	Saved      bool   `json:"saved,omitempty"`
	TargetPath string `json:"targetPath,omitempty"`
}

func TryAcquireDownloadJob(targetIP string) bool {
	key := normalizeDownloadTargetKey(targetIP)
	if key == "" {
		return false
	}

	downloadJobMu.Lock()
	defer downloadJobMu.Unlock()

	if activeDownloadByIP[key] > 0 {
		return false
	}
	activeDownloadByIP[key] = 1
	return true
}

func ReleaseDownloadJob(targetIP string) {
	key := normalizeDownloadTargetKey(targetIP)
	if key == "" {
		return
	}

	downloadJobMu.Lock()
	defer downloadJobMu.Unlock()

	if activeDownloadByIP[key] <= 1 {
		delete(activeDownloadByIP, key)
		return
	}
	activeDownloadByIP[key]--
}

func normalizeDownloadTargetKey(targetIP string) string {
	return strings.ToLower(strings.TrimSpace(targetIP))
}

func NotifyDownloadCallback(ctx context.Context, callbackURL string, payload DownloadNotifyPayload) {
	callbackURL = strings.TrimSpace(callbackURL)
	if callbackURL == "" {
		return
	}
	if strings.TrimSpace(payload.GoServerIP) == "" {
		payload.GoServerIP = resolveCallbackSourceIP(callbackURL)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	notifyCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(notifyCtx, http.MethodPost, callbackURL, bytes.NewReader(data))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

func resolveCallbackSourceIP(callbackURL string) string {
	u, err := url.Parse(callbackURL)
	if err != nil {
		return ""
	}

	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return ""
	}

	port := strings.TrimSpace(u.Port())
	if port == "" {
		if strings.EqualFold(u.Scheme, "https") {
			port = "443"
		} else {
			port = "80"
		}
	}

	addr := net.JoinHostPort(host, port)
	conn, err := net.DialTimeout("udp", addr, time.Second)
	if err == nil {
		defer conn.Close()
		if udpAddr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
			if ip := strings.TrimSpace(udpAddr.IP.String()); ip != "" {
				return ip
			}
		}
	}

	interfaces, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range interfaces {
		ipNet, ok := a.(*net.IPNet)
		if !ok || ipNet == nil || ipNet.IP == nil {
			continue
		}
		if ipNet.IP.IsLoopback() {
			continue
		}
		if v4 := ipNet.IP.To4(); v4 != nil {
			return v4.String()
		}
		if ip := strings.TrimSpace(ipNet.IP.String()); ip != "" {
			return ip
		}
	}
	return ""
}
