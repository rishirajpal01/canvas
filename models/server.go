package models

import (
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
	USERCOOLDOWN  = 10
	PIXELCOOLDOWN = 20
)

const (
	DISCONNECT_AFTER_SECS = 30
	PING_INTERVAL         = 5
)

type Client struct {
	Conn     *websocket.Conn
	LastPong time.Time
	UserId   string
}
