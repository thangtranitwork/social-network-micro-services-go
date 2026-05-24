package config

import "os"

type Config struct {
	HTTPPort  string
	Neo4jURI  string
	Neo4jUser string
	Neo4jPass string
	RedisAddr string
	RedisPass string
}

func LoadConfig() *Config {
	getEnv := func(key, fallback string) string {
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return fallback
	}

	return &Config{
		HTTPPort:  getEnv("ADMIN_HTTP_PORT", "8088"),
		Neo4jURI:  getEnv("NEO4J_URI", "neo4j://localhost:7687"),
		Neo4jUser: getEnv("NEO4J_USER", "neo4j"),
		Neo4jPass: getEnv("NEO4J_PASS", "password"),
		RedisAddr: getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPass: getEnv("REDIS_PASSWORD", ""),
	}
}
