package model

type HostScanConfigRequest struct {
	Port int    `json:"port"`
	User string `json:"user"`
	PW   string `json:"pw"`
}

type HostScanSchedulerRequest struct {
	Enabled bool `json:"enabled"`
}
