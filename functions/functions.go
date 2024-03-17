package functions

import (
	"canvas/models"
	"context"
	"encoding/json"
	"fmt"
	"log"
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

func VerifyPlaceTileMessage(pixelId int, color int) bool {
	if pixelId < 0 || pixelId > 40000 {
		return false
	}
	if color < 0 || color > 15 {
		return false
	}
	return true
}

//#endregion Verify Message

// #region Set Default Canvas
func MakeDefaultCanvas(redisClient *redis.Client) error {
	start := time.Now()
	pipe := redisClient.Pipeline()
	for i := 0; i < 10; i++ {
		for j := 0; j < 10; j++ {
			pixelID := (j * 10) + i
			pipe.Do(context.TODO(), "BITFIELD", "canvas", "SET", "i8", "#"+fmt.Sprint(pixelID), fmt.Sprint(0))
		}
	}
	_, err := pipe.Exec(context.TODO())
	if err != nil {
		return err
	}
	log.Println("MakeDefaultCanvas took", time.Since(start))
	return nil
}

func SetPixel(pixelID int, color int, redisClient *redis.Client) error {
	_, err := redisClient.Do(context.TODO(), "BITFIELD", "canvas", "SET", "i8", "#"+fmt.Sprint(pixelID), fmt.Sprint(color)).Result()
	if err != nil {
		return err
	}
	return nil
}

//#endregion Set Default Canvas

// #region Canvas
func GetCanvas(redisClient *redis.Client) ([]int64, error) {
	responseArr := make([]int64, 100) // todo: update correct value
	val, err := redisClient.Get(context.TODO(), "canvas").Result()
	if err != nil {
		return responseArr, err
	}
	for i, char := range val {
		responseArr[i] = int64(char)
	}
	return responseArr, nil

}

// func CheckPixel(userId string, pixelId int, redisClient *redis.Client) (userCoolDownRes bool, pixelCooldownRes bool, message string) {
// 	userCooldown, userCooldownError := redisClient.Get(context.TODO(), userId).Result()
// 	pixelCooldown, pixelCooldownError := redisClient.Get(context.TODO(), fmt.Sprintf("PIXEL:%d", pixelId)).Result()

// 	if userCooldownError == redis.Nil && pixelCooldownError == redis.Nil {
// 		return false, false, ""
// 	}

// 	if userCooldown != "" {
// 		secsLeft, err := time.Parse(time.RFC3339, userCooldown)
// 		if err != nil {
// 			return true, false, "Error calculating user cooldown expiry"
// 		}
// 		if userCooldown != "" {
// 			return true, false, fmt.Sprintf("User Cooldown: Wait for %v before placing another pixel!", time.Until(secsLeft))
// 		}
// 	}

// 	if pixelCooldown != "" {
// 		secsLeft, err := time.Parse(time.RFC3339, pixelCooldown)
// 		if err != nil {
// 			return false, true, "Error calculating pixel cooldown expiry"
// 		}
// 		if pixelCooldown != "" {
// 			return false, true, fmt.Sprintf("Pixel Cooldown: Wait for %v before placing another pixel!", time.Until(secsLeft))
// 		}
// 	}

// 	return false, false, "Can set pixel!"

// }

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

func SetPixelAndPublish(pixelId int, color int, userId string, redisClient *redis.Client, mongoClient *mongo.Client) (bool, error) {

	err := SetPixel(pixelId, color, redisClient)
	if err != nil {
		return false, err
	}
	userCooldown := time.Now().Add(models.USER_COOLDOWN_PERIOD * time.Second).Format(time.RFC3339)                   //todo: Real Value is 1 min
	pixelCooldown := time.Now().Add(models.PIXEL_COOLDOWN_PERIOD * time.Second).Format(time.RFC3339)                 //todo: Real Value is 1 min
	_, err = redisClient.Do(context.TODO(), "SET", userId, userCooldown, "EX", models.USER_COOLDOWN_PERIOD).Result() //todo: Real Value is 1 min
	if err != nil {
		return false, err
	}
	_, err = redisClient.Do(context.TODO(), "SET", fmt.Sprintf("PIXEL:%d", pixelId), pixelCooldown, "EX", models.PIXEL_COOLDOWN_PERIOD).Result()
	if err != nil {
		return false, err
	}

	//#region Publish on pub sub
	messString, err := json.Marshal(models.ServerPixelUpdate{
		MessageType: models.Update,
		UserId:      userId,
		PixelId:     pixelId,
		Color:       color,
	})
	if err != nil {
		return false, err
	}
	err = redisClient.Publish(context.TODO(), "pixelUpdates", messString).Err()
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

	_, err = mongoClient.Database("canvas").Collection("pixelUpdates").UpdateOne(context.TODO(), filter, update, updateOptions)
	if err != nil {
		return false, err
	}
	//#endregion save to mongo
	return true, nil
}

func GetPixel(pixelId int, mongoClient *mongo.Client) (models.SetPixelData, error) {
	filter := bson.M{"pixelId": pixelId}
	var setPixelData models.SetPixelData
	err := mongoClient.Database("canvas").Collection("pixelUpdates").FindOne(context.TODO(), filter).Decode(&setPixelData)
	if err != nil {
		return setPixelData, err
	}
	return setPixelData, nil
}

//#endregion Canvas
