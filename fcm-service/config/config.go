package config

import "os"

type Config struct {
	GRPCPort                string
	HTTPPort                string
	KafkaAddr               string
	Neo4jURI                string
	Neo4jUser               string
	Neo4jPass               string
	FirebaseProjectID       string
	FirebaseSenderID        string
	FirebaseCredentialsFile string
	FirebaseCredentialsJSON string
}

func LoadConfig() *Config {
	getEnv := func(key, fallback string) string {
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return fallback
	}

	defaultJsonFile := "notification-service/config/pocpoc-498009-firebase-adminsdk-fbsvc-999cce8a3b.json"
	if _, err := os.Stat(defaultJsonFile); os.IsNotExist(err) {
		if _, err2 := os.Stat("fcm-service/config/pocpoc-498009-firebase-adminsdk-fbsvc-999cce8a3b.json"); err2 == nil {
			defaultJsonFile = "fcm-service/config/pocpoc-498009-firebase-adminsdk-fbsvc-999cce8a3b.json"
		}
	}

	return &Config{
		GRPCPort:                getEnv("FCM_GRPC_PORT", "10056"),
		HTTPPort:                getEnv("FCM_HTTP_PORT", "10086"),
		KafkaAddr:               getEnv("KAFKA_ADDR", "localhost:9092"),
		Neo4jURI:                getEnv("NEO4J_URI", "neo4j://localhost:7687"),
		Neo4jUser:               getEnv("NEO4J_USER", "neo4j"),
		Neo4jPass:               getEnv("NEO4J_PASS", "password"),
		FirebaseProjectID:       getEnv("FIREBASE_PROJECT_ID", "pocpoc-498009"),
		FirebaseSenderID:        getEnv("FIREBASE_MESSAGING_SENDER_ID", "1041761445609"),
		FirebaseCredentialsFile: getEnv("FIREBASE_CREDENTIALS_FILE", defaultJsonFile),
		FirebaseCredentialsJSON: getEnv("FIREBASE_CREDENTIALS_JSON", ""),
	}
}
