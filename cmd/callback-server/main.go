package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var Version = "0.0.2"

type callbackEvent struct {
	Event      string `json:"event"`
	RequestID  int64  `json:"requestId"`
	DeviceIP   string `json:"deviceIp"`
	GoServerIP string `json:"goServerIp,omitempty"`
	Message    string `json:"message,omitempty"`
	Timestamp  string `json:"timestamp"`
	Saved      bool   `json:"saved,omitempty"`
	TargetPath string `json:"targetPath,omitempty"`
	ReceivedAt string `json:"receivedAt"`
}

type eventStore struct {
	mu     sync.RWMutex
	events []callbackEvent
}

func (s *eventStore) add(event callbackEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append(s.events, event)
	if len(s.events) > 200 {
		s.events = s.events[len(s.events)-200:]
	}
}

func (s *eventStore) list() []callbackEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]callbackEvent, len(s.events))
	copy(out, s.events)
	return out
}

func main() {
	store := &eventStore{
		events: make([]callbackEvent, 0, 32),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /download-notify", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var event callbackEvent
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&event); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"statusCode": http.StatusBadRequest,
				"error":      "invalid callback payload",
			})
			return
		}

		event.ReceivedAt = time.Now().Format(time.RFC3339)
		store.add(event)
		log.Printf("[callback] event=%s requestId=%d deviceIp=%s goServerIp=%s message=%s saved=%t targetPath=%s",
			event.Event, event.RequestID, event.DeviceIP, event.GoServerIP, event.Message, event.Saved, event.TargetPath)

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": http.StatusOK,
			"message":    "callback received",
		})
	})

	mux.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": http.StatusOK,
			"count":      len(store.list()),
			"events":     store.list(),
		})
	})

	port := ":9000"
	host := resolveAdvertiseHost()
	baseURL := fmt.Sprintf("http://%s%s", host, port)
	fmt.Println("========================================")
	fmt.Printf("Callback Server 가동 중...\n")
	fmt.Printf("API 버전 : %s\n", Version)
	fmt.Printf("수신 주소: %s/download-notify\n", baseURL)
	fmt.Printf("이벤트 조회: %s/events\n", baseURL)
	fmt.Println("========================================")

	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("콜백 서버 실행 실패: %v", err)
	}
}

func resolveAdvertiseHost() string {
	manual := strings.TrimSpace(os.Getenv("CALLBACK_HOST"))
	if manual != "" {
		return manual
	}

	conn, err := net.DialTimeout("udp", "8.8.8.8:80", time.Second)
	if err == nil {
		defer conn.Close()
		if udpAddr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
			if ip := strings.TrimSpace(udpAddr.IP.String()); ip != "" {
				return ip
			}
		}
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet == nil || ipNet.IP == nil || ipNet.IP.IsLoopback() {
			continue
		}
		if v4 := ipNet.IP.To4(); v4 != nil {
			return v4.String()
		}
	}
	return "127.0.0.1"
}
