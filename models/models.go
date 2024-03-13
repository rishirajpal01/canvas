package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	GET_CANVAS = iota
	SET_CANVAS
	VIEW_PIXEL
	DISCONNET
	TEST
)

type UserMessage struct {
	UserId      string           `json:"userId"`
	AuthToken   string           `json:"authToken"`
	MessageType int              `json:"messageType"`
	Content     PlaceTileMessage `json:"content"`
}

type PlaceTileMessage struct {
	XCoordinate int `json:"xCoordinate"`
	YCoordinate int `json:"yCoordinate"`
	Color       int `json:"color"`
}

type SetPixelData struct {
	UserId    primitive.ObjectID `json:"userId" bson:"userId"`
	PixelId   int                `json:"xCoordinate" bson:"pixelId"`
	Color     int                `json:"color" bson:"color"`
	TimeStamp time.Time          `json:"timeStamp" bson:"timeStamp"`
}
