package db

import (
	"context"
	"time"

	"social-network-go/chat-service/config"
	"social-network-go/logger"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var MongoClient *mongo.Client
var MsgCollection *mongo.Collection

func InitDB(cfg *config.Config) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		logger.Error("Failed to connect to MongoDB: %v", err)
		return
	}

	err = client.Ping(ctx, nil)
	if err != nil {
		logger.Error("Failed to ping MongoDB: %v", err)
		return
	}

	logger.Info("Successfully connected to MongoDB")
	MongoClient = client
	MsgCollection = client.Database("chat_db").Collection("messages")

	createIndexes()
}

func createIndexes() {
	if MsgCollection == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	indexModel := mongo.IndexModel{
		Keys: bson.D{
			{Key: "chat_id", Value: 1},
			{Key: "timestamp", Value: -1},
		},
		Options: options.Index().SetName("chat_id_timestamp_idx"),
	}

	name, err := MsgCollection.Indexes().CreateOne(ctx, indexModel)
	if err != nil {
		logger.Error("Failed to create compound index on messages: %v", err)
	} else {
		logger.Info("Successfully created MongoDB compound index: %s", name)
	}
}
