package main

import (
	"canvas/functions"
	"canvas/models"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var redisClient = redis.NewClient(&redis.Options{
	Addr:     "localhost:6379",
	Password: "",
	DB:       0,
})

type Client struct {
	Conn     *websocket.Conn
	LastPong time.Time
}

var clients = make(map[*Client]bool)
var mutex = &sync.Mutex{}

var mongoClient, _ = mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://localhost:27017"))

func main() {

	defer mongoClient.Disconnect(context.Background())
	defer redisClient.Close()

	pong, err := redisClient.Ping(context.TODO()).Result()
	if err != nil {
		panic(fmt.Sprintf("Redis is not live: %v", err))
	}
	fmt.Println("Redis Live, recieved: ", pong)

	functions.MakeDefaultCanvas(redisClient)

	go startPingPongChecker()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		//#region User Auth
		userId := r.URL.Query().Get("userId")
		xAuthToken := r.Header.Get("X-Auth-Token")
		isValidUser := functions.VerifyUser(userId, xAuthToken)
		if !isValidUser {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized"))
			return
		}
		//#endregion User Auth

		//#region Upgrade the HTTP connection to a websocket
		websocket, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println(err)
			return
		}
		log.Printf("User %v is connected!\n", userId)
		defer websocket.Close()
		client := &Client{
			Conn:     websocket,
			LastPong: time.Now(),
		}
		//#endregion Upgrade the HTTP connection to a websocket

		//#region Subscribe to the pixelUpdates channel
		pubsub := redisClient.Subscribe(context.TODO(), "pixelUpdates")
		defer pubsub.Close()
		ch := pubsub.Channel()
		rch := make(chan string, 100)
		log.Println("Subscribed to pixelUpdates channel")
		//#endregion Subscribe to the pixelUpdates channel

		// Start a goroutine to receive messages from the channel
		go func(wc chan string) {
			for msg := range ch {
				// Send the message to the client
				wc <- msg.Payload
			}
		}(rch)

		go func() {
			for msg := range rch {
				if err := websocket.WriteMessage(1, []byte("redisChannel: "+msg)); err != nil {
					log.Println("error writing to websocket:", err)
				}
			}
		}()

		listen(client)
	})
	http.ListenAndServe(":8080", nil)
}

func listen(client *Client) {
	//#region Ping Pong Handler

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			client.Conn.WriteMessage(1, []byte("Ping!"))
		}
	}()

	mutex.Lock()
	clients[client] = true
	mutex.Unlock()
	client.Conn.SetPongHandler(func(string) error { // Idea: set lastpong time only when pong is recieved
		client.LastPong = time.Now()
		return nil
	})
	//#endregion Ping Pong Handler

	for {
		// Set the read deadline to 30 seconds from now
		client.Conn.SetReadDeadline(time.Now().Add(30 * time.Second))

		//#region read a message
		messageType, messageContent, err := client.Conn.ReadMessage()
		if err != nil {
			log.Println(err)
			if err := client.Conn.WriteMessage(messageType, []byte(err.Error())); err != nil {
				log.Println(err)
				return
			}
			return
		}

		var userMessage models.UserMessage
		err = json.Unmarshal(messageContent, &userMessage)
		if err != nil {
			log.Println(err)
			if err := client.Conn.WriteMessage(messageType, []byte(err.Error())); err != nil {
				log.Println(err)
				return
			}
			return
		}
		//#endregion read a message

		//#region Verify User message

		isValidMessage := functions.VerifyMessage(userMessage.MessageType)
		if !isValidMessage {
			if err := client.Conn.WriteMessage(messageType, []byte("Not a valid message!")); err != nil {
				log.Println(err)
				return
			}
		}
		//#endregion Verify User message

		if userMessage.MessageType == models.SET_CANVAS {
			var placeTileMessage models.PlaceTileMessage

			//#region Incoming Message Verifications

			//turn userContent to bytes
			contentBytes, err := json.Marshal(userMessage.Content)
			if err != nil {
				if err := client.Conn.WriteMessage(messageType, []byte(err.Error())); err != nil {
					log.Println(err)
					return
				}
				return
			}

			// turn bytes to placeTileMessage
			err = json.Unmarshal(contentBytes, &placeTileMessage)
			if err != nil {
				if err := client.Conn.WriteMessage(messageType, []byte(err.Error())); err != nil {
					log.Println(err)
					return
				}
				return
			}

			// verify placeTileMessage
			isValid := functions.VerifyPlaceTileMessage(placeTileMessage)
			if !isValid {
				if err := client.Conn.WriteMessage(messageType, []byte("Not a valid place tile request")); err != nil {
					fmt.Println(err)
				}
			}

			//#endregion Incoming Message Verification

			//#region canSet pixel
			canSetPixel, message := functions.CanSetPixel(userMessage.UserId, redisClient)
			if !canSetPixel {
				if err := client.Conn.WriteMessage(messageType, []byte(message)); err != nil {
					log.Println(err)
					return
				}
				continue
			}
			//#endregion canSet pixel

			//#region Set pixel
			success, err := functions.SetPixelAndPublish(placeTileMessage.PixelId, placeTileMessage.Color, userMessage.UserId, redisClient, mongoClient)
			if !success {
				if err := client.Conn.WriteMessage(messageType, []byte(err.Error())); err != nil {
					log.Println(err)
					return
				}
			}
			//#endregion Set pixel

			//#region Send Response
			if err := client.Conn.WriteMessage(messageType, []byte("Pixel Set!")); err != nil {
				log.Println(err)
				return
			}
			//#endregion Send Response

		} else if userMessage.MessageType == models.GET_CANVAS {

			//#region Get Canvas
			val, err := functions.GetCanvas(redisClient)
			if err != nil {
				log.Println(err)
				if err := client.Conn.WriteMessage(messageType, []byte(err.Error())); err != nil {
					log.Println(err)
					return
				}
				return
			}
			//#endregion Get Canvas

			//#region Send Canvas
			if err := client.Conn.WriteMessage(messageType, []byte(fmt.Sprintf("%v", val))); err != nil {
				log.Println(err)
				return
			}
			//#endregion Send Canvas

		} else if userMessage.MessageType == models.VIEW_PIXEL {

			//#region Get Pixel
			pixelValue, err := functions.GetPixel(userMessage.Content.PixelId, mongoClient)
			if err != nil {
				log.Println(err)
				if err := client.Conn.WriteMessage(messageType, []byte(err.Error())); err != nil {
					log.Println(err)
					return
				}
				return
			}
			//#endregion Get Pixel

			//#region Send Pixel
			if err := client.Conn.WriteMessage(messageType, []byte(fmt.Sprintf("Pixel value: %v", pixelValue))); err != nil {
				log.Println(err)
				return
			}
			//#endregion Send Pixel

		} else if userMessage.MessageType == models.TEST {
			canSet, message := functions.CanSetPixel(userMessage.UserId, redisClient)
			if canSet {
				if err := client.Conn.WriteMessage(messageType, []byte(message)); err != nil {
					log.Println(err)
					return
				}
			} else {
				if err := client.Conn.WriteMessage(messageType, []byte(message)); err != nil {
					log.Println(err)
					return
				}
			}
		} else {
			if err := client.Conn.WriteMessage(messageType, []byte("Not a valid message!")); err != nil {
				fmt.Println(err)
			}
		}

	}
}

// startPingPongChecker checks if the clients are still connected
func startPingPongChecker() {
	ticker := time.NewTicker(5 * time.Second)
	for range ticker.C {
		checkClients()
	}
}

// checkClients checks if the clients are still connected
func checkClients() {
	mutex.Lock()
	for client := range clients {
		if time.Since(client.LastPong) > 30*time.Second {
			client.Conn.Close()
			delete(clients, client)
		} else {
			client.Conn.WriteMessage(websocket.PingMessage, nil)
		}
	}
	mutex.Unlock()
}
