package main

import (
	"fmt"
	"net/http"
	"os"

	"social-network-go/logger"
	"social-network-go/recommendation-service/config"
	"social-network-go/recommendation-service/db"
	"social-network-go/recommendation-service/handler"
	"social-network-go/recommendation-service/service"
)

func main() {
	cfg := config.LoadConfig()

	logger.Info("Starting recommendation-service on port %s...", cfg.Port)

	// Initialize Neo4j Graph Database
	db.InitNeo4j(cfg)

	// Initialize Service & Handler
	recService := service.NewRecommendationService(cfg)
	recHandler := handler.NewRecommendationHandler(recService)

	mux := http.NewServeMux()
	mux.HandleFunc("/recommendations/friends", recHandler.HandleGetSuggestedFriends)
	mux.HandleFunc("/recommendations/posts", recHandler.HandleGetSuggestedPosts)

	serverAddr := fmt.Sprintf(":%s", cfg.Port)
	if err := http.ListenAndServe(serverAddr, mux); err != nil {
		logger.Error("recommendation-service failed: %v", err)
		os.Exit(1)
	}
}
