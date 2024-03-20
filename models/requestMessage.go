package models

// Message Types
const (
	GET_CANVAS = 0
	SET_CANVAS = 1
	VIEW_PIXEL = 2
	DISCONNET  = 3
	TEST       = 4
)

type UserMessage struct {
	MessageType      int    `json:"messageType"`
	CanvasIdentifier string `json:"canvasIdentifier"`
	PixelId          int    `json:"pixelId"`
	Color            int    `json:"color"`
}
