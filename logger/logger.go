package logger

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
)

// Level represents log severity
type Level string

const (
	LevelInfo  Level = "INFO"
	LevelWarn  Level = "WARN"
	LevelError Level = "ERROR"
	LevelFatal Level = "FATAL"
)

// Fields represents a map of log context
type Fields map[string]interface{}

// Entry represents a log entry with fields
type Entry struct {
	fields     map[string]interface{}
	jsonFields map[string]interface{}
}

// NewEntry creates a new log entry
func NewEntry() *Entry {
	return &Entry{
		fields:     make(map[string]interface{}),
		jsonFields: make(map[string]interface{}),
	}
}

// Field adds a single key-value context field to the entry
func (e *Entry) Field(key string, value interface{}) *Entry {
	e.fields[key] = value
	return e
}

// JsonField adds a JSON key-value context field to the entry that will format nicely
func (e *Entry) JsonField(key string, value interface{}) *Entry {
	e.jsonFields[key] = value
	return e
}

// Err adds a standard error field ("error") to the entry
func (e *Entry) Err(err error) *Entry {
	if err != nil {
		e.fields["error"] = err.Error()
	}
	return e
}

// Req extracts the request body from a *gin.Context or *http.Request and adds it as context
func (e *Entry) Req(c interface{}) *Entry {
	if c == nil {
		return e
	}

	var reqBody []byte
	var readErr error

	switch r := c.(type) {
	case *gin.Context:
		if val, exists := r.Get("cached_req_body"); exists {
			if cachedStr, ok := val.(string); ok && cachedStr != "" {
				reqBody = []byte(cachedStr)
			}
		}
		if len(reqBody) == 0 && r.Request != nil && r.Request.Body != nil {
			reqBody, readErr = io.ReadAll(r.Request.Body)
			if readErr == nil {
				r.Request.Body = io.NopCloser(bytes.NewBuffer(reqBody))
			}
		}
	case *http.Request:
		if r.Body != nil {
			reqBody, readErr = io.ReadAll(r.Body)
			if readErr == nil {
				r.Body = io.NopCloser(bytes.NewBuffer(reqBody))
			}
		}
	}

	if len(reqBody) > 0 {
		e.fields["req_body"] = string(reqBody)
	}

	return e
}

// Fields adds multiple key-value context fields to the entry
func (e *Entry) Fields(fields Fields) *Entry {
	for k, v := range fields {
		e.fields[k] = v
	}
	return e
}

// Info prints an INFO level log with the chained fields
func (e *Entry) Info(format string, args ...interface{}) {
	logMessage(LevelInfo, colorBlue, fmt.Sprintf(format, args...), e.fields, e.jsonFields)
}

// Warn prints a WARN level log with the chained fields
func (e *Entry) Warn(format string, args ...interface{}) {
	logMessage(LevelWarn, colorYellow, fmt.Sprintf(format, args...), e.fields, e.jsonFields)
}

// Error prints an ERROR level log with the chained fields
func (e *Entry) Error(format string, args ...interface{}) {
	logMessage(LevelError, colorRed, fmt.Sprintf(format, args...), e.fields, e.jsonFields)
}

// Fatal prints a FATAL level log with the chained fields and exits the program
func (e *Entry) Fatal(format string, args ...interface{}) {
	logMessage(LevelFatal, colorRed, fmt.Sprintf(format, args...), e.fields, e.jsonFields)
	os.Exit(1)
}

// --- Package-level direct logging (without fields) ---

// Info logs immediately at INFO level
func Info(format string, args ...interface{}) {
	NewEntry().Info(format, args...)
}

// Warn logs immediately at WARN level
func Warn(format string, args ...interface{}) {
	NewEntry().Warn(format, args...)
}

// Error logs immediately at ERROR level
func Error(format string, args ...interface{}) {
	NewEntry().Error(format, args...)
}

// Fatal logs immediately at FATAL level and exits the program
func Fatal(format string, args ...interface{}) {
	NewEntry().Fatal(format, args...)
}

// --- Package-level entry points for chained logging ---

// WithContext creates an Entry associated with a context and extracts trace metadata
func WithContext(ctx context.Context) *Entry {
	e := NewEntry()
	if ctx != nil {
		// Tự động lấy trace_id/request_id nếu tồn tại trong context
		if traceID, ok := ctx.Value("trace_id").(string); ok {
			e.Field("trace_id", traceID)
		}
		if reqID, ok := ctx.Value("request_id").(string); ok {
			e.Field("request_id", reqID)
		}
	}
	return e
}

// Field creates an Entry and adds a single field
func Field(key string, value interface{}) *Entry {
	return NewEntry().Field(key, value)
}

// Err creates an Entry and adds a standard error field ("error")
func Err(err error) *Entry {
	return NewEntry().Err(err)
}

// ErrorField creates an Entry and adds a standard error field ("error")
func ErrorField(err error) *Entry {
	return NewEntry().Err(err)
}

// Req creates an Entry and extracts the request body
func Req(c interface{}) *Entry {
	return NewEntry().Req(c)
}

// WithFields creates an Entry and adds multiple fields
func WithFields(fields Fields) *Entry {
	return NewEntry().Fields(fields)
}

// Variables for environment and file-based logging
var (
	logFile     *os.File
	initialized bool
	logLevels   map[string]bool // levels allowed to print to console
	serviceName string
	logHttpBody bool
)

func RedactJSON(jsonStr string) string {
	if jsonStr == "" {
		return ""
	}
	var data interface{}
	err := json.Unmarshal([]byte(jsonStr), &data)
	if err != nil {
		return jsonStr // Not a valid JSON, return as is
	}
	redactValue(data)
	redactedBytes, err := json.Marshal(data)
	if err != nil {
		return jsonStr
	}
	return string(redactedBytes)
}

func redactValue(val interface{}) {
	sensitiveKeys := map[string]bool{
		"password":      true,
		"token":         true,
		"accesstoken":   true,
		"refreshtoken":  true,
		"authorization": true,
		"cookie":        true,
		"otp":           true,
		"secret":        true,
		"credential":    true,
	}

	switch m := val.(type) {
	case map[string]interface{}:
		for k, v := range m {
			lowerK := strings.ToLower(k)
			if sensitiveKeys[lowerK] {
				m[k] = "[REDACTED]"
			} else {
				redactValue(v)
			}
		}
	case []interface{}:
		for _, item := range m {
			redactValue(item)
		}
	}
}

func RedactQuery(query string) string {
	if query == "" {
		return ""
	}
	parts := strings.Split(query, "&")
	sensitiveKeys := map[string]bool{
		"password":      true,
		"token":         true,
		"accesstoken":   true,
		"refreshtoken":  true,
		"authorization": true,
		"cookie":        true,
		"otp":           true,
		"secret":        true,
		"credential":    true,
	}
	for i, part := range parts {
		pair := strings.SplitN(part, "=", 2)
		if len(pair) == 2 {
			lowerK := strings.ToLower(pair[0])
			if sensitiveKeys[lowerK] {
				parts[i] = pair[0] + "=[REDACTED]"
			}
		}
	}
	return strings.Join(parts, "&")
}

func loadEnvFile(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			if len(val) > 1 && val[0] == '"' && val[len(val)-1] == '"' {
				val = val[1 : len(val)-1]
			}
			if len(val) > 1 && val[0] == '\'' && val[len(val)-1] == '\'' {
				val = val[1 : len(val)-1]
			}
			if os.Getenv(key) == "" {
				_ = os.Setenv(key, val)
			}
		}
	}
}

func initLogger() {
	if initialized {
		return
	}
	initialized = true

	// Detect service name from executable name
	if len(os.Args) > 0 {
		parts := strings.Split(os.Args[0], "/")
		serviceName = parts[len(parts)-1]
	}

	// Try loading the dedicated service's .env file first
	if serviceName != "" {
		loadEnvFile(serviceName + "/.env")
	}
	// Fallbacks
	loadEnvFile(".env")
	loadEnvFile("../.env")

	// Read environment configuration
	if envServiceName := os.Getenv("SERVICE_NAME"); envServiceName != "" {
		serviceName = envServiceName
	}

	if os.Getenv("LOG_HTTP_BODY") == "true" {
		logHttpBody = true
	} else if os.Getenv("LOG_HTTP_BODY") == "false" {
		logHttpBody = false
	} else {
		// Default to false in production/staging (when APP_ENV is production or staging)
		appEnv := strings.ToLower(os.Getenv("APP_ENV"))
		if appEnv == "production" || appEnv == "prod" || appEnv == "staging" {
			logHttpBody = false
		} else {
			logHttpBody = true
		}
	}

	levelsEnv := os.Getenv("LOG_LEVELS")
	if levelsEnv != "" {
		logLevels = make(map[string]bool)
		parts := strings.Split(strings.ToUpper(levelsEnv), ",")
		for _, p := range parts {
			logLevels[strings.TrimSpace(p)] = true
		}
	} else {
		// Default to all levels
		logLevels = map[string]bool{
			"INFO":  true,
			"WARN":  true,
			"ERROR": true,
			"FATAL": true,
		}
	}

	// Open or create the dedicated log file
	if serviceName != "" {
		_ = os.MkdirAll("logs", 0755)
		filePath := fmt.Sprintf("logs/%s.log", serviceName)
		var err error
		logFile, err = os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Logger failed to open log file %s: %v\n", filePath, err)
		}
	}
}

func getCaller() string {
	var fallback string
	for i := 2; i < 15; i++ {
		_, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}
		if !strings.Contains(file, "logger/logger.go") {
			if fallback == "" {
				fallback = fmt.Sprintf("%s:%d", filepath.Base(file), line)
			}
			if !strings.Contains(file, "github.com/gin-gonic") &&
				!strings.Contains(file, "gin/context.go") &&
				!strings.Contains(file, "net/http") &&
				!strings.Contains(file, "asm_amd64.s") {
				return fmt.Sprintf("%s:%d", filepath.Base(file), line)
			}
		}
	}
	if fallback != "" {
		return fallback
	}
	return "unknown:0"
}

func prettyPrintJSON(raw string) string {
	if raw == "" {
		return "    (empty)"
	}
	var parsed interface{}
	err := json.Unmarshal([]byte(raw), &parsed)
	if err != nil {
		// If it's not valid JSON, indent raw string lines
		return "    " + strings.ReplaceAll(raw, "\n", "\n    ")
	}
	pretty, err := json.MarshalIndent(parsed, "    ", "  ")
	if err != nil {
		return "    " + strings.ReplaceAll(raw, "\n", "\n    ")
	}
	return "    " + string(pretty)
}

func serializeVal(v interface{}) string {
	if v == nil {
		return `""`
	}

	var str string
	if err, ok := v.(error); ok {
		str = err.Error()
	} else if s, ok := v.(string); ok {
		// If a value represents a valid file on disk, only log its base filename.
		if s != "" {
			if info, err := os.Stat(s); err == nil && !info.IsDir() {
				s = filepath.Base(s)
			}
		}
		str = s
	} else {
		str = fmt.Sprintf("%v", v)
	}

	// If the string contains spaces, equal signs, newlines, tabs, quotes or pipes, wrap it in double quotes
	if strings.ContainsAny(str, " \t\n\r=\"'|") {
		escaped := strings.ReplaceAll(str, `"`, `\"`)
		escaped = strings.ReplaceAll(escaped, "\n", " ")
		escaped = strings.ReplaceAll(escaped, "\r", "")
		return `"` + escaped + `"`
	}

	return str
}

func serializeJSONVal(v interface{}) string {
	if v == nil {
		return ""
	}
	if err, ok := v.(error); ok {
		return err.Error()
	}
	if p, ok := v.(proto.Message); ok {
		bytes, err := protojson.Marshal(p)
		if err == nil {
			return string(bytes)
		}
	}
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	}
	bytes, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(bytes)
}

func isTTY() bool {
	fileInfo, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

// LogEntry defines the JSON structure for file logging
type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Caller    string                 `json:"caller"`
	Service   string                 `json:"service"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

const (
	MaxLogSize = 50 * 1024 * 1024 // 50MB
)

func rotateLogFile() {
	if logFile == nil {
		return
	}

	info, err := logFile.Stat()
	if err != nil || info.Size() < MaxLogSize {
		return
	}

	// Close current file
	logFile.Close()

	filePath := fmt.Sprintf("logs/%s.log", serviceName)
	backupPath := fmt.Sprintf("logs/%s.%s.log", serviceName, time.Now().Format("20060102-150405"))

	// Rename current log to backup
	_ = os.Rename(filePath, backupPath)

	// Open new log file
	logFile, err = os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Logger failed to re-open log file %s: %v\n", filePath, err)
	}

	// Optional: Delete very old logs (keep last 5 backups)
	files, _ := os.ReadDir("logs")
	var backups []string
	prefix := serviceName + "."
	for _, f := range files {
		if !f.IsDir() && strings.HasPrefix(f.Name(), prefix) && strings.HasSuffix(f.Name(), ".log") {
			backups = append(backups, f.Name())
		}
	}
	if len(backups) > 5 {
		sort.Strings(backups)
		for i := 0; i < len(backups)-5; i++ {
			_ = os.Remove("logs/" + backups[i])
		}
	}
}

// Internal logging helper
func logMessage(level Level, color string, message string, ctx map[string]interface{}, jsonCtx map[string]interface{}) {
	initLogger()

	caller := getCaller()

	if ctx == nil {
		ctx = make(map[string]interface{})
	}

	// 1. Prepare JSON format for File
	entry := LogEntry{
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Level:     string(level),
		Message:   message,
		Caller:    caller,
		Service:   serviceName,
		Fields:    make(map[string]interface{}),
	}

	for k, v := range ctx {
		entry.Fields[k] = v
	}
	for k, v := range jsonCtx {
		entry.Fields[k] = serializeJSONVal(v)
	}

	jsonBytes, _ := json.Marshal(entry)
	fileLine := string(jsonBytes) + "\n"

	// Write to file with rotation check
	if logFile != nil {
		rotateLogFile()
		_, _ = logFile.WriteString(fileLine)
	}

	// 2. Format log for console (stdout), filtered by LOG_LEVELS
	if logLevels[string(level)] {
		if isTTY() {
			// Extract req_body and resp_body if they exist so they don't print in the inline context on console
			var reqBodyVal, respBodyVal interface{}
			var hasReq, hasResp bool

			if v, ok := ctx["req_body"]; ok {
				reqBodyVal = v
				hasReq = true
			}
			if v, ok := ctx["resp_body"]; ok {
				respBodyVal = v
				hasResp = true
			}

			// Make a copy of ctx for console so we can exclude long fields inline
			consoleCtx := make(map[string]interface{})
			for k, v := range ctx {
				if k != "req_body" && k != "resp_body" {
					if _, isJson := jsonCtx[k]; !isJson {
						consoleCtx[k] = v
					}
				}
			}
			consoleCtx["caller"] = caller

			ctxStr := ""
			if len(consoleCtx) > 0 {
				ctxStr = " " + colorGray + "|" + colorReset
				keys := make([]string, 0, len(consoleCtx))
				for k := range consoleCtx {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					ctxStr += fmt.Sprintf(" %s=%s", colorCyan+k+colorReset, serializeVal(consoleCtx[k]))
				}
			}

			extraConsole := ""
			if hasReq {
				prettyReq := prettyPrintJSON(fmt.Sprintf("%v", reqBodyVal))
				extraConsole += "\n  " + colorYellow + "► Request Body:" + colorReset + "\n" + prettyReq
			}
			if hasResp {
				prettyResp := prettyPrintJSON(fmt.Sprintf("%v", respBodyVal))
				extraConsole += "\n  " + colorCyan + "◄ Response Body:" + colorReset + "\n" + prettyResp
			}

			// Sort jsonCtx keys for deterministic console output
			jsonKeys := make([]string, 0, len(jsonCtx))
			for k := range jsonCtx {
				jsonKeys = append(jsonKeys, k)
			}
			sort.Strings(jsonKeys)
			for _, k := range jsonKeys {
				val := serializeJSONVal(jsonCtx[k])
				prettyVal := prettyPrintJSON(val)
				extraConsole += "\n  " + colorYellow + "► " + k + ":" + colorReset + "\n" + prettyVal
			}

			fmt.Fprintf(os.Stdout, "%s %s[%s]%s %s%s%s\n",
				time.Now().Format("2006/01/02 15:04:05.000"),
				color, level, colorReset,
				message,
				ctxStr,
				extraConsole,
			)
		} else {
			// If redirected, write the clean JSON format directly to stdout
			fmt.Fprint(os.Stdout, fileLine)
		}
	}
}

type bodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w bodyLogWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w bodyLogWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

// GinMiddleware provides request/response logging for Gin web services
func GinMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		initLogger()

		path := c.Request.URL.Path
		isWebSocket := strings.ToLower(c.GetHeader("Upgrade")) == "websocket"
		// Skip logging for telemetry, log streams, health checks, and WebSocket connections
		if path == "/logs/stream" || path == "/debug/profiler" || path == "/debug/profiler/reset" ||
			path == "/logs" || path == "/profiler" || path == "/containers" || path == "/health" ||
			strings.Contains(path, "/ws") || strings.Contains(path, "/stream") || isWebSocket {
			c.Next()
			return
		}

		start := time.Now()
		query := RedactQuery(c.Request.URL.RawQuery)
		method := c.Request.Method
		clientIP := c.ClientIP()

		var reqID string
		if val, exists := c.Get("request_id"); exists {
			reqID = fmt.Sprintf("%v", val)
		} else {
			reqID = c.GetHeader("X-Request-ID")
		}

		// 1. Capture Request Body safely
		reqBody := ""
		if logHttpBody && c.Request.Body != nil {
			bodyBytes, err := io.ReadAll(c.Request.Body)
			if err == nil {
				c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
				reqBody = RedactJSON(string(bodyBytes))
				if len(reqBody) > 4096 {
					reqBody = reqBody[:4096] + " ... [TRUNCATED]"
				}
			}
		}

		// 2. Intercept Response Body using bodyLogWriter
		var blw *bodyLogWriter
		if logHttpBody {
			blw = &bodyLogWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
			c.Writer = blw
		}

		// Process request
		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		// Format latency to a clean, rounded string (e.g. 40.87ms)
		latencyStr := fmt.Sprintf("%.2fms", float64(latency.Nanoseconds())/1e6)
		if latency < time.Millisecond {
			latencyStr = fmt.Sprintf("%.2fµs", float64(latency.Nanoseconds())/1e3)
		}

		// Get Intercepted Response Body
		respBody := ""
		if logHttpBody && blw != nil {
			respBody = RedactJSON(blw.body.String())
			if len(respBody) > 4096 {
				respBody = respBody[:4096] + " ... [TRUNCATED]"
			}
		}

		logBuilder := Field("status", status).
			Field("latency", latencyStr).
			Field("ip", clientIP)

		if query != "" {
			logBuilder = logBuilder.Field("query", query)
		}
		if reqID != "" {
			logBuilder = logBuilder.Field("request_id", reqID)
		}
		if reqBody != "" {
			logBuilder = logBuilder.Field("req_body", reqBody)
		}
		if respBody != "" {
			logBuilder = logBuilder.Field("resp_body", respBody)
		}

		msg := fmt.Sprintf("HTTP %s %s", method, path)

		if status >= 500 {
			logBuilder.Error("%s", msg)
		} else {
			logBuilder.Info("%s", msg)
		}
	}
}

// TraceMiddleware is a Gin middleware that extracts or generates trace and request IDs.
func TraceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader("X-Trace-ID")
		if traceID == "" {
			traceID = c.GetHeader("x-trace-id")
		}
		if traceID == "" {
			traceID = uuid.New().String()
		}

		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = c.GetHeader("x-request-id")
		}
		if requestID == "" {
			requestID = uuid.New().String()
		}

		c.Writer.Header().Set("X-Trace-ID", traceID)
		c.Writer.Header().Set("X-Request-ID", requestID)

		c.Set("trace_id", traceID)
		c.Set("request_id", requestID)

		ctx := context.WithValue(c.Request.Context(), "trace_id", traceID)
		ctx = context.WithValue(ctx, "request_id", requestID)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// UnaryClientInterceptor passes trace_id and request_id over gRPC metadata.
func UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		traceID, _ := ctx.Value("trace_id").(string)
		requestID, _ := ctx.Value("request_id").(string)

		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		} else {
			md = md.Copy()
		}

		if traceID != "" {
			md.Set("x-trace-id", traceID)
		}
		if requestID != "" {
			md.Set("x-request-id", requestID)
		}

		ctx = metadata.NewOutgoingContext(ctx, md)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// UnaryServerInterceptor extracts trace_id and request_id from incoming gRPC metadata and logs the call.
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()
		md, ok := metadata.FromIncomingContext(ctx)
		var traceID, requestID string
		if ok {
			if vals := md.Get("x-trace-id"); len(vals) > 0 {
				traceID = vals[0]
			}
			if vals := md.Get("x-request-id"); len(vals) > 0 {
				requestID = vals[0]
			}
		}

		if traceID != "" {
			ctx = context.WithValue(ctx, "trace_id", traceID)
		}
		if requestID != "" {
			ctx = context.WithValue(ctx, "request_id", requestID)
		}

		resp, err := handler(ctx, req)
		duration := time.Since(start)

		entry := WithContext(ctx).
			Field("grpc_method", info.FullMethod).
			Field("duration_ms", duration.Milliseconds()).
			JsonField("grpc_req", req)

		if err != nil {
			entry.Field("error", err.Error()).
				Error("gRPC Method %s - Failed", info.FullMethod)
		} else {
			entry.JsonField("grpc_resp", resp).
				Info("gRPC Method %s - Success", info.FullMethod)
		}

		return resp, err
	}
}
