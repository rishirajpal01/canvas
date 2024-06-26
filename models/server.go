package models

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
)

// #region User
const (
	USER_COOLDOWN_PERIOD  = 10
	PIXEL_COOLDOWN_PERIOD = 20
)

const (
	DISCONNECT_AFTER_SECS = 30
	PING_INTERVAL         = 5
)

// #endregion User

// #region Canvas

const (
	DEFAULT_X_SIZE    = 200
	DEFAULT_Y_SIZE    = 200
	CAN_PLACE_TILE    = 0
	CANNOT_PLACE_TILE = -1
)

// #endregion Canvas

// #region Client
type Client struct {
	Conn             *websocket.Conn
	ServerChan       chan []byte
	RedisChan        chan []byte
	LastPong         time.Time
	UserId           string
	CanvasIdentifier string
	PixelsAvailable  uint16
}

func (c *Client) WriteEvents() {
	for {
		select {
		case message, ok := <-c.ServerChan:
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			err := c.Conn.WriteMessage(websocket.BinaryMessage, message)
			if err != nil {
				log.Println("server chan: error writing to websocket:", err)
			}
		case message, ok := <-c.RedisChan:
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			err := c.Conn.WriteMessage(websocket.BinaryMessage, message)
			if err != nil {
				log.Println("server redis: error writing to websocket:", err)
			}
		}
	}
}

// #endregion Client

type PixelData struct {
	UserId    string `json:"userId,omitempty" bson:"userId"`
	PixelId   int32  `json:"pixelId,omitempty" bson:"pixelId"`
	Color     int32  `json:"color,omitempty" bson:"color"`
	TimeStamp int64  `json:"timeStamp,omitempty" bson:"timeStamp"`
}
