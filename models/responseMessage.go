package models

type PixelUpdate struct {
	PixelId  int `json:"pixelId"`
	Color    int `json:"color"`
	Cooldown int `json:"cooldown"`
}

// ServerResponse Codes
const (
	Success       = 1
	UserCooldown  = 2
	PixelCooldown = 3
	Update        = 4
	Error         = 5
)

type ServerResponse struct {
	MessageType int    `json:"messageType"`
	Message     string `json:"message"`
}

type GetCanvas struct {
	MessageType int    `json:"messageType"`
	Canvas      []int8 `json:"canvas"`
}

type GetPixel struct {
	MessageType int `json:"messageType"`
	SetPixelData
}

type ServerPixelUpdate struct {
	MessageType int    `json:"messageType"`
	UserId      string `json:"userId"`
	PixelId     int    `json:"pixelId"`
	Color       int    `json:"color"`
}
