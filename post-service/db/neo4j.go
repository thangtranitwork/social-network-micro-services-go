package db

import (
	"context"
	"social-network-go/post-service/config"
	"social-network-go/logger"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

var Neo4jDriver neo4j.DriverWithContext

func InitNeo4j(cfg *config.Config) neo4j.DriverWithContext {
	var err error
	Neo4jDriver, err = neo4j.NewDriverWithContext(
		cfg.Neo4jURI,
		neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPass, ""),
	)
	if err != nil {
		logger.Warn("Warning: Failed to create Neo4j driver at %s: %v", cfg.Neo4jURI, err)
		return nil
	}

	// Verify connectivity
	ctx := context.Background()
	err = Neo4jDriver.VerifyConnectivity(ctx)
	if err != nil {
		logger.Warn("Warning: Neo4j database is unreachable at %s: %v", cfg.Neo4jURI, err)
	} else {
		logger.Info("Successfully connected to Neo4j Graph Database (Post Service)")
	}

	return Neo4jDriver
}
