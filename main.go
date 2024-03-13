package main

import (
	"canvas/functions"
	"canvas/models"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

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

	pong, err := redisClient.Ping(context.TODO()).Result()
	if err != nil {
		panic(fmt.Sprintf("Redis is not live: %v", err))
	}
	fmt.Println("Redis Live, recieved: ", pong)

	functions.MakeDefaultCanvas(redisClient)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		websocket, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println(err)
			return
		}
		defer websocket.Close()

		// Subscribe to the pixelUpdates channel

		pubsub := redisClient.Subscribe(context.TODO(), "pixelUpdates")
		defer pubsub.Close()
		ch := pubsub.Channel()
		_, err = pubsub.Receive(context.TODO())
		rch := make(chan string, 100)
		lch := make(chan string, 100)
		if err != nil {
			log.Fatal(err)
		}
		log.Println("Websocket Connected!")

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
	for {
		//#region read a message
		messageType, messageContent, err := conn.ReadMessage()
		if err != nil {
			log.Println(err)
			wc <- err.Error()
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

		//#region Verify User and their message
		isValidUser := functions.VerifyUser(userMessage)
		if !isValidUser {
			wc <- "Not a valid user!"
			err = conn.Close()
			if err != nil {
				fmt.Println(err)
				return
			}
		}

		isValidMessage := functions.VerifyMessage(userMessage.MessageType)
		if !isValidMessage {
			wc <- "Not a valid message!"
		}
		//#endregion Verify User and their message

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
			success, err := functions.SetPixelAndPublish(placeTileMessage.XCoordinate, placeTileMessage.YCoordinate, placeTileMessage.Color, userMessage.UserId, redisClient, mongoClient)
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
			pixelValue, err := functions.GetPixel(userMessage.Content.XCoordinate, userMessage.Content.YCoordinate, mongoClient)
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
