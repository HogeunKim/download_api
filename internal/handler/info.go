package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"go-api-server/internal/model"
	"go-api-server/internal/service"
)

// InfoHandler는 대상 장비의 info.cgi 결과를 JSON으로 반환합니다.
func InfoHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req model.InfoRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.IP == "" {
		writeJSONError(w, http.StatusBadRequest, "ip is required")
		return
	}

	body, err := service.FetchInfoFromTarget(r.Context(), req)
	if err != nil {
		var targetErr *service.TargetHTTPError
		if errors.As(err, &targetErr) {
			writeJSONError(w, targetErr.StatusCode, targetErr.Message)
			return
		}
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"statusCode": status,
		"error":      message,
	})
}
