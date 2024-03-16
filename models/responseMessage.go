package models

type PixelUpdate struct {
	PixelId  int `json:"pixelId"`
	Color    int `json:"color"`
	Cooldown int `json:"cooldown"`
}

// ServerResponse Codes
const (
	Success  = 1
	Cooldown = 2
	Error    = 3
)

type ServerResponse struct {
	MessageType int    `json:"messageType"`
	Message     string `json:"message"`
}
