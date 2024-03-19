package models

// Message Types
const (
	GET_CANVAS = 0
	SET_CANVAS = 1
	VIEW_PIXEL = 2
	DISCONNET  = 3
)

type UserMessage struct {
	MessageType int `json:"messageType"`
	PixelId     int `json:"pixelId"`
	Color       int `json:"color"`
}
