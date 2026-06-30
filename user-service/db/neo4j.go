package db

import (
	"context"
	"social-network-go/logger"
	"social-network-go/user-service/config"

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
		logger.Info("Successfully connected to Neo4j Graph Database")
		createIndexes(ctx)
	}

	return Neo4jDriver
}

func createIndexes(ctx context.Context) {
	if Neo4jDriver == nil {
		return
	}
	session := Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		q1 := `CREATE CONSTRAINT user_id_unique IF NOT EXISTS FOR (u:User) REQUIRE u.id IS UNIQUE`
		_, err := tx.Run(ctx, q1, nil)
		if err != nil {
			logger.Warn("Failed to create Neo4j constraint for user.id: %v", err)
		}

		q2 := `CREATE INDEX user_username_idx IF NOT EXISTS FOR (u:User) ON (u.username)`
		_, err = tx.Run(ctx, q2, nil)
		if err != nil {
			logger.Warn("Failed to create Neo4j index for user.username: %v", err)
		}

		return nil, nil
	})
	if err != nil {
		logger.Warn("Failed to execute Neo4j index/constraint creation: %v", err)
	} else {
		logger.Info("Neo4j database indexes and constraints checked/created successfully")
	}
}
