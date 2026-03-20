package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"go-api-server/internal/model"
	"go-api-server/internal/service"
)

// HostScanConfigHandler는 host scan용 info.cgi 접속 기본값을 조회/수정합니다.
func HostScanConfigHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": http.StatusOK,
			"config":     service.GetHostScanCGIConfig(),
		})
		return
	case http.MethodPut:
		var req model.HostScanConfigRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.Port < 1 || req.Port > 65535 {
			writeJSONError(w, http.StatusBadRequest, "port must be between 1 and 65535")
			return
		}
		if strings.TrimSpace(req.User) == "" || strings.TrimSpace(req.PW) == "" {
			writeJSONError(w, http.StatusBadRequest, "user and pw are required")
			return
		}

		updated, err := service.UpdateHostScanCGIConfig(req.Port, req.User, req.PW)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": http.StatusOK,
			"message":    "host scan config updated",
			"config":     updated,
		})
		return
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
}

// HostScanSchedulerHandler는 host scan 스케줄러를 on/off 제어합니다.
func HostScanSchedulerHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": http.StatusOK,
			"scheduler":  service.GetHostScanSchedulerStatus(),
		})
		return
	case http.MethodPut:
		var req model.HostScanSchedulerRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		status := service.SetHostScanSchedulerEnabled(req.Enabled)
		message := "host scan scheduler turned off"
		if status.Enabled {
			message = "host scan scheduler turned on"
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": http.StatusOK,
			"message":    message,
			"scheduler":  status,
		})
		return
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
}
