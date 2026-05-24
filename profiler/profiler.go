package profiler

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

type ZProfiler struct {
	mu        sync.RWMutex
	stats     map[string]*CommandStats
	startTime time.Time
}

type CommandStats struct {
	RequestCount       atomic.Int64
	TotalExecutionTime atomic.Int64 // Store nanoseconds
	LastExecutions     []time.Duration
	LastRequestTime    atomic.Int64 // Store nanoseconds
	PendingCount       atomic.Int64
}

type PProfInfo struct {
	Goroutines int    `json:"goroutines"`
	HeapAlloc  uint64 `json:"heapAlloc"`
	HeapSys    uint64 `json:"heapSys"`
	HeapIdle   uint64 `json:"heapIdle"`
	HeapInuse  uint64 `json:"heapInuse"`
}

type CommandStatsSnapshot struct {
	RequestCount       int64         `json:"requestCount"`
	TotalExecutionTime time.Duration `json:"totalExecutionTime"`
	LastRequestTime    time.Time     `json:"lastRequestTime"`
	PendingCount       int64         `json:"pendingCount"`
	LastExecutionCount int           `json:"lastExecutionCount"`
	P50                time.Duration `json:"p50"`
	P90                time.Duration `json:"p90"`
	P99                time.Duration `json:"p99"`
}

var globalProfiler *ZProfiler
var once sync.Once

func Init() {
	once.Do(func() {
		globalProfiler = &ZProfiler{
			stats:     make(map[string]*CommandStats),
			startTime: time.Now(),
		}
	})
}

// ensureInit ensures profiler is initialized
func ensureInit() {
	if globalProfiler == nil {
		Init()
	}
}

func recordExecution(command string, duration time.Duration) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "PROFILER PANIC in recordExecution: %v - Command: %s\n", r, command)
		}
	}()

	ensureInit()

	if command == "" {
		return // Skip empty commands
	}

	stats, exists := globalProfiler.stats[command]
	if !exists {
		stats = &CommandStats{
			LastExecutions: make([]time.Duration, 0, 100),
		}
		globalProfiler.stats[command] = stats
	}

	stats.RequestCount.Add(1)
	stats.TotalExecutionTime.Add(duration.Nanoseconds())
	stats.LastRequestTime.Store(time.Now().UnixNano())
	stats.PendingCount.Add(-1)

	// Safely append to LastExecutions
	if stats.LastExecutions != nil {
		stats.LastExecutions = append(stats.LastExecutions, duration)
		if len(stats.LastExecutions) > 100 {
			stats.LastExecutions = stats.LastExecutions[1:]
		}
	}
}

// GetStats returns a copy of all command stats
func GetStats() map[string]*CommandStats {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "PROFILER PANIC in GetStats: %v\n", r)
		}
	}()

	ensureInit()

	globalProfiler.mu.RLock()
	defer globalProfiler.mu.RUnlock()

	result := make(map[string]*CommandStats)
	for k, v := range globalProfiler.stats {
		if v == nil {
			continue // Skip nil entries
		}

		result[k] = &CommandStats{
			RequestCount:       atomic.Int64{},
			TotalExecutionTime: atomic.Int64{},
			LastExecutions:     append([]time.Duration{}, v.LastExecutions...),
			LastRequestTime:    atomic.Int64{},
			PendingCount:       atomic.Int64{},
		}
		result[k].RequestCount.Store(v.RequestCount.Load())
		result[k].TotalExecutionTime.Store(v.TotalExecutionTime.Load())
		result[k].LastRequestTime.Store(v.LastRequestTime.Load())
		result[k].PendingCount.Store(v.PendingCount.Load())
	}
	return result
}

// GetRequestsPerSecond calculates requests per second for a specific command
func GetRequestsPerSecond(command string) float64 {
	ensureInit()

	globalProfiler.mu.RLock()
	stats, exists := globalProfiler.stats[command]
	globalProfiler.mu.RUnlock()

	if !exists || stats.RequestCount.Load() == 0 {
		return 0
	}

	elapsed := time.Since(globalProfiler.startTime).Seconds()
	if elapsed == 0 {
		return 0
	}

	return float64(stats.RequestCount.Load()) / elapsed
}

// GetAverageExecutionTime calculates average execution time for the most recent runs of a command
func GetAverageExecutionTime(command string) time.Duration {
	ensureInit()

	globalProfiler.mu.RLock()
	defer globalProfiler.mu.RUnlock()

	stats, exists := globalProfiler.stats[command]
	if !exists || len(stats.LastExecutions) == 0 {
		return 0
	}

	var total time.Duration
	for _, d := range stats.LastExecutions {
		total += d
	}

	return total / time.Duration(len(stats.LastExecutions))
}

// calculatePercentiles calculates P50, P90, and P99 from a slice of durations
func calculatePercentiles(durations []time.Duration) (p50, p90, p99 time.Duration) {
	if len(durations) == 0 {
		return 0, 0, 0
	}

	// Create a copy to sort
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)

	// Sort durations using standard Go sort package for efficiency
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	n := len(sorted)
	p50 = sorted[n*50/100]
	p90 = sorted[n*90/100]
	p99 = sorted[n*99/100]

	// Handle edge case for small slices
	if n > 0 {
		if n*99/100 >= n {
			p99 = sorted[n-1]
		}
		if n*90/100 >= n {
			p90 = sorted[n-1]
		}
	}

	return p50, p90, p99
}

// GetCommandStats returns stats for a specific command
func GetCommandStats(command string) (*CommandStats, bool) {
	ensureInit()

	globalProfiler.mu.RLock()
	stats, exists := globalProfiler.stats[command]
	globalProfiler.mu.RUnlock()

	if !exists {
		return nil, false
	}

	globalProfiler.mu.RLock()
	defer globalProfiler.mu.RUnlock()

	result := &CommandStats{
		RequestCount:       atomic.Int64{},
		TotalExecutionTime: atomic.Int64{},
		LastExecutions:     append([]time.Duration{}, stats.LastExecutions...),
		LastRequestTime:    atomic.Int64{},
		PendingCount:       atomic.Int64{},
	}
	result.RequestCount.Store(stats.RequestCount.Load())
	result.TotalExecutionTime.Store(stats.TotalExecutionTime.Load())
	result.LastRequestTime.Store(stats.LastRequestTime.Load())
	result.PendingCount.Store(stats.PendingCount.Load())
	return result, true
}

// TrackExecution tracks a command execution and updates pending count
func TrackExecution(command string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "PROFILER PANIC in TrackExecution: %v - Command: %s\n", r, command)
		}
	}()

	if fn == nil {
		return // Skip nil functions
	}

	ensureInit()

	globalProfiler.mu.Lock()
	stats, exists := globalProfiler.stats[command]
	if !exists {
		stats = &CommandStats{
			LastExecutions: make([]time.Duration, 0, 10),
		}
		globalProfiler.stats[command] = stats
	}

	stats.PendingCount.Add(1) // Increase pending count when execution starts
	globalProfiler.mu.Unlock()

	start := time.Now()

	// Execute function with panic recovery
	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "PANIC in tracked function: %v - Command: %s\n", r, command)
			}
		}()
		fn()
	}()

	duration := time.Since(start)

	globalProfiler.mu.Lock()
	recordExecution(command, duration)
	globalProfiler.mu.Unlock()
}

// TrackExecutionWithReturn tracks a command execution with return value and updates pending count
func TrackExecutionWithReturn(command string, fn func() (any, error)) (any, error) {
	ensureInit()

	globalProfiler.mu.Lock()
	stats, exists := globalProfiler.stats[command]
	if !exists {
		stats = &CommandStats{
			LastExecutions: make([]time.Duration, 0, 10),
		}
		globalProfiler.stats[command] = stats
	}

	stats.PendingCount.Add(1) // Increase pending count when execution starts
	globalProfiler.mu.Unlock()

	start := time.Now()
	result, err := fn()
	duration := time.Since(start)

	globalProfiler.mu.Lock()
	recordExecution(command, duration)
	globalProfiler.mu.Unlock()

	return result, err
}

// Reset clears all stats and resets the start time
func Reset() {
	ensureInit()

	globalProfiler.mu.Lock()
	defer globalProfiler.mu.Unlock()

	globalProfiler.stats = make(map[string]*CommandStats)
	globalProfiler.startTime = time.Now()
}

// GetStartTime returns the start time of the profiler
func GetStartTime() time.Time {
	ensureInit()

	globalProfiler.mu.RLock()
	defer globalProfiler.mu.RUnlock()
	return globalProfiler.startTime
}

// GetPProfInfo returns current pprof and memory information
func GetPProfInfo() PProfInfo {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return PProfInfo{
		Goroutines: runtime.NumGoroutine(),
		HeapAlloc:  m.HeapAlloc,
		HeapSys:    m.HeapSys,
		HeapIdle:   m.HeapIdle,
		HeapInuse:  m.HeapInuse,
	}
}

// GetStatsLightweight returns basic stats without deep copying for better performance
func GetStatsLightweight() map[string]CommandStatsSnapshot {
	ensureInit()

	globalProfiler.mu.RLock()
	defer globalProfiler.mu.RUnlock()

	result := make(map[string]CommandStatsSnapshot, len(globalProfiler.stats))
	for k, v := range globalProfiler.stats {
		result[k] = CommandStatsSnapshot{
			RequestCount:       v.RequestCount.Load(),
			TotalExecutionTime: time.Duration(v.TotalExecutionTime.Load()),
			LastRequestTime:    time.Unix(0, v.LastRequestTime.Load()),
			PendingCount:       v.PendingCount.Load(),
			LastExecutionCount: len(v.LastExecutions),
		}

		p50, p90, p99 := calculatePercentiles(v.LastExecutions)
		snapshot := result[k]
		snapshot.P50 = p50
		snapshot.P90 = p90
		snapshot.P99 = p99
		result[k] = snapshot
	}
	return result
}

// Middleware is a Gin middleware to automatically profile requests for a service
func Middleware(serviceName string) gin.HandlerFunc {
	ensureInit()

	return func(c *gin.Context) {
		start := time.Now()
		
		// Skip logs/profiler routes to avoid polluting stats
		path := c.Request.URL.Path
		if path == "/health" || path == "/logs" || path == "/logs/stream" || path == "/debug/profiler" {
			c.Next()
			return
		}

		command := fmt.Sprintf("%s:%s %s", serviceName, c.Request.Method, c.FullPath())
		if c.FullPath() == "" {
			command = fmt.Sprintf("%s:%s %s", serviceName, c.Request.Method, path)
		}

		globalProfiler.mu.Lock()
		stats, exists := globalProfiler.stats[command]
		if !exists {
			stats = &CommandStats{
				LastExecutions: make([]time.Duration, 0, 100),
			}
			globalProfiler.stats[command] = stats
		}
		stats.PendingCount.Add(1)
		globalProfiler.mu.Unlock()

		defer func() {
			duration := time.Since(start)
			globalProfiler.mu.Lock()
			recordExecution(command, duration)
			globalProfiler.mu.Unlock()
		}()

		c.Next()
	}
}

// Handler returns the current profiling stats and memory info as JSON
func Handler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"startTime": GetStartTime(),
		"pprof":     GetPProfInfo(),
		"commands":  GetStatsLightweight(),
	})
}
