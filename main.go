package main

import (
	"canvas/functions"
	"canvas/models"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
		//#endregion Upgrade the HTTP connection to a websocket

		//#region Subscribe to the pixelUpdates channel
		pubsub := redisClient.Subscribe(context.TODO(), "pixelUpdates")
		defer pubsub.Close()
		ch := pubsub.Channel()
		rch := make(chan string, 100)
		lch := make(chan string, 100)
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
			for {
				select {
				case msg := <-rch:
					if err := websocket.WriteMessage(1, []byte("redisChannel: "+msg)); err != nil {
						log.Println("error writing to websocket:", err)
					}
				case msg := <-lch:
					if err := websocket.WriteMessage(1, []byte("userChannel: "+msg)); err != nil {
						log.Println("error writing to websocket:", err)
					}
				}
			}
		}()

		listen(websocket, lch)
	})
	http.ListenAndServe(":8080", nil)
}

func listen(conn *websocket.Conn, wc chan string) {
	// Done channel to close go routine handling ping pong
	done := make(chan struct{})
	defer close(done)
	// Pong handler updates the deadline on every message revieved
	conn.SetPongHandler(func(string) error { conn.SetReadDeadline(time.Now().Add(20 * time.Second)); return nil })
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				wc <- "Ping!"
			}
		}
	}()
	for {
		// Set the read deadline to 20 seconds from now
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))

		//#region read a message
		messageType, messageContent, err := conn.ReadMessage()
		if err != nil {
			log.Println(err)
			wc <- err.Error()
			close(done)
			return
		}

		var userMessage models.UserMessage
		err = json.Unmarshal(messageContent, &userMessage)
		if err != nil {
			log.Println(err)
			wc <- err.Error()
			return
		}
		//#endregion read a message

		//#region Verify User message

		isValidMessage := functions.VerifyMessage(userMessage.MessageType)
		if !isValidMessage {
			wc <- "Not a valid message!"
		}
		//#endregion Verify User message

		if userMessage.MessageType == models.SET_CANVAS {
			var placeTileMessage models.PlaceTileMessage

			//#region Incoming Message Verifications

			//turn userContent to bytes
			contentBytes, err := json.Marshal(userMessage.Content)
			if err != nil {
				if err := conn.WriteMessage(messageType, []byte(err.Error())); err != nil {
					log.Println(err)
					return
				}
				return
			}

			// turn bytes to placeTileMessage
			err = json.Unmarshal(contentBytes, &placeTileMessage)
			if err != nil {
				if err := conn.WriteMessage(messageType, []byte(err.Error())); err != nil {
					log.Println(err)
					return
				}
				return
			}

			// verify placeTileMessage
			isValid := functions.VerifyPlaceTileMessage(placeTileMessage)
			if !isValid {
				if err := conn.WriteMessage(messageType, []byte("Not a valid place tile request")); err != nil {
					fmt.Println(err)
				}
			}

			//#endregion Incoming Message Verification

			//#region canSet pixel
			canSetPixel, message := functions.CanSetPixel(userMessage.UserId, redisClient)
			if !canSetPixel {
				wc <- message
				continue
			}
			//#endregion canSet pixel

			//#region Set pixel
			success, err := functions.SetPixelAndPublish(placeTileMessage.PixelId, placeTileMessage.Color, userMessage.UserId, redisClient, mongoClient)
			if !success {
				wc <- err.Error()
			}
			//#endregion Set pixel

			//#region Send Response
			wc <- "Pixel set!"
			//#endregion Send Response

		} else if userMessage.MessageType == models.GET_CANVAS {

			//#region Get Canvas
			val, err := functions.GetCanvas(redisClient)
			if err != nil {
				log.Println(err)
				if err := conn.WriteMessage(messageType, []byte(err.Error())); err != nil {
					log.Println(err)
					return
				}
				return
			}
			//#endregion Get Canvas

			//#region Send Canvas
			wc <- fmt.Sprintf("%v", val)
			//#endregion Send Canvas

		} else if userMessage.MessageType == models.VIEW_PIXEL {

			//#region Get Pixel
			pixelValue, err := functions.GetPixel(userMessage.Content.PixelId, mongoClient)
			if err != nil {
				log.Println(err)
				if err := conn.WriteMessage(messageType, []byte(err.Error())); err != nil {
					log.Println(err)
					return
				}
				return
			}
			//#endregion Get Pixel

			//#region Send Pixel
			wc <- fmt.Sprintf("Pixel value: %v", pixelValue)
			//#endregion Send Pixel

		} else if userMessage.MessageType == models.TEST {
			canSet, message := functions.CanSetPixel(userMessage.UserId, redisClient)
			if canSet {
				wc <- message
			} else {
				wc <- message
			}
		} else {
			if err := conn.WriteMessage(messageType, []byte("Not a valid message!")); err != nil {
				fmt.Println(err)
			}
		}

	}
}
