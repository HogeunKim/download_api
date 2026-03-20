package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"go-api-server/internal/model"
	"go-api-server/internal/service"
)

// CgiConfigHandler는 기본 CGI 접속값(devicePort/deviceUserId/deviceUserPw)을 조회/수정합니다.
func CgiConfigHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": http.StatusOK,
			"config":     service.GetDeviceCGIConfig(),
		})
		return
	case http.MethodPut:
		var req model.DeviceCGIConfigRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.DevicePort < 1 || req.DevicePort > 65535 {
			writeJSONError(w, http.StatusBadRequest, "devicePort must be between 1 and 65535")
			return
		}
		if strings.TrimSpace(req.DeviceUserID) == "" || strings.TrimSpace(req.DeviceUserPW) == "" {
			writeJSONError(w, http.StatusBadRequest, "deviceUserId and deviceUserPw are required")
			return
		}
		sourceFPS := req.SourceFPS
		if sourceFPS == 0 {
			sourceFPS = service.GetDeviceCGIConfig().SourceFPS
		}
		if sourceFPS < 1 || sourceFPS > 120 {
			writeJSONError(w, http.StatusBadRequest, "sourceFps must be between 1 and 120")
			return
		}

		updated, err := service.UpdateDeviceCGIConfig(req.DevicePort, req.DeviceUserID, req.DeviceUserPW, sourceFPS)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": http.StatusOK,
			"message":    "cgi config updated",
			"config":     updated,
		})
		return
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
}
