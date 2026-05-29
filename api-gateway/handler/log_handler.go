package handler

import (
	"bufio"
	_ "embed"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"encoding/json"
	"github.com/gin-gonic/gin"
	"social-network-go/api-gateway/config"
	"social-network-go/profiler"
	"sort"
	"sync"
)

// StreamLogs handles Server-Sent Events for streaming service logs
func StreamLogs(c *gin.Context) {
	service := c.Query("service")
	if service == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "service query param required"})
		return
	}

	validServices := map[string]bool{
		"api-gateway":          true,
		"auth-service":         true,
		"user-service":         true,
		"post-service":         true,
		"chat-service":         true,
		"notification-service": true,
		"file-service":         true,
		"ai-service":           true,
		"admin-service":        true,
	}
	if !validServices[service] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid service name"})
		return
	}

	logFile := "logs/" + service + ".log"

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")

	file, err := os.Open(logFile)
	if err != nil {
		c.SSEvent("info", "Log file not created yet. Waiting for service updates...")
		c.Writer.Flush()

		// Simple loop to wait for file creation
		for i := 0; i < 10; i++ {
			time.Sleep(1 * time.Second)
			file, err = os.Open(logFile)
			if err == nil {
				break
			}
		}
		if err != nil {
			c.SSEvent("error", "Log file does not exist: "+logFile)
			c.Writer.Flush()
			return
		}
	}
	defer file.Close()

	// 1. Fetch recent history (last 30KB) of the log file so the console populates instantly
	stat, err := file.Stat()
	var size int64 = 0
	if err == nil {
		size = stat.Size()
		if size > 30000 {
			_, _ = file.Seek(size-30000, io.SeekStart)
		}
	}

	reader := bufio.NewReader(file)

	// Discard the very first partial line if we seeked into the middle of the file
	if size > 30000 {
		_, _ = reader.ReadString('\n')
	}
	// Stream all existing log history lines first
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		c.SSEvent("log", strings.TrimSuffix(line, "\n"))
	}
	c.Writer.Flush()

	// 2. Continue with the real-time tailing loop for any new writes
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					if err == io.EOF {
						break // reached current end of file, wait for new writes
					}
					return
				}
				c.SSEvent("log", strings.TrimSuffix(line, "\n"))
				c.Writer.Flush()
			}
		}
	}
}

// LogDashboard serves the glassmorphic real-time dark log dashboard html
func LogDashboard(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(logDashboardHTML))
}

//go:embed log_dashboard.html
var logDashboardHTML string

// ProfilerDashboard serves the glassmorphic performance profiler dashboard html
func ProfilerDashboard(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(profilerDashboardHTML))
}

//go:embed profiler_dashboard.html
var profilerDashboardHTML string

// ProfilerAggregatorHandler gathers stats in parallel from all active services
func ProfilerAggregatorHandler(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		services := map[string]string{
			"auth-service":         cfg.AuthHttpAddr,
			"user-service":         cfg.UserHttpAddr,
			"post-service":         cfg.PostHttpAddr,
			"chat-service":         cfg.ChatHttpAddr,
			"notification-service": cfg.NotificationHttpAddr,
			"file-service":         cfg.FileHttpAddr,
			"admin-service":        cfg.AdminHttpAddr,
		}

		type ServiceData struct {
			StartTime time.Time   `json:"startTime"`
			Pprof     interface{} `json:"pprof"`
			Commands  interface{} `json:"commands"`
			Online    bool        `json:"online"`
		}

		result := make(map[string]ServiceData)

		// Get Gateway stats
		result["api-gateway"] = ServiceData{
			StartTime: profiler.GetStartTime(),
			Pprof:     profiler.GetPProfInfo(),
			Commands:  profiler.GetStatsLightweight(),
			Online:    true,
		}

		var wg sync.WaitGroup
		var mu sync.Mutex
		client := &http.Client{
			Timeout: 200 * time.Millisecond,
		}

		for name, addr := range services {
			wg.Add(1)
			go func(name, addr string) {
				defer wg.Done()

				resp, err := client.Get(addr + "/debug/profiler")
				if err != nil {
					mu.Lock()
					result[name] = ServiceData{Online: false}
					mu.Unlock()
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					mu.Lock()
					result[name] = ServiceData{Online: false}
					mu.Unlock()
					return
				}

				var data struct {
					StartTime time.Time   `json:"startTime"`
					Pprof     interface{} `json:"pprof"`
					Commands  interface{} `json:"commands"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
					mu.Lock()
					result[name] = ServiceData{Online: false}
					mu.Unlock()
					return
				}

				mu.Lock()
				result[name] = ServiceData{
					StartTime: data.StartTime,
					Pprof:     data.Pprof,
					Commands:  data.Commands,
					Online:    true,
				}
				mu.Unlock()
			}(name, addr)
		}

		wg.Wait()
		c.JSON(http.StatusOK, result)
	}
}

// LogSearchResult represents a matched log line
type LogSearchResult struct {
	Service   string `json:"service"`
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	RawLine   string `json:"rawLine"`
}

// SearchLogs searches all microservice log files for a specific request_id
func SearchLogs(c *gin.Context) {
	reqID := c.Query("request_id")
	if reqID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request_id query param required"})
		return
	}

	validServices := []string{
		"api-gateway",
		"auth-service",
		"user-service",
		"post-service",
		"chat-service",
		"notification-service",
		"file-service",
		"ai-service",
		"admin-service",
	}

	results := []LogSearchResult{}
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, service := range validServices {
		wg.Add(1)
		go func(srvName string) {
			defer wg.Done()
			logFile := "logs/" + srvName + ".log"

			file, err := os.Open(logFile)
			if err != nil {
				return // Ignore if file doesn't exist yet
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			const maxCapacity = 1024 * 1024 // 1MB buffer limit
			buf := make([]byte, maxCapacity)
			scanner.Buffer(buf, maxCapacity)

			for scanner.Scan() {
				line := scanner.Text()
				if strings.Contains(line, reqID) {
					timestamp := ""
					level := ""
					msg := line

					// Extract timestamp and level: YYYY/MM/DD HH:MM:SS [LEVEL] message...
					parts := strings.SplitN(line, " [", 2)
					if len(parts) == 2 {
						timestamp = parts[0]
						subparts := strings.SplitN(parts[1], "] ", 2)
						if len(subparts) == 2 {
							level = subparts[0]
							msg = subparts[1]
						}
					}

					mu.Lock()
					results = append(results, LogSearchResult{
						Service:   srvName,
						Timestamp: timestamp,
						Level:     level,
						Message:   msg,
						RawLine:   line,
					})
					mu.Unlock()
				}
			}
		}(service)
	}

	wg.Wait()

	// Sort results chronologically by timestamp
	sort.Slice(results, func(i, j int) bool {
		if results[i].Timestamp == results[j].Timestamp {
			return results[i].Service < results[j].Service
		}
		return results[i].Timestamp < results[j].Timestamp
	})

	c.JSON(http.StatusOK, results)
}
