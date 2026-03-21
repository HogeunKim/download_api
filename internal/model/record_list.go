package model

type RecordListRequest struct {
	DeviceIP string `json:"deviceIp"`
}

type RecordListItem struct {
	Index     int    `json:"index"`
	IsDriving int    `json:"is_driving"`
	Channels  string `json:"channels"`
	STime     string `json:"stime"`
	ETime     string `json:"etime"`
	Completed bool   `json:"completed"`
}

type RecordListDrivingUnit struct {
	Count int              `json:"count"`
	Items []RecordListItem `json:"items"`
}

type RecordListResponse struct {
	DrivingUnit RecordListDrivingUnit `json:"drivingUnit"`
	StatusCode  int                   `json:"statusCode"`
}
