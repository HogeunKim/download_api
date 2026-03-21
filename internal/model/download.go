package model

type DownloadRequest struct {
	RequestID    int64             `json:"requestId"`
	CallbackURL  *string           `json:"callbackUrl"`
	DeviceIP     string            `json:"deviceIp"`
	ChannelList  []DownloadChannel `json:"channelList"`
	Begin        string            `json:"rangeBegin"`
	End          string            `json:"rangeEnd"`
	TargetFolder string            `json:"targetFolder"`
}

type DownloadChannel struct {
	Channel int    `json:"channel"`
	Name    string `json:"name"`
}
