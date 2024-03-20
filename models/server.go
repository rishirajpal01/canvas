package models

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
)

// #region Workers
const (
	NUM_OF_WORKERS = 150
)

// #endregion Workers

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
	REGULAR_CANVAS = "REGULAR_CANVAS"
	INDIA_CANVAS   = "INDIA_CANVAS"
)

const (
	DEFAULT_X_SIZE    = 200
	DEFAULT_Y_SIZE    = 200
	CAN_PLACE_TILE    = 1
	CANNOT_PLACE_TILE = 0
)

var CANVAS_LIST = []string{REGULAR_CANVAS, INDIA_CANVAS}

var INDIA_CANVAS_DATA = []int{0, 0, 0, 1, 1, 1, 1, 0, 0, 0}

// #endregion Canvas

// #region Client
type Client struct {
	Conn             *websocket.Conn
	ServerChan       chan []byte
	RedisChan        chan []byte
	LastPong         time.Time
	UserId           string
	CanvasIdentifier string
}

func (c *Client) WriteEvents() {
	for {
		select {
		case message, ok := <-c.ServerChan:
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			err := c.Conn.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				log.Println("server chan: error writing to websocket:", err)
			}
		case message, ok := <-c.RedisChan:
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			err := c.Conn.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				log.Println("server redis: error writing to websocket:", err)
			}
		}
	}
}

// #endregion Client

type PixelData struct {
	UserId    string `json:"userId," bson:"userId"`
	PixelId   int    `json:"pixelId" bson:"pixelId"`
	Color     int    `json:"color" bson:"color"`
	TimeStamp int64  `json:"timeStamp,omitempty" bson:"timeStamp"`
}
