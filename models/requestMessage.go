package models

// Message Types
const (
	GET_CONFIG = 0
	GET_CANVAS = 1
	SET_CANVAS = 2
	VIEW_PIXEL = 3
	DISCONNET  = 4
	TEST       = 5
)

type UserMessage struct {
	MessageType int32 `json:"messageType"`
	PixelId     int32 `json:"pixelId"`
	Color       int32 `json:"color"`
}
