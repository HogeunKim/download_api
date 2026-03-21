package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"go-api-server/internal/model"
	"go-api-server/internal/service"
)

// CgiConfigHandler는 connect/record/debug 설정을 조회/수정합니다.
func CgiConfigHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		option := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("option")))
		cfg := service.GetUnifiedConfig()
		switch option {
		case "":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(cfg)
		case "connect":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"connect":    cfg.Connect,
				"statusCode": http.StatusOK,
			})
		case "record":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"record":     cfg.Record,
				"statusCode": http.StatusOK,
			})
		case "debug":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"debug":      cfg.Debug,
				"statusCode": http.StatusOK,
			})
		default:
			writeJSONError(w, http.StatusBadRequest, "option must be connect, record, or debug")
		}
		return
	case http.MethodPut:
		var req model.ConfigUpdateRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		updated, err := service.UpdateUnifiedConfig(req)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}

		w.WriteHeader(http.StatusOK)
		updated.StatusCode = http.StatusOK
		_ = json.NewEncoder(w).Encode(updated)
		return
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
}
