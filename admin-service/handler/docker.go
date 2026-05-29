package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"social-network-go/logger"
)

type ContainerInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	State   string `json:"state"`
	Status  string `json:"status"`
	Created int64  `json:"created"`
}

type ContainerStats struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	CPUPercent  float64 `json:"cpu_percent"`
	MemoryUsage int64   `json:"memory_usage_bytes"`
	MemoryLimit int64   `json:"memory_limit_bytes"`
	MemoryPerc  float64 `json:"memory_percent"`
	NetworkRx   int64   `json:"network_rx_bytes"`
	NetworkTx   int64   `json:"network_tx_bytes"`
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow CORS for local dev
	},
}

// GetContainers returns a list of all containers
func (h *AdminHandler) GetContainers(c *gin.Context) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		logger.Error("Failed to create Docker client: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to Docker daemon"})
		return
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		logger.Error("Failed to list Docker containers: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch containers from Docker"})
		return
	}

	var list []ContainerInfo
	for _, doc := range containers {
		name := ""
		if len(doc.Names) > 0 {
			name = doc.Names[0]
			name = strings.TrimPrefix(name, "/")
		} else {
			name = doc.ID[:12]
		}

		list = append(list, ContainerInfo{
			ID:      doc.ID,
			Name:    name,
			Image:   doc.Image,
			State:   doc.State,
			Status:  doc.Status,
			Created: doc.Created,
		})
	}

	sendSuccess(c, list)
}

// StreamContainersStats streams real-time CPU/RAM stats via WebSocket
func (h *AdminHandler) StreamContainersStats(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error("Failed to upgrade HTTP to WebSocket: %v", err)
		return
	}
	defer conn.Close()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		logger.Error("Failed to create Docker client: %v", err)
		_ = conn.WriteJSON(gin.H{"error": "Failed to connect to Docker daemon"})
		return
	}
	defer cli.Close()

	logger.Info("Client connected to Docker stats WebSocket stream")

	// Goroutine to check for client disconnect
	stopChan := make(chan struct{})
	go func() {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				close(stopChan)
				return
			}
		}
	}()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stopChan:
			logger.Info("Client disconnected from Docker stats WebSocket stream")
			return
		case <-ticker.C:
			stats, err := getDockerStats(cli)
			if err != nil {
				logger.Error("Failed to get Docker stats: %v", err)
				_ = conn.WriteJSON(gin.H{"error": err.Error()})
				continue
			}

			payload := gin.H{
				"containers": stats,
				"timestamp":  time.Now().Format(time.RFC3339),
			}

			if err := conn.WriteJSON(payload); err != nil {
				logger.Error("Failed to write JSON to WebSocket: %v", err)
				return
			}
		}
	}
}

func getDockerStats(cli *client.Client) ([]ContainerStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	containers, err := cli.ContainerList(ctx, container.ListOptions{All: false})
	if err != nil {
		return nil, err
	}

	var wg sync.WaitGroup
	resultsChan := make(chan ContainerStats, len(containers))

	for _, c := range containers {
		wg.Add(1)
		go func(c container.Summary) {
			defer wg.Done()

			reqCtx, reqCancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer reqCancel()

			statsResp, err := cli.ContainerStats(reqCtx, c.ID, false)
			if err != nil {
				return
			}
			defer statsResp.Body.Close()

			var stats container.StatsResponse
			if err := json.NewDecoder(statsResp.Body).Decode(&stats); err != nil {
				return
			}

			// Calculate CPU percent
			cpuPercent := 0.0
			cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage) - float64(stats.PreCPUStats.CPUUsage.TotalUsage)
			systemDelta := float64(stats.CPUStats.SystemUsage) - float64(stats.PreCPUStats.SystemUsage)
			onlineCPUs := float64(stats.CPUStats.OnlineCPUs)
			if onlineCPUs == 0 {
				onlineCPUs = float64(len(stats.CPUStats.CPUUsage.PercpuUsage))
			}
			if systemDelta > 0.0 && cpuDelta > 0.0 {
				cpuPercent = (cpuDelta / systemDelta) * onlineCPUs * 100.0
			}

			// Calculate Memory percent
			memUsage := float64(stats.MemoryStats.Usage)
			memLimit := float64(stats.MemoryStats.Limit)
			memPercent := 0.0
			if memLimit > 0 {
				memPercent = (memUsage / memLimit) * 100.0
			}

			// Calculate Network Rx/Tx
			var rx, tx int64
			for _, netStats := range stats.Networks {
				rx += int64(netStats.RxBytes)
				tx += int64(netStats.TxBytes)
			}

			name := ""
			if len(c.Names) > 0 {
				name = c.Names[0]
				name = strings.TrimPrefix(name, "/")
			} else {
				name = c.ID[:12]
			}

			resultsChan <- ContainerStats{
				ID:          c.ID,
				Name:        name,
				CPUPercent:  cpuPercent,
				MemoryUsage: int64(memUsage),
				MemoryLimit: int64(memLimit),
				MemoryPerc:  memPercent,
				NetworkRx:   rx,
				NetworkTx:   tx,
			}
		}(c)
	}

	wg.Wait()
	close(resultsChan)

	var results []ContainerStats
	for res := range resultsChan {
		results = append(results, res)
	}

	return results, nil
}

// StartContainer starts a stopped container
func (h *AdminHandler) StartContainer(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Container ID is required"})
		return
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		logger.Error("Failed to create Docker client: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to Docker daemon"})
		return
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	if err := cli.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		logger.Error("Failed to start container %s: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sendSuccess(c, gin.H{"status": "started"})
}

// StopContainer stops a running container
func (h *AdminHandler) StopContainer(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Container ID is required"})
		return
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		logger.Error("Failed to create Docker client: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to Docker daemon"})
		return
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	if err := cli.ContainerStop(ctx, id, container.StopOptions{}); err != nil {
		logger.Error("Failed to stop container %s: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sendSuccess(c, gin.H{"status": "stopped"})
}

// RestartContainer restarts a container
func (h *AdminHandler) RestartContainer(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Container ID is required"})
		return
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		logger.Error("Failed to create Docker client: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect to Docker daemon"})
		return
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	if err := cli.ContainerRestart(ctx, id, container.StopOptions{}); err != nil {
		logger.Error("Failed to restart container %s: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sendSuccess(c, gin.H{"status": "restarted"})
}

type wsLogWriter struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (w *wsLogWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	lines := strings.Split(string(p), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			err = w.conn.WriteJSON(gin.H{"log": line})
			if err != nil {
				return 0, err
			}
		}
	}
	return len(p), nil
}

// StreamContainerLogs streams real-time container logs via WebSocket
func (h *AdminHandler) StreamContainerLogs(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Container ID is required"})
		return
	}

	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error("Failed to upgrade HTTP to WebSocket for container logs: %v", err)
		return
	}
	defer conn.Close()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		logger.Error("Failed to create Docker client: %v", err)
		_ = conn.WriteJSON(gin.H{"error": "Failed to connect to Docker daemon"})
		return
	}
	defer cli.Close()

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       "200",
		Timestamps: true,
	}

	reader, err := cli.ContainerLogs(ctx, id, options)
	if err != nil {
		logger.Error("Failed to get container logs: %v", err)
		_ = conn.WriteJSON(gin.H{"error": err.Error()})
		return
	}
	defer reader.Close()

	stopChan := make(chan struct{})
	var closeOnce sync.Once
	safeClose := func() {
		closeOnce.Do(func() {
			close(stopChan)
		})
	}

	go func() {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				safeClose()
				return
			}
		}
	}()

	writer := &wsLogWriter{conn: conn}

	go func() {
		_, _ = stdcopy.StdCopy(writer, writer, reader)
		safeClose()
	}()

	<-stopChan
}
