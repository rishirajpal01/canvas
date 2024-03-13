package functions

import (
	"canvas/models"
	"context"
	"fmt"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/go-redis/redis"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// #region Verify User
func VerifyUser(userMessage models.UserMessage) bool {
	if userMessage.UserId == "" || userMessage.AuthToken == "" {
		return false
	}
	token, err := DecodeJWT(userMessage.AuthToken)
	if err != nil {
		fmt.Println(err)
		return false
	}
	return token.Claims.(jwt.MapClaims)["_id"] == userMessage.UserId
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

func VerifyPlaceTileMessage(placeTileMessage models.PlaceTileMessage) bool {
	if placeTileMessage.XCoordinate < 0 || placeTileMessage.XCoordinate > 1000 {
		return false
	}
	if placeTileMessage.YCoordinate < 0 || placeTileMessage.YCoordinate > 1000 {
		return false
	}
	if placeTileMessage.Color < 0 || placeTileMessage.Color > 15 {
		return false
	}
	return true
}

//#endregion Verify Message

// #region Set Default Canvas
func MakeDefaultCanvas(redisClient *redis.Client) error {
	for i := 0; i < 10; i++ {
		for j := 0; j < 10; j++ {
			_, err := SetPixel(i, j, 0, redisClient)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func SetPixel(x int, y int, color int, redisClient *redis.Client) (bool, error) {
	pixelID := (y * 10) + x
	a := redisClient.Do("BITFIELD", "canvas", "SET", "i8", "#"+fmt.Sprint(pixelID), fmt.Sprint(color))
	_, err := a.Result()
	if err != nil {
		return false, err
	}
	return true, nil
}

//#endregion Set Default Canvas

// #region Canvas
func GetCanvas(redisClient *redis.Client) ([]int64, error) {
	responseArr := make([]int64, 100)
	val, err := redisClient.Get("canvas").Result()
	if err != nil {
		return responseArr, err
	}
	for i, char := range val {
		responseArr[i] = int64(char)
	}
	return responseArr, nil

}

func CanSetPixel(userId string, redisClient *redis.Client) (bool, string) {
	expiry, err := redisClient.Get(userId).Result()
	if err != nil {
		return true, "Can set pixel because no data in redis!"
	}
	if expiry != "" {
		return false, fmt.Sprintf("Can't set pixel because user %s has to wait until %s", userId, expiry)
	}
	return true, "Can set pixel!"

}

func SetPixelAndPublish(x int, y int, color int, userId string, redisClient *redis.Client, mongoClient *mongo.Client) (bool, error) {
	pixelID := (y * 10) + x

	a := redisClient.Do("BITFIELD", "canvas", "SET", "i8", "#"+fmt.Sprint(pixelID), fmt.Sprint(color))
	_, err := a.Result()
	if err != nil {
		return false, err
	}
	expiry := time.Now().Add(30 * time.Second)
	b := redisClient.Do("SET", userId, expiry.String(), "EX", 30)
	_, err = b.Result()
	if err != nil {
		return false, err
	}

	//#region Publish on pub sub
	mess := fmt.Sprintf("User: %s set pixelID #%d to color %d", userId, pixelID, color)
	err = redisClient.Publish("pixelUpdates", mess).Err()
	if err != nil {
		return false, err
	}
	//#endregion Publish on pub sub

	//#region save to mongo
	userObjectId, err := primitive.ObjectIDFromHex(userId)
	if err != nil {
		return false, err
	}
	setPixelData := models.SetPixelData{
		UserId:    userObjectId,
		PixelId:   pixelID,
		Color:     color,
		TimeStamp: time.Now(),
	}

	filter := bson.M{"userId": setPixelData.UserId}
	update := bson.M{"$set": setPixelData}

	updateOptions := options.Update().SetUpsert(true)

	_, err = mongoClient.Database("canvas").Collection("setPixelData").UpdateOne(context.TODO(), filter, update, updateOptions)
	if err != nil {
		return false, err
	}
	//#endregion save to mongo
	return true, nil
}

func GetPixel(x int, y int, mongoClient *mongo.Client) (models.SetPixelData, error) {
	pixelID := (y * 10) + x
	filter := bson.M{"pixelId": pixelID}
	var setPixelData models.SetPixelData
	err := mongoClient.Database("canvas").Collection("setPixelData").FindOne(context.TODO(), filter).Decode(&setPixelData)
	if err != nil {
		return setPixelData, err
	}
	fmt.Println(setPixelData)
	return setPixelData, nil
}

//#endregion Canvas
