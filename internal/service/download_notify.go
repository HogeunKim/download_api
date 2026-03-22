package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
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
