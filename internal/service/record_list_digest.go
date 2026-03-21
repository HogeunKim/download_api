package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go-api-server/internal/model"
)

type datalistResponse struct {
	DrivingUnit datalistDrivingUnit `json:"drivingUnit"`
}

type datalistDrivingUnit struct {
	Count int               `json:"count"`
	Items []datalistItemRaw `json:"items"`
}

type datalistItemRaw struct {
	Index        int    `json:"index"`
	IsDriving    int    `json:"is_driving"`
	ModelChFlags string `json:"model_ch_flags"`
	STime        string `json:"stime"`
	ETime        string `json:"etime"`
	Completed    bool   `json:"completed"`
}

func FetchRecordListFromTarget(ctx context.Context, deviceIP string) (model.RecordListResponse, error) {
	targetAddress := ResolveTargetAddress(deviceIP)
	if strings.TrimSpace(targetAddress) == "" {
		return model.RecordListResponse{}, fmt.Errorf("deviceIp is required")
	}

	cfg := GetHostScanCGIConfig()
	targetURL := fmt.Sprintf("http://%s/datalist.cgi", net.JoinHostPort(targetAddress, strconv.Itoa(cfg.Port)))
	client := &http.Client{Timeout: 10 * time.Second}

	body, statusCode, err := doDigestRequest(ctx, client, http.MethodGet, targetURL, cfg.User, cfg.PW)
	if err != nil {
		return model.RecordListResponse{}, err
	}
	if statusCode < 200 || statusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = "empty error body"
		}
		return model.RecordListResponse{}, &TargetHTTPError{
			StatusCode: statusCode,
			Message:    msg,
		}
	}

	var raw datalistResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return model.RecordListResponse{}, fmt.Errorf("failed to parse datalist.cgi response: %w", err)
	}

	items := make([]model.RecordListItem, 0, len(raw.DrivingUnit.Items))
	for _, it := range raw.DrivingUnit.Items {
		items = append(items, model.RecordListItem{
			Index:     it.Index,
			IsDriving: it.IsDriving,
			Channels:  channelFlagsToIndexes(it.ModelChFlags),
			STime:     it.STime,
			ETime:     it.ETime,
			Completed: it.Completed,
		})
	}

	return model.RecordListResponse{
		DrivingUnit: model.RecordListDrivingUnit{
			Count: raw.DrivingUnit.Count,
			Items: items,
		},
		StatusCode: http.StatusOK,
	}, nil
}

func channelFlagsToIndexes(flags string) string {
	v, err := strconv.ParseUint(strings.TrimSpace(flags), 0, 64)
	if err != nil || v == 0 {
		return ""
	}

	channels := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		if (v & (1 << i)) != 0 {
			channels = append(channels, strconv.Itoa(i+1))
		}
	}
	return strings.Join(channels, ",")
}
