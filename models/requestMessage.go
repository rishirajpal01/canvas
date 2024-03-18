package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

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

type SetPixelData struct {
	UserId    primitive.ObjectID `json:"userId" bson:"userId"`
	PixelId   int                `json:"pixelId" bson:"pixelId"`
	Color     int                `json:"color" bson:"color"`
	TimeStamp time.Time          `json:"timeStamp" bson:"timeStamp"`
}
