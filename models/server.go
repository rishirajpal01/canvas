package models

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
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

const (
	USER_COOLDOWN_PERIOD  = 10
	PIXEL_COOLDOWN_PERIOD = 20
)

const (
	NUM_OF_WORKERS = 150
)

const (
	DISCONNECT_AFTER_SECS = 30
	PING_INTERVAL         = 5
)

type Client struct {
	Conn       *websocket.Conn
	ServerChan chan []byte
	RedisChan  chan []byte
	LastPong   time.Time
	UserId     string
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
				log.Println("error writing to websocket:", err)
			}
		case message, ok := <-c.RedisChan:
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			err := c.Conn.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				log.Println("error writing to websocket:", err)
			}
		}
	}
}
