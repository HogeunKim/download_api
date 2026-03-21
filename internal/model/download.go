package model

type DownloadRequest struct {
	RequestID    int64   `json:"requestId"`
	CallbackURL  *string `json:"callbackUrl"`
	DeviceIP     string  `json:"deviceIp"`
	Channel      string  `json:"channels"`
	Begin        string  `json:"rangeBegin"`
	End          string  `json:"rangeEnd"`
	TargetFolder string  `json:"targetFolder"`
}
