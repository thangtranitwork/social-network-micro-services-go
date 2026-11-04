package db

import (
	"context"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"social-network-go/logger"
	"social-network-go/recommendation-service/config"
)

var Neo4jDriver neo4j.DriverWithContext

func InitNeo4j(cfg *config.Config) neo4j.DriverWithContext {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
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
		// Warm up Neo4j connection pool and query planner to eliminate cold-start latency
		warmupCtx, warmupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer warmupCancel()
		session := driver.NewSession(warmupCtx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
		defer session.Close(warmupCtx)
		if _, err := session.Run(warmupCtx, "MATCH (n) RETURN 1 LIMIT 1", nil); err != nil {
			logger.Warn("Neo4j warmup query failed: %v", err)
		} else {
			logger.Info("Neo4j connection and query planner warmed up successfully (Recommendation Service)")
		}
	}

	Neo4jDriver = driver
	return driver
}
