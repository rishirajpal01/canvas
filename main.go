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
	"github.com/redis/go-redis/v9"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var clients = &sync.Map{}

func main() {

	// Redis Live Check
	pong, err := connections.RedisClient.Ping(context.TODO()).Result()
	if err != nil {
		panic(fmt.Sprintf("Redis is not live: %v", err))
	}
	log.Println("Redis Live, recieved: ", pong)
	defer connections.RedisClient.Close()
	// Mongo Live Check
	err = connections.MongoClient.Ping(context.Background(), nil)
	if err != nil {
		panic(fmt.Sprintf("Mongo is not live: %v", err))
	}
	log.Println("Mongo Live")
	defer connections.MongoClient.Disconnect(context.Background())

	// Subscribe to the pixelUpdates channel
	pubsub := connections.RedisClient.Subscribe(context.TODO(), "pixelUpdates")
	defer pubsub.Close()
	redisSubChan := pubsub.Channel()

	functions.MakeDefaultCanvas(connections.RedisClient)

	// Creates a channel for the jobs.
	jobs := make(chan *models.Client, models.NUM_OF_WORKERS)

	for i := 0; i < models.NUM_OF_WORKERS; i++ {
		go worker(jobs)
	}

	go broadcastRedisMessages(redisSubChan, clients)
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
			log.Println("ERR0: ", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Error upgrading to websocket!"))
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
		//#endregion Upgrade the HTTP connection to a websocket

		clients.Store(client, true)

		jobs <- client
	})
	http.ListenAndServe(":8080", nil)
}

func listen(client *models.Client) {

	//log the disconnect message if recieved by socket connection
	defer func() {
		log.Printf("User %v is disconnected!\n", client.UserId)
		clients.Delete(client)
	}()

	for {
		//set the last pong time to the current time on any event recieve
		client.LastPong = time.Now()

		//#region read a message
		messageType, messageContent, err := client.Conn.ReadMessage()
		if messageType == websocket.CloseMessage || messageType == -1 {
			close(client.ServerChan)
			close(client.RedisChan)
			client.Conn.Close()
			clients.Delete(client)
			return
		}
		if err != nil {
			log.Println("ERR1: ", err)
			client.ServerChan <- []byte("ERR128: Internal server error!")
			return
		}

		var userMessage models.UserMessage
		err = json.Unmarshal(messageContent, &userMessage)
		if err != nil {
			log.Println("ERR2: ", err)
			client.ServerChan <- []byte("ERR136: Internal server error!")
			return
		}
		//#endregion read a message

		//#region Verify User message

		isValidMessage := functions.VerifyMessage(userMessage.MessageType)
		if !isValidMessage {
			response, err := json.Marshal(models.ServerResponse{
				MessageType: models.Error,
				Message:     "Not a valid message!",
			})
			if err != nil {
				log.Println("ERR3: ", err)
			}
			client.ServerChan <- response
			continue
		}
		//#endregion Verify User message

		if userMessage.MessageType == models.SET_CANVAS {

			//#region verify placeTileMessage
			isValid := functions.VerifyPlaceTileMessage(userMessage.PixelId, userMessage.Color)
			if !isValid {
				client.ServerChan <- []byte("Not a valid place tile request!")
			}
			//#endregion verify placeTileMessage

			//#region canSet pixel
			userCoolDown, message := functions.CheckUserCooldown(client.UserId, connections.RedisClient)
			if userCoolDown {
				response, err := json.Marshal(models.ServerResponse{
					MessageType: models.UserCooldown,
					Message:     message,
				})
				if err != nil {
					log.Println("ERR4: ", err)
				}
				client.ServerChan <- response
				continue
			}
			pixelCoolDown, message := functions.CheckPixelCooldown(userMessage.PixelId, connections.RedisClient)
			if pixelCoolDown {
				response, err := json.Marshal(models.ServerResponse{
					MessageType: models.PixelCooldown,
					Message:     message,
				})
				if err != nil {
					log.Println("ERR5: ", err)
				}
				client.ServerChan <- response
				continue
			}
			//#endregion canSet pixel

			//#region Set pixel
			success, err := functions.SetPixelAndPublish(userMessage.PixelId, userMessage.Color, client.UserId, connections.RedisClient, connections.MongoClient)
			if !success {
				log.Println("ERR6: ", err)
				response, err := json.Marshal(models.ServerResponse{
					MessageType: models.Error,
					Message:     "Error setting pixel!",
				})
				if err != nil {
					client.ServerChan <- []byte("Error setting pixel!")
				}
				client.ServerChan <- response
			}
			//#endregion Set pixel

			//#region Send Response
			response, err := json.Marshal(models.ServerResponse{
				MessageType: models.Success,
				Message:     "Pixel set!",
			})
			if err != nil {
				log.Println("ERR7: ", err)
			}
			client.ServerChan <- response
			//#endregion Send Response

		} else if userMessage.MessageType == models.GET_CANVAS {

			//#region Get Canvas
			val, err := functions.GetCanvas(connections.RedisClient)
			if err != nil {
				log.Println("ERR8: ", err)
				client.ServerChan <- []byte("Error getting canvas!")
			}
			//#endregion Get Canvas

			//#region Send Canvas
			response, err := json.Marshal(models.GetCanvas{
				MessageType: models.Success,
				Canvas:      val,
			})
			if err != nil {
				log.Println("ERR9: ", err)
			}
			client.ServerChan <- response
			//#endregion Send Canvas

		} else if userMessage.MessageType == models.VIEW_PIXEL {

			//#region Get Pixel
			pixelValue, err := functions.GetPixel(userMessage.PixelId, connections.MongoClient)
			if err != nil {
				log.Println("ERR10: ", err)
				client.ServerChan <- []byte("Error viewing pixel")
			}
			//#endregion Get Pixel

			//#region Send Pixel
			response, err := json.Marshal(models.GetPixel{
				MessageType:  models.Success,
				SetPixelData: pixelValue,
			})
			if err != nil {
				log.Println("ERR10: ", err)
			}
			client.ServerChan <- response
			//#endregion Send Pixel

		} else if userMessage.MessageType == models.TEST {
			client.ServerChan <- []byte("Test message")
		} else {
			response, err := json.Marshal(models.ServerResponse{
				MessageType: models.Error,
				Message:     "Not a valid message!",
			})
			if err != nil {
				log.Println("ERR11: ", err)
			}
			client.ServerChan <- response
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
	clients.Range(func(key, value interface{}) bool {
		client := key.(*models.Client)
		if time.Since(client.LastPong) > models.DISCONNECT_AFTER_SECS*time.Second {
			log.Println("Client is not responding, closing connection: ", client.UserId)
			client.Conn.Close()
			clients.Delete(client)
		}
		return true
	})
}

func broadcastRedisMessages(redisSubChan <-chan *redis.Message, clients *sync.Map) {
	for msg := range redisSubChan {
		clients.Range(func(key, value interface{}) bool {
			client := key.(*models.Client)
			client.RedisChan <- []byte(msg.Payload)
			return true
		})

	}
}

func worker(jobs <-chan *models.Client) {
	for client := range jobs {
		listen(client)
	}
}
