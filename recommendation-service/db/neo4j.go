package db

import (
	"context"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"social-network-go/logger"
	"social-network-go/recommendation-service/config"
)

var Neo4jDriver neo4j.DriverWithContext

func InitNeo4j(cfg *config.Config) neo4j.DriverWithContext {
	ctx := context.Background()
	driver, err := neo4j.NewDriverWithContext(cfg.Neo4jURI, neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPass, ""))
	if err != nil {
		logger.Warn("Failed to create Neo4j driver: %v", err)
		return nil
	}

	err = driver.VerifyConnectivity(ctx)
	if err != nil {
		logger.Warn("Warning: Neo4j database is unreachable at %s: %v", cfg.Neo4jURI, err)
	} else {
		logger.Info("Connected to Neo4j Graph Database successfully.")
	}

	Neo4jDriver = driver
	return driver
}
