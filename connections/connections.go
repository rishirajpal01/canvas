package connections

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var RedisClient = redis.NewClient(&redis.Options{
	Addr:        "localhost:6379",
	Password:    "",
	DB:          0,
	PoolSize:    100,
	PoolTimeout: 30 * time.Second,
})

var MongoClient, _ = mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://localhost:27017"))
