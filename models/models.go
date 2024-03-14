package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	GET_CANVAS = 0
	SET_CANVAS = 1
	VIEW_PIXEL = 2
	DISCONNET  = 3
	TEST       = 4
)

const (
	MUMBAI     = 1  // Blue
	KOLKATA    = 2  // Purple
	DELHI      = 3  // Navy Blue
	BANGALORE  = 4  // Red
	HYDERABAD  = 5  // Orange
	CHENNAI    = 6  // Yellow
	JAIPUR     = 7  // Pink
	MOHALI     = 8  // Red (Silver Accents)
	AHEMEDABAD = 9  // Light Blue
	PUNE       = 10 // Lavender
)

type UserMessage struct {
	UserId      string           `json:"userId"`
	AuthToken   string           `json:"authToken"`
	MessageType int              `json:"messageType"`
	Content     PlaceTileMessage `json:"content"`
}

type PlaceTileMessage struct {
	PixelId int `json:"pixelId"`
	Color   int `json:"color"`
}

type SetPixelData struct {
	UserId    primitive.ObjectID `json:"userId" bson:"userId"`
	PixelId   int                `json:"xCoordinate" bson:"pixelId"`
	Color     int                `json:"color" bson:"color"`
	TimeStamp time.Time          `json:"timeStamp" bson:"timeStamp"`
}
