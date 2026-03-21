package model

type ConfigConnect struct {
	DevicePort   int    `json:"devicePort"`
	DeviceUserID string `json:"deviceUserId"`
	DeviceUserPW string `json:"deviceUserPw"`
}

type ConfigRecord struct {
	SourceFPS       int    `json:"sourceFps"`
	ContainerFormat string `json:"containerFormat"`
	ContainerOut    bool   `json:"containerOut"`
}

type ConfigDebug struct {
	Debug  bool `json:"debug"`
	JpgOut bool `json:"jpgOut"`
}

type ConfigResponse struct {
	Connect    ConfigConnect `json:"connect"`
	Record     ConfigRecord  `json:"record"`
	Debug      ConfigDebug   `json:"debug"`
	StatusCode int           `json:"statusCode"`
}

type ConfigConnectPatch struct {
	DevicePort   *int    `json:"devicePort"`
	DeviceUserID *string `json:"deviceUserId"`
	DeviceUserPW *string `json:"deviceUserPw"`
}

type ConfigRecordPatch struct {
	SourceFPS       *int    `json:"sourceFps"`
	ContainerFormat *string `json:"containerFormat"`
	ContainerOut    *bool   `json:"containerOut"`
}

type ConfigDebugPatch struct {
	Debug  *bool `json:"debug"`
	JpgOut *bool `json:"jpgOut"`
}

type ConfigUpdateRequest struct {
	Connect *ConfigConnectPatch `json:"connect"`
	Record  *ConfigRecordPatch  `json:"record"`
	Debug   *ConfigDebugPatch   `json:"debug"`
}
