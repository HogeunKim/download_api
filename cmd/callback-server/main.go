package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

type callbackEvent struct {
	Event      string `json:"event"`
	RequestID  int64  `json:"requestId"`
	DeviceIP   string `json:"deviceIp"`
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
		log.Printf("[callback] event=%s requestId=%d deviceIp=%s message=%s saved=%t targetPath=%s",
			event.Event, event.RequestID, event.DeviceIP, event.Message, event.Saved, event.TargetPath)

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
	fmt.Println("========================================")
	fmt.Printf("Callback Server 가동 중...\n")
	fmt.Printf("수신 주소: http://localhost%s/download-notify\n", port)
	fmt.Printf("이벤트 조회: http://localhost%s/events\n", port)
	fmt.Println("========================================")

	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("콜백 서버 실행 실패: %v", err)
	}
}
