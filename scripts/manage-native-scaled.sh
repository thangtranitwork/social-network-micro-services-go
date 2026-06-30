#!/bin/bash

# ========================================================
# MANAGE NATIVE MICROSERVICES WITH SCALING & LB
# ========================================================

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="$PROJECT_ROOT/bin"
LOG_DIR="$PROJECT_ROOT/logs"

# Load environment defaults
export PORT_GW1=11111
export PORT_GW2=11112

export PORT_CHAT1=10084
export PORT_CHAT2=10184

stop_services() {
    echo "Stopping Nginx Load Balancer..."
    docker rm -f native-gateway-lb 2>/dev/null || true

    echo "Stopping native Go processes..."
    pkill -f "bin/api-gateway" || true
    pkill -f "bin/auth-service" || true
    pkill -f "bin/user-service" || true
    pkill -f "bin/post-service" || true
    pkill -f "bin/chat-service" || true
    pkill -f "bin/notification-service" || true
    pkill -f "bin/ai-service" || true
    pkill -f "bin/file-service" || true
    pkill -f "bin/admin-service" || true
    pkill -f "bin/search-service" || true
    pkill -f "bin/story-service" || true
    echo "All native services stopped."
}

start_services() {
    echo "====== Step 1: Spinning Up Docker Infra (DBs, Broker) ======"
    cd "$PROJECT_ROOT" && make infra-up

    echo "====== Step 2: Compiling Go Microservices Natively ======"
    make build

    echo "====== Step 3: Cleaning Existing Native Instances ======"
    stop_services

    echo "====== Step 4: Starting Standard Microservices ======"
    mkdir -p "$LOG_DIR"

    # Run standard services natively using local configurations (.env)
    nohup "$BIN_DIR/auth-service" > /dev/null 2> "$LOG_DIR/auth-service.log" &
    nohup "$BIN_DIR/user-service" > /dev/null 2> "$LOG_DIR/user-service.log" &
    nohup "$BIN_DIR/post-service" > /dev/null 2> "$LOG_DIR/post-service.log" &
    nohup "$BIN_DIR/notification-service" > /dev/null 2> "$LOG_DIR/notification-service.log" &
    nohup "$BIN_DIR/ai-service" > /dev/null 2> "$LOG_DIR/ai-service.log" &
    nohup "$BIN_DIR/file-service" > /dev/null 2> "$LOG_DIR/file-service.log" &
    nohup "$BIN_DIR/admin-service" > /dev/null 2> "$LOG_DIR/admin-service.log" &
    nohup "$BIN_DIR/search-service" > /dev/null 2> "$LOG_DIR/search-service.log" &
    nohup "$BIN_DIR/story-service" > /dev/null 2> "$LOG_DIR/story-service.log" &

    echo "====== Step 5: Starting Scaled Chat Service (2 Instances) ======"
    # Instance 1 uses default port (10084) from environment
    CHAT_HTTP_PORT=$PORT_CHAT1 nohup "$BIN_DIR/chat-service" > /dev/null 2> "$LOG_DIR/chat-service-1.log" &

    # Instance 2 overrides port to 10184
    CHAT_HTTP_PORT=$PORT_CHAT2 nohup "$BIN_DIR/chat-service" > /dev/null 2> "$LOG_DIR/chat-service-2.log" &

    echo "====== Step 6: Starting Scaled API Gateways (2 Instances pointing to Chat LB) ======"
    # Both instances are configured to proxy to the Chat LB endpoint (http://localhost:10284)
    GATEWAY_PORT=$PORT_GW1 CHAT_HTTP_ADDR=http://localhost:10284 CHAT_WS_ADDR=ws://localhost:10284 nohup "$BIN_DIR/api-gateway" > /dev/null 2> "$LOG_DIR/api-gateway-1.log" &
    GATEWAY_PORT=$PORT_GW2 CHAT_HTTP_ADDR=http://localhost:10284 CHAT_WS_ADDR=ws://localhost:10284 nohup "$BIN_DIR/api-gateway" > /dev/null 2> "$LOG_DIR/api-gateway-2.log" &

    echo "====== Step 7: Launching Nginx Load Balancer (Docker Host Network) ======"
    # Runs Nginx in Docker with host network mode to proxy host localports directly
    docker run -d \
        --name native-gateway-lb \
        --network host \
        -v "$PROJECT_ROOT/scripts/nginx-native.conf:/etc/nginx/nginx.conf:ro" \
        nginx:alpine

    echo "=========================================================="
    echo "🎉 Native scaled deployment completed successfully!"
    echo "   API Gateway Load Balancer listening on: http://localhost:11110"
    echo "   Chat Service Load Balancer listening on: http://localhost:10284"
    echo "   Logs directory: $LOG_DIR"
    echo "=========================================================="
}

case "$1" in
    start)
        start_services
        ;;
    stop)
        stop_services
        ;;
    restart)
        stop_services
        sleep 1
        start_services
        ;;
    *)
        echo "Usage: $0 {start|stop|restart}"
        exit 1
        ;;
esac
