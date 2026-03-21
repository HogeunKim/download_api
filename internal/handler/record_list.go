package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"go-api-server/internal/model"
	"go-api-server/internal/service"
)

// RecordListHandler는 대상 장비의 datalist.cgi 결과 중 drivingUnit 목록을 반환합니다.
func RecordListHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req model.RecordListRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	deviceIP := strings.TrimSpace(req.DeviceIP)
	if deviceIP == "" {
		writeJSONError(w, http.StatusBadRequest, "deviceIp is required")
		return
	}

	result, err := service.FetchRecordListFromTarget(r.Context(), deviceIP)
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
	_ = json.NewEncoder(w).Encode(result)
}
