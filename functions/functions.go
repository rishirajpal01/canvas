package functions

import (
	"canvas/models"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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
func VerifyMessage(messageType int) bool {
	if messageType < 0 || messageType > 3 {
		return false
	}
	return true
}

func VerifyPlaceTileMessage(xCordinate, yCordinate, color int, canvasIdentifier string) bool {
	validPixelId := false
	validColor := false
	if canvasIdentifier == models.REGULAR_CANVAS {
		if xCordinate >= 0 || xCordinate < models.DEFAULT_X_SIZE || yCordinate >= 0 || yCordinate < models.DEFAULT_Y_SIZE {
			validPixelId = true
		}
	} else if canvasIdentifier == models.INDIA_CANVAS {
		pixelId := GetPixelId(xCordinate, yCordinate)
		if models.INDIA_CANVAS_DATA[pixelId] == 1 {
			validPixelId = true
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
	for _, canvas := range models.CANVAS_LIST {
		err := MakeCanvas(redisClient, canvas)
		if err != nil {
			return err
		}
	}
	return nil
}

func MakeCanvas(redisClient *redis.Client, canvasIdentifier string) error {
	pixelID := GetPixelId(models.DEFAULT_X_SIZE, models.DEFAULT_Y_SIZE)
	_, err := redisClient.Do(context.TODO(), "BITFIELD", canvasIdentifier, "SET", "i8", "#"+fmt.Sprint(pixelID), fmt.Sprint(0)).Result()
	if err != nil {
		return err
	}
	return nil
}

func SetPixel(pixelID int, color int, canvasIndentifier string, redisClient *redis.Client) error {
	_, err := redisClient.Do(context.TODO(), "BITFIELD", canvasIndentifier, "SET", "i8", "#"+fmt.Sprint(pixelID), fmt.Sprint(color)).Result()
	if err != nil {
		return err
	}
	return nil
}

//#endregion Set Default Canvas

// #region Canvas
func GetCanvas(canvasIdentifier string, redisClient *redis.Client) ([]int8, error) {
	responseArr := make([]int8, models.DEFAULT_X_SIZE*models.DEFAULT_Y_SIZE)
	val, err := redisClient.Get(context.TODO(), canvasIdentifier).Result()
	if err != nil {
		return responseArr, err
	}
	for i, char := range val {
		responseArr[i] = int8(char)
	}
	return responseArr, nil

}

func GetPixelId(xCordinate, yCordinate int) int {
	return (yCordinate * models.DEFAULT_X_SIZE) + xCordinate
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

func CheckPixelCooldown(pixelId int, redisClient *redis.Client) (bool, string) {
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

func SetPixelAndPublish(pixelId int, color int, userId string, canvasIdentifier string, redisClient *redis.Client, mongoClient *mongo.Client) (bool, error) {

	err := SetPixel(pixelId, color, canvasIdentifier, redisClient)
	if err != nil {
		return false, err
	}
	userCooldown := time.Now().Add(models.USER_COOLDOWN_PERIOD * time.Second).Format(time.RFC3339)
	pixelCooldown := time.Now().Add(models.PIXEL_COOLDOWN_PERIOD * time.Second).Format(time.RFC3339)
	messString, err := json.Marshal(models.ServerResponse{
		MessageType: models.Update,
		PixelData: models.PixelData{
			UserId:  userId,
			PixelId: pixelId,
			Color:   color,
		},
	})
	if err != nil {
		return false, err
	}
	pipe := redisClient.Pipeline()
	pipe.Do(context.TODO(), "SET", userId, userCooldown, "EX", models.USER_COOLDOWN_PERIOD).Result()
	pipe.Do(context.TODO(), "SET", fmt.Sprintf("PIXEL:%d", pixelId), pixelCooldown, "EX", models.PIXEL_COOLDOWN_PERIOD).Result()
	pipe.Publish(context.TODO(), "pixelUpdates", messString)
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

func GetPixel(pixelId int, canvasIdentifier string, mongoClient *mongo.Client) models.PixelData {
	filter := bson.M{"pixelId": pixelId}
	var pixelData models.PixelData
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
