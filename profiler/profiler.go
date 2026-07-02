package profiler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

const AdminTokenHeader = "X-Profiler-Token"

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

func (c *CommandStats) MarshalJSON() ([]byte, error) {
	var executions []time.Duration
	if c.LastExecutions != nil {
		executions = c.LastExecutions
	} else {
		executions = []time.Duration{}
	}

	return json.Marshal(struct {
		RequestCount       int64           `json:"requestCount"`
		TotalExecutionTime int64           `json:"totalExecutionTime"`
		LastExecutions     []time.Duration `json:"lastExecutions"`
		LastRequestTime    int64           `json:"lastRequestTime"`
		PendingCount       int64           `json:"pendingCount"`
	}{
		RequestCount:       c.RequestCount.Load(),
		TotalExecutionTime: c.TotalExecutionTime.Load(),
		LastExecutions:     executions,
		LastRequestTime:    c.LastRequestTime.Load(),
		PendingCount:       c.PendingCount.Load(),
	})
}

func (c *CommandStats) UnmarshalJSON(data []byte) error {
	var aux struct {
		RequestCount       int64           `json:"requestCount"`
		TotalExecutionTime int64           `json:"totalExecutionTime"`
		LastExecutions     []time.Duration `json:"lastExecutions"`
		LastRequestTime    int64           `json:"lastRequestTime"`
		PendingCount       int64           `json:"pendingCount"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	c.RequestCount.Store(aux.RequestCount)
	c.TotalExecutionTime.Store(aux.TotalExecutionTime)
	if aux.LastExecutions == nil {
		c.LastExecutions = make([]time.Duration, 0, 100)
	} else {
		c.LastExecutions = aux.LastExecutions
	}
	c.LastRequestTime.Store(aux.LastRequestTime)
	c.PendingCount.Store(aux.PendingCount)
	return nil
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

type RouteStatsSnapshot struct {
	Command string `json:"command"`
	Service string `json:"service"`
	Method  string `json:"method"`
	Path    string `json:"path"`
	CommandStatsSnapshot
}

var globalProfiler *ZProfiler
var once sync.Once

func Init() {
	once.Do(func() {
		globalProfiler = &ZProfiler{
			stats:     make(map[string]*CommandStats),
			startTime: time.Now(),
		}
		loadStats()

		// Start periodic save
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			for range ticker.C {
				saveStats()
			}
		}()
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

	globalProfiler.mu.Lock()
	defer globalProfiler.mu.Unlock()

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

func recordEvent(command string) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "PROFILER PANIC in recordEvent: %v - Command: %s\n", r, command)
		}
	}()

	ensureInit()

	if command == "" {
		return
	}

	globalProfiler.mu.Lock()
	defer globalProfiler.mu.Unlock()

	stats, exists := globalProfiler.stats[command]
	if !exists {
		stats = &CommandStats{
			LastExecutions: make([]time.Duration, 0, 100),
		}
		globalProfiler.stats[command] = stats
	}

	stats.RequestCount.Add(1)
	stats.LastRequestTime.Store(time.Now().UnixNano())
	stats.LastExecutions = append(stats.LastExecutions, 0)
	if len(stats.LastExecutions) > 100 {
		stats.LastExecutions = stats.LastExecutions[1:]
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

	recordExecution(command, duration)
}

// TrackExecutionWithReturn tracks a command execution with return value and updates pending count
func TrackExecutionWithReturn(command string, fn func() (any, error)) (any, error) {
	if fn == nil {
		return nil, nil
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
	defer func() {
		recordExecution(command, time.Since(start))
	}()

	return fn()
}

// TrackResult tracks a typed operation and returns its result without forcing callers to type assert.
func TrackResult[T any](command string, fn func() (T, error)) (T, error) {
	var zero T
	if fn == nil {
		return zero, nil
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
	stats.PendingCount.Add(1)
	globalProfiler.mu.Unlock()

	start := time.Now()
	defer func() {
		recordExecution(command, time.Since(start))
	}()

	return fn()
}

// TrackEvent records a count-only profiler event with no pending duration.
func TrackEvent(command string) {
	recordEvent(command)
}

// TrackCacheLookup records hit/miss/error counters for a cache lookup prefix.
func TrackCacheLookup(command string, hit bool, err error) {
	if err != nil {
		TrackEvent(command + ".error")
		return
	}
	if hit {
		TrackEvent(command + ".hit")
		return
	}
	TrackEvent(command + ".miss")
}

// Reset clears all stats and resets the start time
func Reset() {
	ensureInit()

	globalProfiler.mu.Lock()
	globalProfiler.stats = make(map[string]*CommandStats)
	globalProfiler.startTime = time.Now()
	globalProfiler.mu.Unlock()

	saveStats()
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

// GetRouteStatsLightweight returns profiler stats as structured rows for dashboard consumers.
func GetRouteStatsLightweight() []RouteStatsSnapshot {
	stats := GetStatsLightweight()
	result := make([]RouteStatsSnapshot, 0, len(stats))

	for command, snapshot := range stats {
		if isLegacyWildcardCommand(command) {
			continue
		}
		service, method, path := parseProfiledCommand(command)
		result = append(result, RouteStatsSnapshot{
			Command:              command,
			Service:              service,
			Method:               method,
			Path:                 path,
			CommandStatsSnapshot: snapshot,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Service != result[j].Service {
			return result[i].Service < result[j].Service
		}
		if result[i].Path != result[j].Path {
			return result[i].Path < result[j].Path
		}
		return result[i].Method < result[j].Method
	})

	return result
}

func parseProfiledCommand(command string) (service, method, path string) {
	method = "ANY"
	path = command

	if idx := strings.Index(command, ":"); idx >= 0 {
		service = command[:idx]
		command = command[idx+1:]
		path = command
	}

	if idx := strings.Index(command, " "); idx >= 0 {
		method = command[:idx]
		path = command[idx+1:]
	}

	return service, method, path
}

func isLegacyWildcardCommand(command string) bool {
	_, _, path := parseProfiledCommand(command)
	return strings.Contains(path, "*any")
}

// normalizePath converts dynamic path parameters to standard placeholders (e.g. :username, :id)
func normalizePath(path string) string {
	path = strings.Split(path, "?")[0]
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		return "/"
	}

	// 1. Specific static prefix rules for maximum precision
	if strings.HasPrefix(path, "/v1/users/of-user/") {
		return "/v1/users/of-user/:username"
	}
	if strings.HasPrefix(path, "/v1/posts/of-user/") {
		return "/v1/posts/of-user/:username"
	}
	if strings.HasPrefix(path, "/v1/posts/files/") {
		return "/v1/posts/files/:username"
	}
	if strings.HasPrefix(path, "/v1/posts/like/") {
		return "/v1/posts/like/:postId"
	}
	if strings.HasPrefix(path, "/v1/friends/mutual-friends/") {
		return "/v1/friends/mutual-friends/:username"
	}
	if strings.HasPrefix(path, "/v1/friend-request/send/") {
		return "/v1/friend-request/send/:username"
	}
	if strings.HasPrefix(path, "/v1/friend-request/accept/") {
		return "/v1/friend-request/accept/:username"
	}
	if strings.HasPrefix(path, "/v1/friend-request/delete/") {
		return "/v1/friend-request/delete/:username"
	}

	// 2. Generic segmentation rules
	segments := strings.Split(path, "/")

	// If /v1/users/<username>
	if len(segments) == 4 && segments[1] == "v1" && segments[2] == "users" {
		sub := segments[3]
		if !strings.HasPrefix(sub, "update-") {
			segments[3] = ":username"
		}
	}

	// If /v1/friends/<username>
	if len(segments) == 4 && segments[1] == "v1" && segments[2] == "friends" {
		if segments[3] != "suggested" {
			segments[3] = ":username"
		}
	}

	// If /v1/blocks/<username>
	if len(segments) == 4 && segments[1] == "v1" && segments[2] == "blocks" {
		segments[3] = ":username"
	}

	// If /v1/stories/<id>
	if len(segments) == 4 && segments[1] == "v1" && segments[2] == "stories" {
		if segments[3] != "feed" {
			segments[3] = ":id"
		}
	}

	// If /v1/files/<id> or /v1/files/<id>/presigned
	if len(segments) >= 4 && segments[1] == "v1" && segments[2] == "files" {
		if segments[3] != "upload" && segments[3] != "upload-multiple" && segments[3] != "delete-multiple" {
			if len(segments) >= 5 && segments[4] == "presigned" {
				return "/v1/files/:id/presigned"
			}
			segments[3] = ":id"
		}
	}

	// If /v1/admin/containers/<id>/...
	if len(segments) >= 5 && segments[1] == "v1" && segments[2] == "admin" && segments[3] == "containers" {
		segments[4] = ":id"
	}

	// 3. Fallback heuristic detection for IDs/UUIDs
	for i, seg := range segments {
		if seg == "" || strings.HasPrefix(seg, ":") {
			continue
		}
		if isNumeric(seg) {
			segments[i] = ":id"
		} else if isHexOrUUID(seg) || isAlphanumericID(seg) {
			segments[i] = ":id"
		}
	}

	return strings.Join(segments, "/")
}

func isNumeric(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func isHexOrUUID(s string) bool {
	if len(s) == 24 {
		for _, c := range s {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
		return true
	}
	if len(s) == 36 && strings.Count(s, "-") == 4 {
		return true
	}
	return false
}

func isAlphanumericID(s string) bool {
	if len(s) < 5 {
		return false
	}
	hasLetter := false
	hasDigit := false
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			hasLetter = true
		} else if c >= '0' && c <= '9' {
			hasDigit = true
		} else if c != '_' && c != '-' {
			return false
		}
	}
	return hasLetter && hasDigit
}

// Middleware is a Gin middleware to automatically profile requests for a service
func Middleware(serviceName string) gin.HandlerFunc {
	ensureInit()

	return func(c *gin.Context) {
		start := time.Now()

		// Skip logs/profiler routes to avoid polluting stats
		path := c.Request.URL.Path
		if path == "/health" || path == "/logs" || path == "/logs/stream" || path == "/debug/profiler" ||
			path == "/debug/profiler/reset" || path == "/profiler" || path == "/containers" {
			c.Next()
			return
		}

		isWebSocket := strings.ToLower(c.GetHeader("Upgrade")) == "websocket" || strings.Contains(path, "/ws") || strings.Contains(path, "/stream")

		// Determine the matching route path template
		var routePath string
		if c.FullPath() == "" || strings.Contains(c.FullPath(), "*any") {
			routePath = normalizePath(path)
		} else {
			routePath = normalizePath(c.FullPath())
		}

		var command string
		if isWebSocket {
			command = fmt.Sprintf("%s:WS %s", serviceName, routePath)
		} else {
			command = fmt.Sprintf("%s:%s %s", serviceName, c.Request.Method, routePath)
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
			recordExecution(command, duration)
		}()

		c.Next()
	}
}

// IsEnabled returns true if the profiler endpoints should be enabled
func IsEnabled() bool {
	enabledEnv := os.Getenv("ENABLE_PROFILER_ENDPOINT")
	if enabledEnv == "true" {
		return true
	}
	if enabledEnv == "false" {
		return false
	}
	// Default to true on non-production/non-staging, false on production/staging
	appEnv := strings.ToLower(os.Getenv("APP_ENV"))
	return appEnv != "production" && appEnv != "prod" && appEnv != "staging"
}

func IsAuthorized(c *gin.Context) bool {
	adminToken := os.Getenv("PROFILER_ADMIN_TOKEN")
	if adminToken == "" {
		return true
	}
	if c.GetHeader(AdminTokenHeader) == adminToken {
		return true
	}
	authHeader := c.GetHeader("Authorization")
	const bearerPrefix = "Bearer "
	return strings.HasPrefix(authHeader, bearerPrefix) && strings.TrimPrefix(authHeader, bearerPrefix) == adminToken
}

// Handler returns the current profiling stats and memory info as JSON
func Handler(c *gin.Context) {
	if !IsEnabled() {
		c.JSON(http.StatusNotFound, gin.H{"error": "Profiler endpoint is disabled"})
		return
	}
	if !IsAuthorized(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Profiler endpoint is unauthorized"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"startTime": GetStartTime(),
		"pprof":     GetPProfInfo(),
		"commands":  GetStatsLightweight(),
		"routes":    GetRouteStatsLightweight(),
	})
}

// EndpointGuard is a Gin middleware that returns 404 if the profiler is disabled
func EndpointGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !IsEnabled() {
			c.JSON(http.StatusNotFound, gin.H{"error": "Profiler endpoint is disabled"})
			c.Abort()
			return
		}
		if !IsAuthorized(c) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Profiler endpoint is unauthorized"})
			c.Abort()
			return
		}
		c.Next()
	}
}

type profilerPersistentData struct {
	StartTime time.Time                `json:"startTime"`
	Stats     map[string]*CommandStats `json:"stats"`
}

func getStatsFilePath() string {
	exeName := filepath.Base(os.Args[0])
	exeName = strings.TrimSuffix(exeName, ".exe")
	_ = os.MkdirAll("logs", 0755)
	return filepath.Join("logs", fmt.Sprintf("profiler_stats_%s.json", exeName))
}

func saveStats() {
	if globalProfiler == nil {
		return
	}
	globalProfiler.mu.RLock()
	persistentData := profilerPersistentData{
		StartTime: globalProfiler.startTime,
		Stats:     copyStatsLocked(globalProfiler.stats),
	}
	globalProfiler.mu.RUnlock()

	data, err := json.MarshalIndent(persistentData, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "PROFILER ERROR marshaling stats: %v\n", err)
		return
	}

	filePath := getStatsFilePath()
	err = os.WriteFile(filePath, data, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "PROFILER ERROR saving stats to %s: %v\n", filePath, err)
	}
}

func copyStatsLocked(stats map[string]*CommandStats) map[string]*CommandStats {
	copied := make(map[string]*CommandStats, len(stats))
	for command, stat := range stats {
		if stat == nil {
			continue
		}
		next := &CommandStats{
			LastExecutions: append([]time.Duration{}, stat.LastExecutions...),
		}
		next.RequestCount.Store(stat.RequestCount.Load())
		next.TotalExecutionTime.Store(stat.TotalExecutionTime.Load())
		next.LastRequestTime.Store(stat.LastRequestTime.Load())
		next.PendingCount.Store(stat.PendingCount.Load())
		copied[command] = next
	}
	return copied
}

func loadStats() {
	filePath := getStatsFilePath()
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		fmt.Fprintf(os.Stderr, "PROFILER ERROR reading stats file %s: %v\n", filePath, err)
		return
	}

	var persistentData profilerPersistentData
	if err := json.Unmarshal(data, &persistentData); err != nil {
		fmt.Fprintf(os.Stderr, "PROFILER ERROR unmarshaling stats from %s: %v\n", filePath, err)
		return
	}

	globalProfiler.mu.Lock()
	defer globalProfiler.mu.Unlock()

	if !persistentData.StartTime.IsZero() {
		globalProfiler.startTime = persistentData.StartTime
	}

	for k, v := range persistentData.Stats {
		if isLegacyWildcardCommand(k) {
			continue
		}
		if v != nil {
			if v.LastExecutions == nil {
				v.LastExecutions = make([]time.Duration, 0, 100)
			}
			globalProfiler.stats[k] = v
		}
	}
}
