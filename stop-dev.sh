#!/bin/bash

echo "====== Stopping Go Microservices ======"

kill_process() {
    local name=$1
    local proc=$2
    echo "Stopping $name..."
    pkill -f "$proc"
}

kill_process "api-gateway" "bin/api-gateway"
kill_process "auth-service" "bin/auth-service"
kill_process "user-service" "bin/user-service"
kill_process "post-service" "bin/post-service"
kill_process "chat-service" "bin/chat-service"
kill_process "notification-service" "bin/notification-service"
kill_process "ai-service" "bin/ai-service"
kill_process "admin-service" "bin/admin-service"

echo "====== All Services Stopped! ======"
