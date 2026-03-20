package model

type DeviceCGIConfigRequest struct {
	DevicePort   int    `json:"devicePort"`
	DeviceUserID string `json:"deviceUserId"`
	DeviceUserPW string `json:"deviceUserPw"`
	SourceFPS    int    `json:"sourceFps"`
}
