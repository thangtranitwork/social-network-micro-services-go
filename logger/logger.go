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
	"strings"
	"time"

	"github.com/gin-gonic/gin"
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
)

// Fields represents a map of log context
type Fields map[string]interface{}

// Entry represents a log entry with fields
type Entry struct {
	fields map[string]interface{}
}

// NewEntry creates a new log entry
func NewEntry() *Entry {
	return &Entry{fields: make(map[string]interface{})}
}

// Field adds a single key-value context field to the entry
func (e *Entry) Field(key string, value interface{}) *Entry {
	e.fields[key] = value
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
		bodyStr := string(reqBody)
		if len(bodyStr) > 2048 {
			bodyStr = bodyStr[:2048] + "... [truncated]"
		}
		e.fields["req_body"] = bodyStr
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
	logMessage(LevelInfo, colorBlue, fmt.Sprintf(format, args...), e.fields)
}

// Warn prints a WARN level log with the chained fields
func (e *Entry) Warn(format string, args ...interface{}) {
	logMessage(LevelWarn, colorYellow, fmt.Sprintf(format, args...), e.fields)
}

// Error prints an ERROR level log with the chained fields
func (e *Entry) Error(format string, args ...interface{}) {
	logMessage(LevelError, colorRed, fmt.Sprintf(format, args...), e.fields)
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
)

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

	levelsEnv := os.Getenv("LOG_LEVELS")
	if levelsEnv != "" {
		logLevels = make(map[string]bool)
		parts := strings.Split(strings.ToUpper(levelsEnv), ",")
		for _, p := range parts {
			logLevels[strings.TrimSpace(p)] = true
		}
	} else {
		// Default to all 3 levels
		logLevels = map[string]bool{
			"INFO":  true,
			"WARN":  true,
			"ERROR": true,
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

// Internal logging helper
func logMessage(level Level, color string, message string, ctx map[string]interface{}) {
	initLogger()

	timestamp := time.Now().Format("2006/01/02 15:04:05")
	caller := getCaller()

	if ctx == nil {
		ctx = make(map[string]interface{})
	}
	ctx["caller"] = caller

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

	// 1. Format raw log without ANSI colors for the file output (keep req_body/resp_body on the same single line)
	ctxFileStr := ""
	if len(ctx) > 0 {
		ctxFileStr = " |"
		for k, v := range ctx {
			ctxFileStr += fmt.Sprintf(" %s=%s", k, serializeVal(v))
		}
	}

	fileLine := fmt.Sprintf("%s [%s] %s%s\n", timestamp, level, message, ctxFileStr)

	if logFile != nil {
		_, _ = logFile.WriteString(fileLine)
	}

	// 2. Format colored log for console (stdout), filtered by LOG_LEVELS
	if logLevels[string(level)] {
		// Make a copy of ctx for console so we can exclude long fields inline
		consoleCtx := make(map[string]interface{})
		for k, v := range ctx {
			if k != "req_body" && k != "resp_body" {
				consoleCtx[k] = v
			}
		}

		ctxStr := ""
		if len(consoleCtx) > 0 {
			ctxStr = " " + colorGray + "|" + colorReset
			for k, v := range consoleCtx {
				ctxStr += fmt.Sprintf(" %s=%s", colorCyan+k+colorReset, serializeVal(v))
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

		fmt.Fprintf(os.Stdout, "%s %s[%s]%s %s%s%s\n",
			timestamp,
			color, level, colorReset,
			message,
			ctxStr,
			extraConsole,
		)
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
		path := c.Request.URL.Path
		// Skip logging for long-running log streams and WebSocket connections
		if path == "/logs/stream" || strings.Contains(path, "/ws") || strings.Contains(path, "/stream") {
			c.Next()
			return
		}

		start := time.Now()
		query := c.Request.URL.RawQuery
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
		if c.Request.Body != nil {
			bodyBytes, err := io.ReadAll(c.Request.Body)
			if err == nil {
				c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
				reqBody = string(bodyBytes)
				if len(reqBody) > 2048 {
					reqBody = reqBody[:2048] + "... [truncated]"
				}
			}
		}

		// 2. Intercept Response Body using bodyLogWriter
		blw := &bodyLogWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
		c.Writer = blw

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
		respBody := blw.body.String()
		if len(respBody) > 2048 {
			respBody = respBody[:2048] + "... [truncated]"
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
		}  else {
			logBuilder.Info("%s", msg)
		}
	}
}
