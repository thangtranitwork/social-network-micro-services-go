package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port         string
	Neo4jURI     string
	Neo4jUser    string
	Neo4jPass    string
	RedisAddr    string
	KafkaAddr    string
}

func LoadConfig() *Config {
	_ = godotenv.Load("recommendation-service/.env")
	_ = godotenv.Load(".env")

	port := os.Getenv("RECOMMENDATION_HTTP_PORT")
	if port == "" {
		port = "10092"
	}

	neo4jURI := os.Getenv("NEO4J_URI")
	if neo4jURI == "" {
		neo4jURI = "neo4j://localhost:7687"
	}

	neo4jUser := os.Getenv("NEO4J_USER")
	if neo4jUser == "" {
		neo4jUser = "neo4j"
	}

	neo4jPass := os.Getenv("NEO4J_PASSWORD")
	if neo4jPass == "" {
		neo4jPass = "password"
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	kafkaAddr := os.Getenv("KAFKA_ADDR")
	if kafkaAddr == "" {
		kafkaAddr = "localhost:9092"
	}

	return &Config{
		Port:      port,
		Neo4jURI:  neo4jURI,
		Neo4jUser: neo4jUser,
		Neo4jPass: neo4jPass,
		RedisAddr: redisAddr,
		KafkaAddr: kafkaAddr,
	}
}
