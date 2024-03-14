package functions

import (
	"canvas/models"
	"context"
	"fmt"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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

func VerifyPlaceTileMessage(placeTileMessage models.PlaceTileMessage) bool {
	if placeTileMessage.PixelId < 0 || placeTileMessage.PixelId > 40000 {
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
	a := redisClient.Do(context.TODO(), "BITFIELD", "canvas", "SET", "i8", "#"+fmt.Sprint(pixelID), fmt.Sprint(color))
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
	val, err := redisClient.Get(context.TODO(), "canvas").Result()
	if err != nil {
		return responseArr, err
	}
	for i, char := range val {
		responseArr[i] = int64(char)
	}
	return responseArr, nil

}

func CanSetPixel(userId string, redisClient *redis.Client) (bool, string) {
	expiry, err := redisClient.Get(context.TODO(), userId).Result()
	if err != nil {
		return true, "Can set pixel because no data in redis!"
	}
	secsLeft, err := time.Parse(time.RFC3339, expiry)
	if err != nil {
		return false, err.Error()
	}
	if expiry != "" {
		return false, fmt.Sprintf("Wait for %v secs before placing another pixel!", time.Until(secsLeft))
	}
	return true, "Can set pixel!"

}

func SetPixelAndPublish(pixelId int, color int, userId string, redisClient *redis.Client, mongoClient *mongo.Client) (bool, error) {

	a := redisClient.Do(context.TODO(), "BITFIELD", "canvas", "SET", "i8", "#"+fmt.Sprint(pixelId), fmt.Sprint(color))
	_, err := a.Result()
	if err != nil {
		return false, err
	}
	expiry := time.Now().Add(30 * time.Second).Format(time.RFC3339)      //todo: Real Value is 1 min
	b := redisClient.Do(context.TODO(), "SET", userId, expiry, "EX", 30) //todo: Real Value is 1 min
	_, err = b.Result()
	if err != nil {
		return false, err
	}

	//#region Publish on pub sub
	mess := fmt.Sprintf("User: %s set pixelID #%d to color %d", userId, pixelId, color)
	err = redisClient.Publish(context.TODO(), "pixelUpdates", mess).Err()
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
		PixelId:   pixelId,
		Color:     color,
		TimeStamp: time.Now(),
	}

	filter := bson.M{"pixelId": setPixelData.PixelId}
	update := bson.M{"$set": setPixelData}

	updateOptions := options.Update().SetUpsert(true)

	_, err = mongoClient.Database("canvas").Collection("setPixelData").UpdateOne(context.TODO(), filter, update, updateOptions)
	if err != nil {
		return false, err
	}
	//#endregion save to mongo
	return true, nil
}

func GetPixel(pixelId int, mongoClient *mongo.Client) (models.SetPixelData, error) {
	filter := bson.M{"pixelId": pixelId}
	var setPixelData models.SetPixelData
	err := mongoClient.Database("canvas").Collection("setPixelData").FindOne(context.TODO(), filter).Decode(&setPixelData)
	if err != nil {
		return setPixelData, err
	}
	fmt.Println(setPixelData)
	return setPixelData, nil
}

//#endregion Canvas