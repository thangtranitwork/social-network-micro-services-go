package main

import (
	"bufio"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"social-network-go/api-gateway/config"
	"social-network-go/api-gateway/middleware"
	"social-network-go/logger"
	"social-network-go/pb"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

// Allowed CORS origins
var allowedOrigins = map[string]bool{
	"http://localhost:3000":      true,
	"http://localhost:3001":      true,
	"http://192.168.1.48:3000":  true,
	"https://pocpoc.online":     true,
	"https://www.pocpoc.online": true,
}

func main() {
	logger.Info("Starting API Gateway...")

	// 1. Load configuration
	cfg := config.LoadConfig()

	// 2. Establish gRPC connection to Auth Service
	var authClient pb.AuthServiceClient
	conn, err := grpc.NewClient(
		cfg.AuthGrpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		logger.Warn("Warning: Failed to create Auth gRPC client at %s: %v. Auth validation will fail.", cfg.AuthGrpcAddr, err)
	} else {
		defer conn.Close()
		authClient = pb.NewAuthServiceClient(conn)
		// Force connection establishment immediately (not lazy)
		conn.Connect()
		logger.Info("Auth gRPC client created for %s (state: %s)", cfg.AuthGrpcAddr, conn.GetState().String())
		// Wait up to 3s for the connection to become READY
		for i := 0; i < 6; i++ {
			state := conn.GetState()
			if state == connectivity.Ready {
				logger.Info("Connected to Auth gRPC Service at %s", cfg.AuthGrpcAddr)
				break
			}
			logger.Info("Auth gRPC state: %s, waiting...", state.String())
			time.Sleep(500 * time.Millisecond)
		}
	}

	// 3. Initialize Gin engine
	r := gin.Default()

	// CORS Setup - Must use explicit origin (not wildcard) when credentials: true
	r.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if allowedOrigins[origin] {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		} else if origin == "" {
			// Same-origin or non-browser requests (curl, Postman, etc.)
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		}
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE, PATCH")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-User-ID, X-User-Email, X-User-Role, x-continue-page")
		c.Writer.Header().Set("Vary", "Origin")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "UP",
			"timestamp": time.Now().Format(time.RFC3339),
			"gateway":   "UP",
		})
	})

	// Real-time Log Stream Endpoint (Server-Sent Events)
	r.GET("/logs/stream", func(c *gin.Context) {
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
			// If file doesn't exist yet, wait or return a notice
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
	})

	// Log Dashboard Frontend GUI
	r.GET("/logs", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(logDashboardHTML))
	})

	// Reverse Proxy helper
	proxyTo := func(target string) gin.HandlerFunc {
		targetUrl, err := url.Parse(target)
		if err != nil {
			log.Fatalf("Invalid proxy target URL: %v", err)
		}
		proxy := httputil.NewSingleHostReverseProxy(targetUrl)
		// Strip CORS headers from upstream responses — Gateway is the sole CORS authority
		proxy.ModifyResponse = func(resp *http.Response) error {
			resp.Header.Del("Access-Control-Allow-Origin")
			resp.Header.Del("Access-Control-Allow-Methods")
			resp.Header.Del("Access-Control-Allow-Headers")
			resp.Header.Del("Access-Control-Allow-Credentials")
			resp.Header.Del("Access-Control-Expose-Headers")
			resp.Header.Del("Vary")
			return nil
		}
		return func(c *gin.Context) {
			log.Printf("[PROXY] %s %s -> %s%s", c.Request.Method, c.Request.URL.Path, target, c.Request.URL.Path)
			proxy.ServeHTTP(c.Writer, c.Request)
		}
	}

	// Public Routes (No authentication required)
	r.Any("/v1/auth/*any", proxyTo(cfg.AuthHttpAddr))
	r.Any("/v1/register/*any", proxyTo(cfg.AuthHttpAddr))
	r.Any("/v1/register", proxyTo(cfg.AuthHttpAddr))
	r.Any("/v1/forgot-password", proxyTo(cfg.AuthHttpAddr))
	r.Any("/v1/reset-password", proxyTo(cfg.AuthHttpAddr))
	r.Any("/v1/update-password", proxyTo(cfg.AuthHttpAddr))
	
	// Authenticated Routes (Requires JWT Token)
	authMiddleware := middleware.AuthRequired(authClient)

	// File Service Routes
	r.GET("/v1/files/:id", proxyTo(cfg.FileHttpAddr))
	r.POST("/v1/files/upload", authMiddleware, proxyTo(cfg.FileHttpAddr))
	r.POST("/v1/files/upload-multiple", authMiddleware, proxyTo(cfg.FileHttpAddr))
	r.GET("/v1/files/upload/presigned", authMiddleware, proxyTo(cfg.FileHttpAddr))
	r.GET("/v1/files/:id/presigned", authMiddleware, proxyTo(cfg.FileHttpAddr))
	r.DELETE("/v1/files/:id", authMiddleware, proxyTo(cfg.FileHttpAddr))
	r.POST("/v1/files/delete-multiple", authMiddleware, proxyTo(cfg.FileHttpAddr))

	authGroup := r.Group("")
	authGroup.Use(authMiddleware)
	{
		// Proxy to Admin Service
		authGroup.Any("/v2/statistics/*any", proxyTo(cfg.AdminHttpAddr))
		authGroup.Any("/v2/statistics", proxyTo(cfg.AdminHttpAddr))

		// Proxy to User Service
		authGroup.Any("/v1/users/*any", proxyTo(cfg.UserHttpAddr))
		authGroup.Any("/v1/users", func(c *gin.Context) {
			if c.Request.Method == "GET" && c.GetHeader("X-User-Role") == "ADMIN" {
				proxyTo(cfg.AdminHttpAddr)(c)
			} else {
				proxyTo(cfg.UserHttpAddr)(c)
			}
		})
		authGroup.Any("/v1/friends/*any", proxyTo(cfg.UserHttpAddr))
		authGroup.Any("/v1/friends", proxyTo(cfg.UserHttpAddr))
		authGroup.Any("/v1/blocks/*any", proxyTo(cfg.UserHttpAddr))
		authGroup.Any("/v1/blocks", proxyTo(cfg.UserHttpAddr))
		authGroup.Any("/v1/friend-request/*any", proxyTo(cfg.UserHttpAddr))
		authGroup.Any("/v1/friend-request", proxyTo(cfg.UserHttpAddr))

		// Proxy to Post & Feed Service
		authGroup.Any("/v1/posts/*any", proxyTo(cfg.PostHttpAddr))
		authGroup.Any("/v1/posts", func(c *gin.Context) {
			if c.Request.Method == "GET" && c.GetHeader("X-User-Role") == "ADMIN" {
				proxyTo(cfg.AdminHttpAddr)(c)
			} else {
				proxyTo(cfg.PostHttpAddr)(c)
			}
		})
		authGroup.Any("/v1/comments/*any", proxyTo(cfg.PostHttpAddr))
		authGroup.Any("/v1/comments", proxyTo(cfg.PostHttpAddr))

		// Proxy to Chat Service
		authGroup.Any("/v1/chat/*any", proxyTo(cfg.ChatHttpAddr))
		authGroup.Any("/v1/chat", proxyTo(cfg.ChatHttpAddr))
		authGroup.Any("/v1/stringee/*any", proxyTo(cfg.ChatHttpAddr))
		authGroup.Any("/v1/stringee", proxyTo(cfg.ChatHttpAddr))

		// Proxy to Notification Service
		authGroup.Any("/v1/notifications/*any", proxyTo(cfg.NotificationHttpAddr))
		authGroup.Any("/v1/notifications", proxyTo(cfg.NotificationHttpAddr))
	}

	logger.Info("API Gateway listening on port %s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		logger.Error("Failed to run API Gateway: %v", err)
	}
}

// Ultra-premium Glassmorphic Real-time Dark Log Dashboard
const logDashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>🌌 Core Microservices Log Center</title>
    <link href="https://fonts.googleapis.com/css2?family=Fira+Code:wght@400;500&family=Outfit:wght@400;600;800&display=swap" rel="stylesheet">
    <style>
        :root {
            --bg-color: #0b0c16;
            --panel-bg: rgba(17, 19, 39, 0.65);
            --border-color: rgba(255, 255, 255, 0.06);
            --primary: #6366f1;
            --primary-glow: rgba(99, 102, 241, 0.35);
            --active-tab-glow: rgba(99, 102, 241, 0.15);
            --text-color: #f1f5f9;
            --text-muted: #64748b;
            --success: #10b981;
            --warn: #f59e0b;
            --error: #ef4444;
        }

        * {
            box-sizing: border-box;
            margin: 0;
            padding: 0;
        }

        body {
            font-family: 'Outfit', sans-serif;
            background-color: var(--bg-color);
            color: var(--text-color);
            overflow: hidden;
            height: 100vh;
            display: flex;
            flex-direction: column;
            background-image: 
                radial-gradient(at 10% 10%, rgba(99, 102, 241, 0.12) 0px, transparent 40%),
                radial-gradient(at 90% 85%, rgba(168, 85, 247, 0.1) 0px, transparent 40%);
        }

        /* Top Header Grid */
        header {
            padding: 1.25rem 2rem;
            background: rgba(13, 14, 28, 0.7);
            backdrop-filter: blur(12px);
            border-bottom: 1px solid var(--border-color);
            display: flex;
            align-items: center;
            justify-content: space-between;
            z-index: 10;
        }

        .logo-group {
            display: flex;
            align-items: center;
            gap: 0.75rem;
        }

        .logo-indicator {
            width: 10px;
            height: 10px;
            background-color: var(--success);
            border-radius: 50%;
            box-shadow: 0 0 12px var(--success);
            animation: pulse 2s infinite;
        }

        @keyframes pulse {
            0% { transform: scale(0.9); opacity: 0.6; }
            50% { transform: scale(1.1); opacity: 1; box-shadow: 0 0 16px var(--success); }
            100% { transform: scale(0.9); opacity: 0.6; }
        }

        h1 {
            font-size: 1.35rem;
            font-weight: 800;
            letter-spacing: -0.025em;
            background: linear-gradient(to right, #fff, #94a3b8);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }

        .subtitle {
            font-size: 0.75rem;
            color: var(--text-muted);
        }

        .system-stats {
            display: flex;
            gap: 1.5rem;
        }

        .stat-item {
            text-align: right;
        }

        .stat-label {
            font-size: 0.65rem;
            color: var(--text-muted);
            text-transform: uppercase;
            letter-spacing: 0.05em;
        }

        .stat-val {
            font-size: 0.85rem;
            font-weight: 600;
            color: var(--text-color);
        }

        /* Main Workspace */
        .workspace {
            flex: 1;
            display: flex;
            overflow: hidden;
            padding: 1.5rem;
            gap: 1.5rem;
        }

        /* Sidebar Tabs */
        .sidebar {
            width: 280px;
            background: var(--panel-bg);
            backdrop-filter: blur(16px);
            border: 1px solid var(--border-color);
            border-radius: 16px;
            padding: 1rem;
            display: flex;
            flex-direction: column;
            gap: 0.5rem;
        }

        .panel-title {
            font-size: 0.75rem;
            font-weight: 600;
            color: var(--text-muted);
            padding: 0.25rem 0.5rem 0.5rem;
            text-transform: uppercase;
            letter-spacing: 0.08em;
            border-bottom: 1px solid rgba(255, 255, 255, 0.03);
            margin-bottom: 0.5rem;
        }

        .service-row {
            display: flex;
            align-items: center;
            justify-content: space-between;
            gap: 0.5rem;
            background: transparent;
            border-radius: 12px;
            padding: 0.15rem 0.5rem;
            transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
            border: 1px solid transparent;
        }

        .service-row:hover {
            background: rgba(255, 255, 255, 0.02);
            border-color: rgba(255, 255, 255, 0.01);
        }

        .service-row.active {
            background: var(--active-tab-glow);
            border-color: rgba(99, 102, 241, 0.15);
            box-shadow: inset 0 0 12px rgba(99, 102, 241, 0.08);
        }

        .tab-btn {
            flex: 1;
            display: flex;
            align-items: center;
            justify-content: space-between;
            padding: 0.75rem 0.5rem;
            background: transparent;
            border: none;
            color: #94a3b8;
            font-family: inherit;
            font-size: 0.85rem;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.2s;
            text-align: left;
            outline: none;
        }

        .tab-btn:hover {
            color: #fff;
        }

        .service-row.active .tab-btn {
            color: #fff;
        }

        .action-btn {
            background: rgba(255, 255, 255, 0.03);
            border: 1px solid var(--border-color);
            color: #94a3b8;
            width: 28px;
            height: 28px;
            border-radius: 6px;
            cursor: pointer;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 0.75rem;
            transition: all 0.15s;
            flex-shrink: 0;
            outline: none;
        }

        .action-btn:hover {
            background: rgba(255, 255, 255, 0.08);
            color: #fff;
        }

        .action-btn.paused {
            background: rgba(245, 158, 11, 0.15);
            border-color: rgba(245, 158, 11, 0.3);
            color: var(--warn);
        }

        .service-badge {
            font-size: 0.65rem;
            padding: 0.15rem 0.4rem;
            background: rgba(255, 255, 255, 0.05);
            border-radius: 6px;
            color: var(--text-muted);
            border: 1px solid rgba(255, 255, 255, 0.03);
        }

        .service-row.active .service-badge {
            background: rgba(99, 102, 241, 0.2);
            color: #a5b4fc;
            border-color: rgba(99, 102, 241, 0.1);
        }

        /* Terminal Console */
        .console-area {
            flex: 1;
            display: flex;
            flex-direction: column;
            background: var(--panel-bg);
            backdrop-filter: blur(16px);
            border: 1px solid var(--border-color);
            border-radius: 16px;
            overflow: hidden;
            box-shadow: 0 12px 36px rgba(0, 0, 0, 0.25);
        }

        /* Console Controls */
        .console-header {
            padding: 0.85rem 1.25rem;
            background: rgba(13, 14, 28, 0.4);
            border-bottom: 1px solid var(--border-color);
            display: flex;
            align-items: center;
            justify-content: space-between;
        }

        .console-meta {
            display: flex;
            align-items: center;
            gap: 0.5rem;
        }

        .status-dot {
            width: 8px;
            height: 8px;
            background-color: var(--success);
            border-radius: 50%;
            display: inline-block;
        }

        .status-text {
            font-size: 0.75rem;
            font-weight: 600;
            color: var(--text-muted);
        }

        .controls-group {
            display: flex;
            align-items: center;
            gap: 0.75rem;
        }

        .level-select {
            background: rgba(0, 0, 0, 0.25);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            padding: 0.4rem 0.75rem;
            color: var(--text-color);
            font-family: inherit;
            font-size: 0.75rem;
            outline: none;
            cursor: pointer;
            transition: all 0.2s;
        }

        .level-select:focus {
            border-color: var(--primary);
            box-shadow: 0 0 10px rgba(99, 102, 241, 0.15);
        }

        .level-select option {
            background: #0b0c16;
            color: var(--text-color);
        }

        .search-bar {
            background: rgba(0, 0, 0, 0.25);
            border: 1px solid var(--border-color);
            border-radius: 8px;
            padding: 0.4rem 0.75rem;
            color: var(--text-color);
            font-family: inherit;
            font-size: 0.75rem;
            outline: none;
            width: 180px;
            transition: all 0.2s;
        }

        .search-bar:focus {
            border-color: var(--primary);
            box-shadow: 0 0 10px rgba(99, 102, 241, 0.15);
        }

        .ctrl-btn {
            background: rgba(255, 255, 255, 0.03);
            border: 1px solid var(--border-color);
            color: #94a3b8;
            padding: 0.4rem 0.75rem;
            font-family: inherit;
            font-size: 0.75rem;
            font-weight: 600;
            border-radius: 8px;
            cursor: pointer;
            transition: all 0.15s;
            display: flex;
            align-items: center;
            gap: 0.35rem;
        }

        .ctrl-btn:hover {
            background: rgba(255, 255, 255, 0.08);
            color: #fff;
        }

        .ctrl-btn.active-action {
            background: rgba(239, 68, 68, 0.15);
            border-color: rgba(239, 68, 68, 0.2);
            color: #fca5a5;
        }

        /* Log Output Container */
        .terminal {
            flex: 1;
            padding: 1.25rem;
            font-family: 'Fira Code', monospace;
            font-size: 0.75rem;
            line-height: 1.6;
            overflow-y: auto;
            background: rgba(5, 6, 12, 0.55);
            color: #cbd5e1;
        }

        .log-line {
            padding: 0.25rem 0.5rem;
            border-radius: 4px;
            white-space: pre-wrap;
            word-break: break-all;
            display: flex;
            align-items: center;
            gap: 1rem;
        }

        .log-line:hover {
            background: rgba(255, 255, 255, 0.02);
        }

        .time-tag {
            color: var(--text-muted);
            user-select: none;
            flex-shrink: 0;
            width: 75px;
        }

        .log-content {
            flex: 1;
        }

        /* Color Coding based on keywords */
        .level-info { color: #f1f5f9; }
        .level-warn { color: var(--warn); }
        .level-error { color: var(--error); }
        .level-system { color: var(--text-muted); }

        .caller-tag {
            background: rgba(255, 255, 255, 0.03);
            border: 1px solid rgba(255, 255, 255, 0.05);
            border-radius: 4px;
            padding: 0.1rem 0.35rem;
            font-family: inherit;
        }

        /* Custom Scrollbars */
        ::-webkit-scrollbar {
            width: 8px;
            height: 8px;
        }
        ::-webkit-scrollbar-track {
            background: rgba(0, 0, 0, 0.05);
        }
        ::-webkit-scrollbar-thumb {
            background: rgba(255, 255, 255, 0.08);
            border-radius: 4px;
        }
        ::-webkit-scrollbar-thumb:hover {
            background: rgba(255, 255, 255, 0.15);
        }
    </style>
</head>
<body>

    <header>
        <div class="logo-group">
            <div class="logo-indicator"></div>
            <div>
                <h1>Social Network Log Center</h1>
                <div class="subtitle">Real-time Microservice Monitor & Aggregator</div>
            </div>
        </div>

        <div class="system-stats">
            <div class="stat-item">
                <div class="stat-label">Deploy Mode</div>
                <div class="stat-val" style="color: var(--primary);">DEV LOCAL</div>
            </div>
            <div class="stat-item">
                <div class="stat-label">Total Services</div>
                <div class="stat-val">8 Services</div>
            </div>
            <div class="stat-item">
                <div class="stat-label">Core Engine</div>
                <div class="stat-val" style="color: var(--success);">GO NATIVE</div>
            </div>
        </div>
    </header>

    <div class="workspace">
        <div class="sidebar">
            <div class="panel-title">Microservices</div>
            
            <div class="service-row active" id="row-api-gateway">
                <button class="tab-btn" onclick="selectService('api-gateway')">
                    <span>API Gateway</span>
                    <span class="service-badge">:2003</span>
                </button>
                <button class="action-btn" onclick="toggleServiceStream('api-gateway', event)" id="btn-api-gateway" title="Pause/Resume Streaming">
                    <span>⏸</span>
                </button>
            </div>
            
            <div class="service-row" id="row-auth-service">
                <button class="tab-btn" onclick="selectService('auth-service')">
                    <span>Auth Service</span>
                    <span class="service-badge">:8081</span>
                </button>
                <button class="action-btn" onclick="toggleServiceStream('auth-service', event)" id="btn-auth-service" title="Pause/Resume Streaming">
                    <span>⏸</span>
                </button>
            </div>

            <div class="service-row" id="row-user-service">
                <button class="tab-btn" onclick="selectService('user-service')">
                    <span>User Service</span>
                    <span class="service-badge">:8082</span>
                </button>
                <button class="action-btn" onclick="toggleServiceStream('user-service', event)" id="btn-user-service" title="Pause/Resume Streaming">
                    <span>⏸</span>
                </button>
            </div>

            <div class="service-row" id="row-post-service">
                <button class="tab-btn" onclick="selectService('post-service')">
                    <span>Post Service</span>
                    <span class="service-badge">:8083</span>
                </button>
                <button class="action-btn" onclick="toggleServiceStream('post-service', event)" id="btn-post-service" title="Pause/Resume Streaming">
                    <span>⏸</span>
                </button>
            </div>

            <div class="service-row" id="row-chat-service">
                <button class="tab-btn" onclick="selectService('chat-service')">
                    <span>Chat Service</span>
                    <span class="service-badge">:8084</span>
                </button>
                <button class="action-btn" onclick="toggleServiceStream('chat-service', event)" id="btn-chat-service" title="Pause/Resume Streaming">
                    <span>⏸</span>
                </button>
            </div>

            <div class="service-row" id="row-notification-service">
                <button class="tab-btn" onclick="selectService('notification-service')">
                    <span>Notification</span>
                    <span class="service-badge">:8085</span>
                </button>
                <button class="action-btn" onclick="toggleServiceStream('notification-service', event)" id="btn-notification-service" title="Pause/Resume Streaming">
                    <span>⏸</span>
                </button>
            </div>

            <div class="service-row" id="row-admin-service">
                <button class="tab-btn" onclick="selectService('admin-service')">
                    <span>Admin Service</span>
                    <span class="service-badge">:8088</span>
                </button>
                <button class="action-btn" onclick="toggleServiceStream('admin-service', event)" id="btn-admin-service" title="Pause/Resume Streaming">
                    <span>⏸</span>
                </button>
            </div>

            <div class="service-row" id="row-file-service">
                <button class="tab-btn" onclick="selectService('file-service')">
                    <span>File Service</span>
                    <span class="service-badge">:8087</span>
                </button>
                <button class="action-btn" onclick="toggleServiceStream('file-service', event)" id="btn-file-service" title="Pause/Resume Streaming">
                    <span>⏸</span>
                </button>
            </div>

            <div class="service-row" id="row-ai-service">
                <button class="tab-btn" onclick="selectService('ai-service')">
                    <span>AI Service</span>
                    <span class="service-badge">Kafka</span>
                </button>
                <button class="action-btn" onclick="toggleServiceStream('ai-service', event)" id="btn-ai-service" title="Pause/Resume Streaming">
                    <span>⏸</span>
                </button>
            </div>
        </div>

        <div class="console-area">
            <div class="console-header">
                <div class="console-meta">
                    <span class="status-dot" id="stream-status-dot"></span>
                    <span class="status-text" id="stream-status-text">Connected</span>
                </div>

                <div class="controls-group">
                    <select class="level-select" id="level-filter" onchange="applyFilter()">
                        <option value="ALL">All Levels</option>
                        <option value="INFO">INFO</option>
                        <option value="WARN">WARN</option>
                        <option value="ERROR">ERROR</option>
                    </select>
                    <input type="text" class="search-bar" placeholder="Search / Filter..." id="search-input" oninput="applyFilter()">
                    <button class="ctrl-btn" onclick="togglePlayPause()" id="play-pause-btn">
                        <span>Pause</span>
                    </button>
                    <button class="ctrl-btn" onclick="clearTerminal()">
                        <span>Clear</span>
                    </button>
                </div>
            </div>

            <div class="terminal" id="terminal-out">
                <!-- Log Streams will append here -->
            </div>
        </div>
    </div>

    <script>
        let currentService = 'api-gateway';
        let eventSource = null;
        let isPaused = false;
        let filterText = '';

        const serviceStates = {
            'api-gateway': { isPaused: false },
            'auth-service': { isPaused: false },
            'user-service': { isPaused: false },
            'post-service': { isPaused: false },
            'chat-service': { isPaused: false },
            'notification-service': { isPaused: false },
            'ai-service': { isPaused: false },
            'admin-service': { isPaused: false },
            'file-service': { isPaused: false }
        };

        function toggleServiceStream(serviceName, event) {
            if (event) {
                event.stopPropagation(); // prevent triggering tab selection!
            }

            const state = serviceStates[serviceName];
            state.isPaused = !state.isPaused;

            const btn = document.getElementById('btn-' + serviceName);
            const icon = btn.querySelector('span');

            if (state.isPaused) {
                icon.innerText = '▶';
                btn.classList.add('paused');
                btn.title = 'Resume Streaming';
            } else {
                icon.innerText = '⏸';
                btn.classList.remove('paused');
                btn.title = 'Pause Streaming';
            }

            if (serviceName === currentService) {
                updateGlobalPauseUI(state.isPaused);
            }
        }

        function updateGlobalPauseUI(isPausedState) {
            isPaused = isPausedState;
            const globalBtn = document.getElementById('play-pause-btn');
            const statusText = document.getElementById('stream-status-text');
            const statusDot = document.getElementById('stream-status-dot');

            if (isPaused) {
                globalBtn.innerText = 'Resume';
                globalBtn.classList.add('active-action');
                statusText.innerText = 'Paused';
                statusDot.style.backgroundColor = 'var(--warn)';
            } else {
                globalBtn.innerText = 'Pause';
                globalBtn.classList.remove('active-action');
                statusText.innerText = 'Streaming Logs';
                statusDot.style.backgroundColor = 'var(--success)';
            }
        }

        function togglePlayPause() {
            toggleServiceStream(currentService);
        }

        function selectService(serviceName) {
            // Update Active Tab UI
            document.querySelectorAll('.service-row').forEach(row => row.classList.remove('active'));
            document.getElementById('row-' + serviceName).classList.add('active');

            currentService = serviceName;
            
            // Sync with service pause state
            isPaused = serviceStates[serviceName].isPaused;
            updateGlobalPauseUI(isPaused);

            clearTerminal();
            connectStream();
        }

        function connectStream() {
            if (eventSource) {
                eventSource.close();
            }

            const statusDot = document.getElementById('stream-status-dot');
            const statusText = document.getElementById('stream-status-text');

            statusDot.style.backgroundColor = 'var(--warn)';
            statusText.innerText = 'Connecting...';

            eventSource = new EventSource('/logs/stream?service=' + currentService);

            eventSource.addEventListener('log', function(event) {
                if (isPaused) return;
                appendLogLine(event.data);
            });

            eventSource.addEventListener('info', function(event) {
                appendLogLine("[INFO] " + event.data, 'level-system');
            });

            eventSource.addEventListener('error', function(event) {
                appendLogLine("[ERROR] " + event.data, 'level-error');
            });

            eventSource.onopen = function() {
                if (isPaused) {
                    statusDot.style.backgroundColor = 'var(--warn)';
                    statusText.innerText = 'Paused';
                } else {
                    statusDot.style.backgroundColor = 'var(--success)';
                    statusText.innerText = 'Streaming Logs';
                }
            };

            eventSource.onerror = function() {
                statusDot.style.backgroundColor = 'var(--error)';
                statusText.innerText = 'Disconnected. Retrying...';
            };
        }

        function tryFormatJSON(raw) {
            try {
                const parsed = JSON.parse(raw);
                return JSON.stringify(parsed, null, 2);
            } catch(e) {
                return raw;
            }
        }

        function formatLogLine(rawText) {
            let text = rawText;
            
            // 1. Extract caller if present
            let callerStr = "";
            const callerMatch = text.match(/(?:\|\s*)?caller=([^\s|]+)/);
            if (callerMatch) {
                callerStr = callerMatch[1];
                text = text.replace(callerMatch[0], ""); // remove caller from main text
            }

            // 2. Extract timestamp
            let timestamp = "";
            if (text.length >= 19 && /^\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}:\d{2}/.test(text)) {
                timestamp = text.substring(11, 19); // only get HH:MM:SS for compact display
                text = text.substring(20); // rest of the text
            }

            // 3. Detect and style Level Badge
            let levelBadge = "";
            let levelClass = "level-info";
            if (text.includes("[INFO]")) {
                levelBadge = '<span style="color: #818cf8; background: rgba(129, 140, 248, 0.15); padding: 0.15rem 0.4rem; border-radius: 4px; font-weight: bold; margin-right: 0.5rem; font-size: 0.7rem;">INFO</span>';
                text = text.replace("[INFO]", "").trim();
                levelClass = "level-info";
            } else if (text.includes("[WARN]")) {
                levelBadge = '<span style="color: #fbbf24; background: rgba(251, 191, 36, 0.15); padding: 0.15rem 0.4rem; border-radius: 4px; font-weight: bold; margin-right: 0.5rem; font-size: 0.7rem;">WARN</span>';
                text = text.replace("[WARN]", "").trim();
                levelClass = "level-warn";
            } else if (text.includes("[ERROR]")) {
                levelBadge = '<span style="color: #f87171; background: rgba(248, 113, 113, 0.15); padding: 0.15rem 0.4rem; border-radius: 4px; font-weight: bold; margin-right: 0.5rem; font-size: 0.7rem;">ERROR</span>';
                text = text.replace("[ERROR]", "").trim();
                levelClass = "level-error";
            }

            // 4. Format key=value pairs nicely
            const parts = text.split(" |");
            let message = parts[0];
            let contextHtml = "";
            let reqBody = "";
            let respBody = "";

            if (parts.length > 1) {
                contextHtml = ' <span style="color: var(--text-muted)">|</span>';
                const fieldsString = parts.slice(1).join(" |");
                const fieldPairs = fieldsString.match(/([^\s=]+)=(?:"[^"\\]*(?:\\.[^"\\]*)*"|[^\s|]*)/g);
                if (fieldPairs) {
                    fieldPairs.forEach(pair => {
                        const eqIdx = pair.indexOf("=");
                        if (eqIdx !== -1) {
                            const key = pair.substring(0, eqIdx);
                            let val = pair.substring(eqIdx + 1);
                            if (val.startsWith('"') && val.endsWith('"')) {
                                val = val.substring(1, val.length - 1).replace(/\\"/g, '"');
                            }
                            
                            if (key === "req_body") {
                                reqBody = val;
                            } else if (key === "resp_body") {
                                respBody = val;
                            } else {
                                contextHtml += ' <span style="color: #22d3ee; font-weight: 500;">' + key + '</span>=<span style="color: #f1f5f9;">' + val + '</span>';
                            }
                        }
                    });
                } else {
                    contextHtml += ' <span style="color: var(--text-muted)">' + fieldsString + '</span>';
                }
            }

            return {
                timestamp,
                levelBadge,
                message,
                contextHtml,
                callerStr,
                levelClass,
                reqBody,
                respBody
            };
        }

        function appendLogLine(rawText, customClass = '') {
            const terminal = document.getElementById('terminal-out');
            const isScrolledToBottom = terminal.scrollHeight - terminal.clientHeight <= terminal.scrollTop + 50;

            const lineDiv = document.createElement('div');
            lineDiv.className = 'log-line';
            lineDiv.style.flexDirection = 'column';
            lineDiv.style.alignItems = 'stretch';
            lineDiv.style.padding = '0.5rem 0.75rem';

            const formatted = formatLogLine(rawText);

            // Create top container for the log meta & main text
            const topDiv = document.createElement('div');
            topDiv.style.display = 'flex';
            topDiv.style.alignItems = 'center';
            topDiv.style.width = '100%';

            const timeTag = document.createElement('span');
            timeTag.className = 'time-tag';
            timeTag.innerText = formatted.timestamp || new Date().toTimeString().split(' ')[0];

            const contentSpan = document.createElement('span');
            contentSpan.className = 'log-content';
            if (formatted.levelClass) {
                contentSpan.classList.add(formatted.levelClass);
            }
            contentSpan.innerHTML = formatted.levelBadge + formatted.message + formatted.contextHtml;

            topDiv.appendChild(timeTag);
            topDiv.appendChild(contentSpan);

            if (formatted.callerStr) {
                const callerTag = document.createElement('span');
                callerTag.className = 'caller-tag';
                callerTag.style.color = 'var(--text-muted)';
                callerTag.style.fontSize = '0.7rem';
                callerTag.style.flexShrink = '0';
                callerTag.style.marginLeft = 'auto';
                callerTag.innerText = formatted.callerStr;
                topDiv.appendChild(callerTag);
            }

            lineDiv.appendChild(topDiv);

            // Add expandable request/response bodies under the log line
            if (formatted.reqBody || formatted.respBody) {
                const expandContainer = document.createElement('div');
                expandContainer.style.paddingLeft = '3rem';
                expandContainer.style.marginTop = '0.4rem';
                expandContainer.style.fontSize = '0.8rem';
                expandContainer.style.display = 'flex';
                expandContainer.style.flexDirection = 'column';
                expandContainer.style.gap = '0.3rem';

                if (formatted.reqBody) {
                    const btn = document.createElement('button');
                    btn.className = 'log-btn';
                    btn.innerHTML = '▶ View Request Payload';
                    btn.style.background = 'rgba(251, 191, 36, 0.1)';
                    btn.style.border = '1px solid rgba(251, 191, 36, 0.2)';
                    btn.style.borderRadius = '4px';
                    btn.style.color = 'var(--warn)';
                    btn.style.cursor = 'pointer';
                    btn.style.padding = '0.2rem 0.5rem';
                    btn.style.width = 'fit-content';
                    btn.style.fontSize = '0.7rem';
                    btn.style.fontWeight = '500';
                    btn.style.transition = 'all 0.2s';
                    
                    const content = document.createElement('pre');
                    content.style.display = 'none';
                    content.style.background = 'rgba(0,0,0,0.4)';
                    content.style.padding = '0.6rem';
                    content.style.borderRadius = '4px';
                    content.style.color = '#e2e8f0';
                    content.style.margin = '0.2rem 0 0 0';
                    content.style.whiteSpace = 'pre-wrap';
                    content.style.borderLeft = '3px solid var(--warn)';
                    content.innerText = tryFormatJSON(formatted.reqBody);
                    
                    btn.onclick = () => {
                        if (content.style.display === 'none') {
                            content.style.display = 'block';
                            btn.innerHTML = '▼ Hide Request Payload';
                            btn.style.background = 'rgba(251, 191, 36, 0.2)';
                        } else {
                            content.style.display = 'none';
                            btn.innerHTML = '▶ View Request Payload';
                            btn.style.background = 'rgba(251, 191, 36, 0.1)';
                        }
                    };
                    
                    expandContainer.appendChild(btn);
                    expandContainer.appendChild(content);
                }

                if (formatted.respBody) {
                    const btn = document.createElement('button');
                    btn.className = 'log-btn';
                    btn.innerHTML = '▶ View Response Body';
                    btn.style.background = 'rgba(34, 211, 238, 0.1)';
                    btn.style.border = '1px solid rgba(34, 211, 238, 0.2)';
                    btn.style.borderRadius = '4px';
                    btn.style.color = 'var(--cyan)';
                    btn.style.cursor = 'pointer';
                    btn.style.padding = '0.2rem 0.5rem';
                    btn.style.width = 'fit-content';
                    btn.style.fontSize = '0.7rem';
                    btn.style.fontWeight = '500';
                    btn.style.transition = 'all 0.2s';
                    
                    const content = document.createElement('pre');
                    content.style.display = 'none';
                    content.style.background = 'rgba(0,0,0,0.4)';
                    content.style.padding = '0.6rem';
                    content.style.borderRadius = '4px';
                    content.style.color = '#e2e8f0';
                    content.style.margin = '0.2rem 0 0 0';
                    content.style.whiteSpace = 'pre-wrap';
                    content.style.borderLeft = '3px solid var(--cyan)';
                    content.innerText = tryFormatJSON(formatted.respBody);
                    
                    btn.onclick = () => {
                        if (content.style.display === 'none') {
                            content.style.display = 'block';
                            btn.innerHTML = '▼ Hide Response Body';
                            btn.style.background = 'rgba(34, 211, 238, 0.2)';
                        } else {
                            content.style.display = 'none';
                            btn.innerHTML = '▶ View Response Body';
                            btn.style.background = 'rgba(34, 211, 238, 0.1)';
                        }
                    };
                    
                    expandContainer.appendChild(btn);
                    expandContainer.appendChild(content);
                }

                lineDiv.appendChild(expandContainer);
            }

            // Apply search & level filters
            const selectedLevel = document.getElementById('level-filter').value;
            let lineLevel = "ALL";
            if (rawText.includes('[INFO]')) lineLevel = "INFO";
            else if (rawText.includes('[WARN]')) lineLevel = "WARN";
            else if (rawText.includes('[ERROR]')) lineLevel = "ERROR";

            const matchesSearch = !filterText || rawText.toLowerCase().includes(filterText.toLowerCase());
            const matchesLevel = (selectedLevel === "ALL" || lineLevel === selectedLevel);

            if (matchesSearch && matchesLevel) {
                lineDiv.style.display = 'flex';
            } else {
                lineDiv.style.display = 'none';
            }

            terminal.appendChild(lineDiv);

            if (terminal.childElementCount > 1000) {
                terminal.removeChild(terminal.firstElementChild);
            }

            if (isScrolledToBottom) {
                terminal.scrollTop = terminal.scrollHeight;
            }
        }

        function clearTerminal() {
            document.getElementById('terminal-out').innerHTML = '';
        }

        function applyFilter() {
            filterText = document.getElementById('search-input').value;
            const selectedLevel = document.getElementById('level-filter').value;
            const lines = document.querySelectorAll('.log-line');

            lines.forEach(line => {
                const text = line.innerText;
                
                let lineLevel = "ALL";
                if (text.includes('INFO')) lineLevel = "INFO";
                else if (text.includes('WARN')) lineLevel = "WARN";
                else if (text.includes('ERROR')) lineLevel = "ERROR";

                const matchesSearch = !filterText || text.toLowerCase().includes(filterText.toLowerCase());
                const matchesLevel = (selectedLevel === "ALL" || lineLevel === selectedLevel);

                if (matchesSearch && matchesLevel) {
                    line.style.display = 'flex';
                } else {
                    line.style.display = 'none';
                }
            });
        }

        window.onload = function() {
            connectStream();
        };
    </script>
</body>
</html>`
