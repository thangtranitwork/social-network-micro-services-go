package db

import (
	"context"
	"time"

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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = Neo4jDriver.VerifyConnectivity(ctx)
	if err != nil {
		logger.Warn("Warning: Neo4j database is unreachable at %s: %v", cfg.Neo4jURI, err)
		return Neo4jDriver
	}
	logger.Info("Successfully connected to Neo4j Graph Database (Post Service)")

	// Warm up Neo4j connection pool and query planner to eliminate cold-start latency
	warmupCtx, warmupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer warmupCancel()
	session := Neo4jDriver.NewSession(warmupCtx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(warmupCtx)
	if _, err := session.Run(warmupCtx, "MATCH (n) RETURN 1 LIMIT 1", nil); err != nil {
		logger.Warn("Neo4j warmup query failed: %v", err)
	} else {
		logger.Info("Neo4j connection and query planner warmed up successfully (Post Service)")
	}

	return Neo4jDriver
}
