package config

import "os"

type Config struct {
	HTTPPort  string
	KafkaAddr string
	Neo4jURI  string
	Neo4jUser string
	Neo4jPass string
}

func LoadConfig() *Config {
	getEnv := func(key, fallback string) string {
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return fallback
	}

	return &Config{
		HTTPPort:  getEnv("NOTIF_HTTP_PORT", "10085"),
		KafkaAddr: getEnv("KAFKA_ADDR", "localhost:9092"),
		Neo4jURI:  getEnv("NEO4J_URI", "neo4j://localhost:7687"),
		Neo4jUser: getEnv("NEO4J_USER", "neo4j"),
		Neo4jPass: getEnv("NEO4J_PASS", "password"),
	}
}
