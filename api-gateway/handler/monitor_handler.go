package handler

import (
	_ "embed"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"social-network-go/api-gateway/config"
)

//go:embed monitor_dashboard.html
var monitorDashboardHTML string

// MonitorDashboard serves the interactive Service Map & Monitor Dashboard
func MonitorDashboard(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(monitorDashboardHTML))
}

// ServiceStatus represents the detailed state of a microservice or database
type ServiceStatus struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // "gateway", "service", "db", "mq", "search"
	HttpPort string `json:"httpPort"`
	GrpcPort string `json:"grpcPort"`
	Status   string `json:"status"`   // "UP", "DOWN"
	PingTime int64  `json:"pingTime"` // in milliseconds
	Error    string `json:"error,omitempty"`
}

// CheckHealth queries all services and dependencies concurrently
func CheckHealth(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		services := []struct {
			Name     string
			Type     string
			HttpAddr string
			GrpcAddr string
		}{
			{"api-gateway", "gateway", "localhost:" + cfg.Port, ""},
			{"auth-service", "service", cfg.AuthHttpAddr, cfg.AuthGrpcAddr},
			{"user-service", "service", cfg.UserHttpAddr, "localhost:10052"},
			{"post-service", "service", cfg.PostHttpAddr, "localhost:10053"},
			{"chat-service", "service", cfg.ChatHttpAddr, ""},
			{"notification-service", "service", cfg.NotificationHttpAddr, ""},
			{"file-service", "service", cfg.FileHttpAddr, "localhost:10057"},
			{"admin-service", "service", cfg.AdminHttpAddr, ""},
			{"search-service", "service", cfg.SearchHttpAddr, ""},
			{"story-service", "service", cfg.StoryHttpAddr, ""},
			{"ai-service", "service", "http://localhost:10091", ""},
			{"PostgreSQL", "db", "localhost:5432", ""},
			{"Redis", "db", cfg.RedisAddr, ""},
			{"MongoDB", "db", "localhost:27017", ""},
			{"Neo4j", "db", "localhost:7687", ""},
			{"Kafka", "mq", "localhost:9092", ""},
			{"Elasticsearch", "search", "localhost:9200", ""},
		}

		results := make([]ServiceStatus, len(services))
		var wg sync.WaitGroup

		for i, s := range services {
			wg.Add(1)
			go func(index int, name, srvType, httpAddr, grpcAddr string) {
				defer wg.Done()

				status := "DOWN"
				var pingTime time.Duration
				var errMsg string

				// Choose target address to perform TCP check
				var targetHost string
				if httpAddr != "" {
					clean := httpAddr
					if idx := strings.Index(clean, "://"); idx != -1 {
						clean = clean[idx+3:]
					}
					targetHost = clean
				} else if grpcAddr != "" {
					targetHost = grpcAddr
				}

				if targetHost != "" {
					// Standardize hostname
					if !strings.Contains(targetHost, ":") {
						if name == "Redis" {
							targetHost += ":6379"
						} else {
							targetHost += ":80"
						}
					}

					start := time.Now()
					conn, err := net.DialTimeout("tcp", targetHost, 150*time.Millisecond)
					if err == nil {
						status = "UP"
						pingTime = time.Since(start)
						conn.Close()
					} else {
						errMsg = err.Error()
					}
				}

				// Extract ports for presentation
				httpPort := ""
				if httpAddr != "" {
					parts := strings.Split(httpAddr, ":")
					if len(parts) > 0 {
						httpPort = parts[len(parts)-1]
						if slashIdx := strings.Index(httpPort, "/"); slashIdx != -1 {
							httpPort = httpPort[:slashIdx]
						}
					}
				}

				grpcPort := ""
				if grpcAddr != "" {
					parts := strings.Split(grpcAddr, ":")
					if len(parts) > 0 {
						grpcPort = parts[len(parts)-1]
					}
				}

				results[index] = ServiceStatus{
					Name:     name,
					Type:     srvType,
					HttpPort: httpPort,
					GrpcPort: grpcPort,
					Status:   status,
					PingTime: pingTime.Milliseconds(),
					Error:    errMsg,
				}
			}(i, s.Name, s.Type, s.HttpAddr, s.GrpcAddr)
		}

		wg.Wait()
		c.JSON(http.StatusOK, results)
	}
}
