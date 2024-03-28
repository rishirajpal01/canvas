package main

import (
	"canvas/catalogue"
	"canvas/connections"
	"canvas/functions"
	"canvas/models"
	canvas "canvas/proto"
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"google.golang.org/protobuf/proto"
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
	redisSubChan := pubsub.Channel(redis.WithChannelSize(5000))

	err = functions.MakeDefaultCanvas(connections.RedisClient)
	if err != nil {
		panic(fmt.Sprintf("Error making default canvas: %v", err))
	}

	go broadcastRedisMessages(redisSubChan, clients)
	go startPingPongChecker()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		//#region User Auth
		userId := r.URL.Query().Get("userId")
		canvasIdentifier := r.URL.Query().Get("canvasIdentifier")
		validCanvas := functions.CanvasExists(catalogue.CANVAS_LIST, canvasIdentifier)
		if !validCanvas {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid Canvas Identifier"))
			return
		}
		_, err := primitive.ObjectIDFromHex(userId)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid User ID"))
			return
		}
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
			Conn:             websocket,
			ServerChan:       make(chan []byte),
			RedisChan:        make(chan []byte),
			LastPong:         time.Now(),
			UserId:           userId,
			CanvasIdentifier: canvasIdentifier,
		}
		go client.WriteEvents()
		//#endregion Upgrade the HTTP connection to a websocket

		clients.Store(client, true)

		go listen(client)
	})
	http.ListenAndServe(":8080", nil)
}

func listen(client *models.Client) {

	//log the disconnect message if recieved by socket connection
	defer func() {
		log.Printf("User %v is disconnected!\n", client.UserId)
		client.Conn.Close()
		clients.Delete(client)
	}()

	for {
		//set the last pong time to the current time on any event recieve
		client.LastPong = time.Now()

		//#region read a message
		messageType, messageContent, err := client.Conn.ReadMessage()
		if messageType == websocket.PingMessage {
			response := &canvas.ResponseMessage{
				MessageType: models.Error,
				Message:     "Pong!",
			}
			protoMessage, err := proto.Marshal(response)
			if err != nil {
				log.Println("ERR1: ", err)
				continue
			}
			client.ServerChan <- protoMessage
		}
		if messageType == websocket.CloseMessage || messageType == -1 {
			client.Conn.Close()
			clients.Delete(client)
			return
		}
		if err != nil {
			log.Println("ERR2: ", err)
			response := &canvas.ResponseMessage{
				MessageType: models.Error,
				Message:     "ERR4: Internal server error!",
			}
			protoMessage, err := proto.Marshal(response)
			if err != nil {
				log.Println("ERR3: ", err)
				client.Conn.Close()
				clients.Delete(client)
				return
			}
			client.ServerChan <- protoMessage
			client.Conn.Close()
			clients.Delete(client)
			return
		}

		var userMessage canvas.RequestMessage
		err = proto.Unmarshal(messageContent, &userMessage)
		if err != nil {
			log.Println("ERR4: ", err)
			response := &canvas.ResponseMessage{
				MessageType: models.Error,
				Message:     "ERR7: Internal server error!",
			}
			protoMessage, err := proto.Marshal(response)
			if err != nil {
				log.Println("ERR5: ", err)
				client.Conn.Close()
				clients.Delete(client)
				return
			}
			client.ServerChan <- protoMessage
			client.Conn.Close()
			clients.Delete(client)
			return
		}
		//#endregion read a message

		//#region Verify User message

		isValidMessage := functions.VerifyMessage(userMessage.GetMessageType())
		if !isValidMessage {
			response := &canvas.ResponseMessage{
				MessageType: models.Error,
				Message:     "Not a valid message!",
			}
			protoMessage, err := proto.Marshal(response)
			if err != nil {
				log.Println("ERR6: ", err)
				continue
			}
			client.ServerChan <- protoMessage
			client.Conn.Close()
			clients.Delete(client)
			return
		}
		//#endregion Verify User message

		if userMessage.GetMessageType() == models.GET_CONFIG {
			response := &canvas.ResponseMessage{
				MessageType:       models.Success,
				CanvasWidth:       models.DEFAULT_X_SIZE,
				CanvasHeight:      models.DEFAULT_Y_SIZE,
				UserCooldown:      models.USER_COOLDOWN_PERIOD,
				PixelCooldown:     models.PIXEL_COOLDOWN_PERIOD,
				PingInterval:      models.PING_INTERVAL,
				DisconnectTimeout: models.DISCONNECT_AFTER_SECS,
			}

			protoMessage, err := proto.Marshal(response)
			if err != nil {
				log.Println("ERR19: ", err)
				continue
			}
			client.ServerChan <- protoMessage
		} else if userMessage.GetMessageType() == models.SET_CANVAS {

			//#region verify placeTileMessage
			isValid := functions.VerifyPlaceTileMessage(userMessage.GetMessageType(), userMessage.GetColor(), client.CanvasIdentifier)
			if !isValid {
				response := &canvas.ResponseMessage{
					MessageType: models.Error,
					Message:     "Not a valid place tile request!",
				}
				protoMessage, err := proto.Marshal(response)
				if err != nil {
					log.Println("ERR7: ", err)
					continue
				}
				client.ServerChan <- protoMessage
				continue
			}
			//#endregion verify placeTileMessage

			//#region canSet pixel
			userCoolDown, message := functions.CheckUserCooldown(client.UserId, connections.RedisClient)
			if userCoolDown {
				response := &canvas.ResponseMessage{
					MessageType: models.UserCooldown,
					Message:     message,
				}
				protoMessage, err := proto.Marshal(response)
				if err != nil {
					log.Println("ERR8: ", err)
					continue
				}
				client.ServerChan <- protoMessage
				continue
			}
			pixelCoolDown, message := functions.CheckPixelCooldown(userMessage.GetPixelId(), connections.RedisClient)
			if pixelCoolDown {
				response := &canvas.ResponseMessage{
					MessageType: models.PixelCooldown,
					Message:     message,
				}
				protoMessage, err := proto.Marshal(response)
				if err != nil {
					log.Println("ERR9: ", err)
					continue
				}
				client.ServerChan <- protoMessage
				continue
			}
			//#endregion canSet pixel

			//#region Set pixel
			success, err := functions.SetPixelAndPublish(userMessage.GetPixelId(), userMessage.GetColor(), client.UserId, client.CanvasIdentifier, connections.RedisClient, connections.MongoClient)
			if !success {
				log.Println("ERR10: ", err)
				response := &canvas.ResponseMessage{
					MessageType: models.Error,
					Message:     "Error setting pixel!",
				}
				protoMessage, err := proto.Marshal(response)
				if err != nil {
					log.Println("ERR11: ", err)
					continue
				}
				client.ServerChan <- protoMessage
				continue
			}
			//#endregion Set pixel

			//#region Send Response
			response := &canvas.ResponseMessage{
				MessageType: models.Success,
				Message:     "Pixel set!",
			}
			protoMessage, err := proto.Marshal(response)
			if err != nil {
				log.Println("ERR12: ", err)
				continue
			}
			client.ServerChan <- protoMessage
			//#endregion Send Response

		} else if userMessage.GetMessageType() == models.GET_CANVAS {

			//#region Get Canvas
			val, err := functions.GetCanvas(client.CanvasIdentifier, connections.RedisClient)
			if err != nil {
				log.Println("ERR13: ", err)
				response := &canvas.ResponseMessage{
					MessageType: models.Error,
					Message:     "Error getting canvas!",
				}
				protoMessage, err := proto.Marshal(response)
				if err != nil {
					log.Println("ERR14: ", err)
					continue
				}
				client.ServerChan <- protoMessage
				continue
			}
			//#endregion Get Canvas

			//#region Send Canvas
			response := &canvas.ResponseMessage{
				MessageType: models.Success,
				Canvas:      val,
			}
			protoMessage, err := proto.Marshal(response)
			if err != nil {
				log.Println("ERR15: ", err)
				continue
			}
			client.ServerChan <- protoMessage
			//#endregion Send Canvas

		} else if userMessage.GetMessageType() == models.VIEW_PIXEL {

			//#region Get Pixel
			pixelValue := functions.GetPixel(userMessage.GetPixelId(), client.CanvasIdentifier, connections.MongoClient)
			//#endregion Get Pixel

			//#region Send Pixel
			var protoMessage []byte
			if pixelValue.UserId == "" {
				response := &canvas.ResponseMessage{
					MessageType: models.Success,
					Message:     "Fill the pixel!",
					UserId:      pixelValue.UserId,
					PixelId:     pixelValue.PixelId,
					Color:       pixelValue.Color,
				}
				protoMessage, err = proto.Marshal(response)
				if err != nil {
					log.Println("ERR16: ", err)
				}
			} else {
				response := &canvas.ResponseMessage{
					MessageType: models.Success,
					UserId:      pixelValue.UserId,
					PixelId:     pixelValue.PixelId,
					Color:       pixelValue.Color,
				}
				protoMessage, err = proto.Marshal(response)
				if err != nil {
					log.Println("ERR17: ", err)
				}
			}

			client.ServerChan <- protoMessage
			//#endregion Send Pixel

		} else {
			response := &canvas.ResponseMessage{
				MessageType: models.Error,
				Message:     "Not a valid message!",
			}
			protoMessage, err := proto.Marshal(response)
			if err != nil {
				log.Println("ERR18: ", err)
				continue
			}
			client.ServerChan <- protoMessage
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
