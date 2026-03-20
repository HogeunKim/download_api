package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"go-api-server/internal/model"
	"go-api-server/internal/service"
)

var timestampWithMillisPattern = regexp.MustCompile(`^\d{17}$`)
var timestampWithoutMillisPattern = regexp.MustCompile(`^\d{14}$`)
var timestampWithTWithoutMillisPattern = regexp.MustCompile(`^\d{8}T\d{6}$`)
var timestampWithTWithMillisPattern = regexp.MustCompile(`^\d{8}T\d{9}$`)

// DownloadHandler는 대상 장비의 download.cgi 데이터를 로컬 경로로 저장합니다.
func DownloadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req model.DownloadRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := validateDownloadRequest(req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.RequestID <= 0 {
		req.RequestID = time.Now().UnixMilli()
	}
	callbackURL := normalizeOptionalString(req.CallbackURL)
	targetIP := service.ResolveTargetAddress(req.DeviceIP)
	if !service.TryAcquireDownloadJob() {
		writeJSONError(w, http.StatusTooManyRequests, "too many concurrent download jobs")
		return
	}
	defer service.ReleaseDownloadJob()

	service.NotifyDownloadCallback(r.Context(), callbackURL, buildRunningNotifyPayload(req.RequestID, targetIP))

	notifyCtx, cancelNotify := context.WithCancel(r.Context())
	notifyDone := make(chan struct{})
	go runRunningNotifier(notifyCtx, notifyDone, callbackURL, req.RequestID, targetIP)

	result, err := service.DownloadToLocalPath(r.Context(), req)
	cancelNotify()
	<-notifyDone
	if err != nil {
		service.NotifyDownloadCallback(r.Context(), callbackURL, service.DownloadNotifyPayload{
			Event:     "failed",
			RequestID: req.RequestID,
			TargetIP:  targetIP,
			Message:   err.Error(),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		var targetErr *service.TargetHTTPError
		if errors.As(err, &targetErr) {
			writeJSONError(w, targetErr.StatusCode, targetErr.Message)
			return
		}
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	service.NotifyDownloadCallback(r.Context(), callbackURL, service.DownloadNotifyPayload{
		Event:      "completed",
		RequestID:  req.RequestID,
		TargetIP:   targetIP,
		Timestamp:  time.Now().Format(time.RFC3339),
		Saved:      result.Saved,
		TargetPath: result.TargetPath,
	})

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode":        http.StatusOK,
		"message":           "download completed",
		"requestId":         req.RequestID,
		"saved":             result.Saved,
		"targetPath":        result.TargetPath,
		"startTime":         result.StartTime,
		"endTime":           result.EndTime,
		"videoFormat":       result.VideoFormat,
		"selectedOutputFPS": result.SelectedOutputFPS,
		"containerPath":     result.ContainerPath,
		"containerPaths":    result.ContainerPaths,
		"containerFormat":   result.ContainerFormat,
		"audioFrameCount":   result.AudioFrameCount,
		"videoFrameCount":   result.VideoFrameCount,
	})
}

func runRunningNotifier(ctx context.Context, done chan<- struct{}, callbackURL string, requestID int64, targetIP string) {
	defer close(done)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			service.NotifyDownloadCallback(ctx, callbackURL, buildRunningNotifyPayload(requestID, targetIP))
		}
	}
}

func buildRunningNotifyPayload(requestID int64, targetIP string) service.DownloadNotifyPayload {
	return service.DownloadNotifyPayload{
		Event:     "running",
		RequestID: requestID,
		TargetIP:  targetIP,
		Message:   "download in progress",
		Timestamp: time.Now().Format(time.RFC3339),
	}
}

func validateDownloadRequest(req model.DownloadRequest) error {
	if req.DeviceIP == "" || req.TargetFolder == "" || req.Channel == "" || req.Begin == "" || req.End == "" {
		return errors.New("deviceIp, channels, rangeBegin, rangeEnd, targetFolder are required")
	}
	callbackURL := normalizeOptionalString(req.CallbackURL)
	if callbackURL != "" {
		u, err := url.ParseRequestURI(callbackURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return errors.New("callbackUrl must be valid http/https URL")
		}
	}
	if err := validateTargetFolderByOS(req.TargetFolder); err != nil {
		return err
	}
	beginValid := timestampWithMillisPattern.MatchString(req.Begin) ||
		timestampWithoutMillisPattern.MatchString(req.Begin) ||
		timestampWithTWithoutMillisPattern.MatchString(req.Begin) ||
		timestampWithTWithMillisPattern.MatchString(req.Begin)
	endValid := timestampWithMillisPattern.MatchString(req.End) ||
		timestampWithoutMillisPattern.MatchString(req.End) ||
		timestampWithTWithoutMillisPattern.MatchString(req.End) ||
		timestampWithTWithMillisPattern.MatchString(req.End)
	if !beginValid || !endValid {
		return errors.New("begin and end must be yyyyMMddTHHmmss or yyyyMMddTHHmmssSSS")
	}
	return nil
}

func normalizeOptionalString(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}
