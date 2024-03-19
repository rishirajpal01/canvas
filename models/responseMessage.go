package models

// ServerResponse Codes
const (
	Success       = 1
	UserCooldown  = 2
	PixelCooldown = 3
	Update        = 4
	NotFound      = 5
	Error         = 6
)

type ServerResponse struct {
	MessageType int    `json:"messageType"`
	Message     string `json:"message,omitempty"`
	Canvas      []int8 `json:"canvas,omitempty"`
	PixelData
}
