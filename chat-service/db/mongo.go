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
var DeviceKeysCollection *mongo.Collection
var OneTimePreKeysCollection *mongo.Collection
var GroupSenderKeysCollection *mongo.Collection
var KeyBackupsCollection *mongo.Collection

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
	db := client.Database("chat_db")
	MsgCollection = db.Collection("messages")
	DeviceKeysCollection = db.Collection("e2e_device_keys")
	OneTimePreKeysCollection = db.Collection("e2e_one_time_prekeys")
	GroupSenderKeysCollection = db.Collection("e2e_group_sender_keys")
	KeyBackupsCollection = db.Collection("e2e_key_backups")

	createIndexes()
}

func createIndexes() {
	if MsgCollection == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	models := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "chat_id", Value: 1},
				{Key: "timestamp", Value: -1},
			},
			Options: options.Index().SetName("chat_id_timestamp_idx"),
		},
		{
			Keys: bson.D{
				{Key: "chat_id", Value: 1},
				{Key: "sender_id", Value: 1},
				{Key: "status", Value: 1},
			},
			Options: options.Index().SetName("chat_unread_messages_idx"),
		},
		{
			Keys: bson.D{
				{Key: "call_id", Value: 1},
			},
			Options: options.Index().SetName("call_id_idx").SetSparse(true),
		},
	}

	names, err := MsgCollection.Indexes().CreateMany(ctx, models)
	if err != nil {
		logger.Error("Failed to create compound indexes on messages: %v", err)
	} else {
		logger.Info("Successfully created MongoDB compound indexes: %v", names)
	}

	createE2EIndexes(ctx)
}

func createE2EIndexes(ctx context.Context) {
	if DeviceKeysCollection != nil {
		_, _ = DeviceKeysCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys: bson.D{
				{Key: "user_id", Value: 1},
				{Key: "device_id", Value: 1},
			},
			Options: options.Index().SetName("user_device_idx").SetUnique(true),
		})
	}

	if OneTimePreKeysCollection != nil {
		_, _ = OneTimePreKeysCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys: bson.D{
				{Key: "user_id", Value: 1},
				{Key: "device_id", Value: 1},
				{Key: "key_id", Value: 1},
			},
			Options: options.Index().SetName("user_device_key_idx").SetUnique(true),
		})
		_, _ = OneTimePreKeysCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys: bson.D{
				{Key: "user_id", Value: 1},
				{Key: "device_id", Value: 1},
			},
			Options: options.Index().SetName("user_device_lookup_idx"),
		})
	}

	if GroupSenderKeysCollection != nil {
		_, _ = GroupSenderKeysCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys: bson.D{
				{Key: "chat_id", Value: 1},
				{Key: "recipient_id", Value: 1},
				{Key: "recipient_device_id", Value: 1},
				{Key: "key_generation", Value: -1},
			},
			Options: options.Index().SetName("group_sender_key_lookup_idx"),
		})
	}
}
