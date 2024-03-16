package main

import (
	"canvas/connections"
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
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var clients = make(map[*models.Client]bool)
var mutex = &sync.Mutex{}

func main() {

	// Redis Live Check
	pong, err := connections.RedisClient.Ping(context.TODO()).Result()
	if err != nil {
		panic(fmt.Sprintf("Redis is not live: %v", err))
	}
	log.Println("Redis Live, recieved: ", pong)
	// Mongo Live Check
	err = connections.MongoClient.Ping(context.Background(), nil)
	if err != nil {
		panic(fmt.Sprintf("Mongo is not live: %v", err))
	}
	log.Println("Mongo Live")

	defer connections.MongoClient.Disconnect(context.Background())
	defer connections.RedisClient.Close()
	pubsub := connections.RedisClient.Subscribe(context.TODO(), "pixelUpdates")
	defer pubsub.Close()
	redisSubChan := pubsub.Channel()

	functions.MakeDefaultCanvas(connections.RedisClient)

	go startPingPongChecker()

	go func() {
		for msg := range redisSubChan {
			mutex.Lock()
			for client := range clients {
				client.RedisChan <- []byte(msg.Payload)
			}
			mutex.Unlock()
		}
	}()

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
		client := &models.Client{
			Conn:       websocket,
			ServerChan: make(chan []byte),
			RedisChan:  make(chan []byte),
			LastPong:   time.Now(),
			UserId:     userId,
		}
		go client.WriteEvents()
		defer client.Conn.Close()
		//#endregion Upgrade the HTTP connection to a websocket

		mutex.Lock()
		clients[client] = true
		mutex.Unlock()

		listen(client)
	})
	http.ListenAndServe(":8080", nil)
}

func listen(client *models.Client) {

	//log the disconnect message if recieved by socket connection
	defer func() {
		log.Printf("User %v is disconnected!\n", client.UserId)
		mutex.Lock()
		delete(clients, client)
		mutex.Unlock()
	}()

	for {
		//set the last pong time to the current time on any event recieve
		client.LastPong = time.Now()

		//#region read a message
		_, messageContent, err := client.Conn.ReadMessage()
		if err != nil {
			log.Println(err)
			client.ServerChan <- []byte("Error reading message from connection!")
			return
		}

		var userMessage models.UserMessage
		err = json.Unmarshal(messageContent, &userMessage)
		if err != nil {
			log.Println(err)
			client.ServerChan <- []byte("Error unmarshaling user message!")
			return
		}
		//#endregion read a message

		//#region Verify User message

		isValidMessage := functions.VerifyMessage(userMessage.MessageType)
		if !isValidMessage {
			client.ServerChan <- []byte("Not a valid message!")
		}
		//#endregion Verify User message

		if userMessage.MessageType == models.SET_CANVAS {
			//#region Incoming Message Verifications

			//turn userContent to bytes

			// verify placeTileMessage
			isValid := functions.VerifyPlaceTileMessage(userMessage.PixelId, userMessage.Color)
			if !isValid {
				client.ServerChan <- []byte("Not a valid place tile request!")
			}

			//#endregion Incoming Message Verification

			//#region canSet pixel
			canSetPixel, message := functions.CanSetPixel(client.UserId, userMessage.PixelId, connections.RedisClient)
			if !canSetPixel {
				client.ServerChan <- []byte(message)
				continue
			}
			//#endregion canSet pixel

			//#region Set pixel
			success, _ := functions.SetPixelAndPublish(userMessage.PixelId, userMessage.Color, client.UserId, connections.RedisClient, connections.MongoClient)
			if !success {
				client.ServerChan <- []byte("Error setting pixel!")
			}
			//#endregion Set pixel

			//#region Send Response
			client.ServerChan <- []byte("Pixel set!")
			//#endregion Send Response

		} else if userMessage.MessageType == models.GET_CANVAS {

			//#region Get Canvas
			val, err := functions.GetCanvas(connections.RedisClient)
			if err != nil {
				log.Println(err)
				client.ServerChan <- []byte("Error getting canvas!")
				return
			}
			//#endregion Get Canvas

			//#region Send Canvas
			client.ServerChan <- []byte(fmt.Sprintf("%v", val))
			//#endregion Send Canvas

		} else if userMessage.MessageType == models.VIEW_PIXEL {

			//#region Get Pixel
			pixelValue, err := functions.GetPixel(userMessage.PixelId, connections.MongoClient)
			if err != nil {
				log.Println(err)
				client.ServerChan <- []byte(fmt.Sprintf("Error viewing pixel %d", userMessage.PixelId))
				return
			}
			//#endregion Get Pixel

			//#region Send Pixel
			client.ServerChan <- []byte(fmt.Sprintf("Pixel value: %v", pixelValue))
			//#endregion Send Pixel

		} else if userMessage.MessageType == models.TEST {
			client.ServerChan <- []byte("Test message")
		} else {
			client.ServerChan <- []byte("Not a valid message!")
		}

	}
}

// startPingPongChecker checks if the clients are still connected
func startPingPongChecker() {
	ticker := time.NewTicker(models.PING_INTERVAL * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		checkClients()
	}
}

// checkClients checks if the clients are still connected
func checkClients() {
	for client := range clients {
		if time.Since(client.LastPong) > models.DISCONNECT_AFTER_SECS*time.Second {
			log.Println("Client is not responding, closing connection: ", client.UserId)
			mutex.Lock()
			client.Conn.Close()
			delete(clients, client)
			mutex.Unlock()
		} else {
			client.Conn.WriteMessage(websocket.PingMessage, nil)
		}
	}
}
