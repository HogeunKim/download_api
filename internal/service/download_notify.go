package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const MaxConcurrentDownloadJobs = 1

var downloadJobSemaphore = make(chan struct{}, MaxConcurrentDownloadJobs)

type DownloadNotifyPayload struct {
	Event      string `json:"event"`
	RequestID  int64  `json:"requestId"`
	TargetIP   string `json:"targetIp"`
	Message    string `json:"message,omitempty"`
	Timestamp  string `json:"timestamp"`
	Saved      bool   `json:"saved,omitempty"`
	TargetPath string `json:"targetPath,omitempty"`
}

func TryAcquireDownloadJob() bool {
	select {
	case downloadJobSemaphore <- struct{}{}:
		return true
	default:
		return false
	}
}

func ReleaseDownloadJob() {
	select {
	case <-downloadJobSemaphore:
	default:
	}
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
