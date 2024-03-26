package functions

import (
	"canvas/catalogue"
	"canvas/models"
	canvas "canvas/proto"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/protobuf/proto"
)

// #region Verify User
func VerifyUser(userId string, authToken string) bool {
	if userId == "" || authToken == "" {
		return false
	}
	tokenContent, err := DecodeJWT(authToken)
	if err != nil {
		fmt.Println(err)
		return false
	}
	return tokenContent.Claims.(jwt.MapClaims)["_id"] == userId
}

func DecodeJWT(tokenString string) (*jwt.Token, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte("xxx"), nil
	})
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	return token, nil
}

//#endregion Verify User

// #region Verify Message
func VerifyMessage(messageType int32) bool {
	if messageType < 0 || messageType > 3 {
		return false
	}
	return true
}

func VerifyPlaceTileMessage(pixelId, color int32, canvasIdentifier string) bool {
	validPixelId := false
	validColor := false
	if pixelId >= 0 || pixelId <= ((models.DEFAULT_X_SIZE*models.DEFAULT_Y_SIZE)-1) {
		validPixelId = true
	}
	if canvasIdentifier == catalogue.INDIA_CANVAS {
		if catalogue.INDIA_CANVAS_DATA[pixelId] == models.CAN_PLACE_TILE {
			validPixelId = true
		} else {
			validPixelId = false
		}
	}
	if color >= 1 || color <= 10 {
		validColor = true
	}
	return validPixelId && validColor
}

//#endregion Verify Message

// #region Set Default Canvas
func MakeDefaultCanvas(redisClient *redis.Client) error {
	for _, canvas := range catalogue.CANVAS_LIST {
		err := MakeCanvas(redisClient, canvas)
		if err != nil {
			return err
		}
	}
	return nil
}

func MakeCanvas(redisClient *redis.Client, canvasIdentifier string) error {
	pixelID := (models.DEFAULT_X_SIZE * models.DEFAULT_Y_SIZE) - 1
	_, err := redisClient.Do(context.TODO(), "BITFIELD", canvasIdentifier, "SET", "i8", "#"+fmt.Sprint(pixelID), fmt.Sprint(0)).Result()
	if err != nil {
		return err
	}
	return nil
}

func SetPixel(pixelID int32, color int32, canvasIndentifier string, redisClient *redis.Client) error {
	_, err := redisClient.Do(context.TODO(), "BITFIELD", canvasIndentifier, "SET", "i8", "#"+fmt.Sprint(pixelID), fmt.Sprint(color)).Result()
	if err != nil {
		return err
	}
	return nil
}

//#endregion Set Default Canvas

// #region Canvas
func GetCanvas(canvasIdentifier string, redisClient *redis.Client) ([]int32, error) {
	responseArr := make([]int32, models.DEFAULT_X_SIZE*models.DEFAULT_Y_SIZE)
	val, err := redisClient.Get(context.TODO(), canvasIdentifier).Result()
	if err != nil {
		return responseArr, err
	}
	for i, char := range val {
		responseArr[i] = int32(char)
	}
	return responseArr, nil

}

func CheckUserCooldown(userId string, redisClient *redis.Client) (bool, string) {
	userCooldown, userCooldownError := redisClient.Get(context.TODO(), userId).Result()
	if userCooldownError == redis.Nil {
		return false, ""
	}
	if userCooldown != "" {
		secsLeft, err := time.Parse(time.RFC3339, userCooldown)
		if err != nil {
			return false, "Error calculating user cooldown expiry"
		}
		if userCooldown != "" {
			return true, fmt.Sprintf("User Cooldown: Wait for %v before placing another pixel!", time.Until(secsLeft))
		}
	}
	return false, ""

}

func CheckPixelCooldown(pixelId int32, redisClient *redis.Client) (bool, string) {
	pixelCooldown, pixelCooldownError := redisClient.Get(context.TODO(), fmt.Sprintf("PIXEL:%d", pixelId)).Result()
	if pixelCooldownError == redis.Nil {
		return false, ""
	}
	if pixelCooldown != "" {
		secsLeft, err := time.Parse(time.RFC3339, pixelCooldown)
		if err != nil {
			return false, "Error calculating pixel cooldown expiry"
		}
		if pixelCooldown != "" {
			return true, fmt.Sprintf("Pixel Cooldown: Wait for %v before placing another pixel!", time.Until(secsLeft))
		}
	}
	return false, ""

}

func SetPixelAndPublish(pixelId int32, color int32, userId string, canvasIdentifier string, redisClient *redis.Client, mongoClient *mongo.Client) (bool, error) {

	err := SetPixel(pixelId, color, canvasIdentifier, redisClient)
	if err != nil {
		return false, err
	}
	userCooldown := time.Now().Add(models.USER_COOLDOWN_PERIOD * time.Second).Format(time.RFC3339)
	pixelCooldown := time.Now().Add(models.PIXEL_COOLDOWN_PERIOD * time.Second).Format(time.RFC3339)
	message := &canvas.ResponseMessage{
		MessageType: models.Update,
		UserId:      userId,
		PixelId:     pixelId,
		Color:       color,
	}
	messageByte, err := proto.Marshal(message)
	if err != nil {
		return false, err
	}
	pipe := redisClient.Pipeline()
	pipe.Do(context.TODO(), "SET", userId, userCooldown, "EX", models.USER_COOLDOWN_PERIOD).Result()
	pipe.Do(context.TODO(), "SET", fmt.Sprintf("PIXEL:%d", pixelId), pixelCooldown, "EX", models.PIXEL_COOLDOWN_PERIOD).Result()
	pipe.Publish(context.TODO(), "pixelUpdates", messageByte)
	_, err = pipe.Exec(context.TODO())
	if err != nil {
		return false, err
	}

	//#region save to mongo
	pixelData := models.PixelData{
		UserId:    userId,
		PixelId:   pixelId,
		Color:     color,
		TimeStamp: time.Now().Unix(),
	}

	filter := bson.M{"pixelId": pixelData.PixelId}
	update := bson.M{"$set": pixelData}

	updateOptions := options.Update().SetUpsert(true)

	_, err = mongoClient.Database("canvas").Collection(canvasIdentifier).UpdateOne(context.TODO(), filter, update, updateOptions)
	if err != nil {
		return false, err
	}
	//#endregion save to mongo
	return true, nil
}

func GetPixel(pixelId int32, canvasIdentifier string, mongoClient *mongo.Client) *canvas.ResponseMessage {
	filter := bson.M{"pixelId": pixelId}
	var pixelData *canvas.ResponseMessage
	mongoClient.Database("canvas").Collection(canvasIdentifier).FindOne(context.TODO(), filter).Decode(&pixelData)
	return pixelData
}

//#endregion Canvas

// #region Helper Functions

func CanvasExists(arr []string, element string) bool {
	for _, a := range arr {
		if a == element {
			return true
		}
	}
	return false
}

// #endregion Helper Functions

// #region HW

func GetPixelsAvailable(userId string) uint16 {

	response, err := http.Get(fmt.Sprintf("https://example.com/getPixelsAvailable/%s", userId))
	if err != nil {
		log.Println("Error getting pixels available:", err)
		return 0
	}
	body, err := io.ReadAll(response.Body)
	defer response.Body.Close()
	if err != nil {
		log.Println("Error reading response body:", err)
		return 0
	}

	var responseMap map[string]uint16
	err = json.Unmarshal(body, &responseMap)
	if err != nil {
		log.Println("Error unmarshalling response body:", err)
		return 0
	}

	if responseMap["pixelsAvailable"] > 0 {
		return responseMap["pixelsAvailable"]
	}

	return 0
}

// #endregion HW
