#!/bin/bash

# Create logs directory
mkdir -p logs

echo "====== Starting Go Microservices in Development Mode (Background) ======"

# Function to run a service and pipe logs
run_service() {
    local name=$1
    local cmd=$2
    echo "Starting $name..."
    $cmd > logs/$name.log 2>&1 &
    echo "$name started (PID: $!) | Logs: logs/$name.log"
}

# Ensure binaries exist
if [ ! -d "bin" ] || [ ! -f "bin/api-gateway" ]; then
    echo "Binaries missing. Running build first..."
    make build
fi

# Run services
run_service "auth-service" "./bin/auth-service"
sleep 1 # small delay to let DB connections establish
run_service "user-service" "./bin/user-service"
run_service "post-service" "./bin/post-service"
run_service "chat-service" "./bin/chat-service"
run_service "notification-service" "./bin/notification-service"
run_service "ai-service" "./bin/ai-service"
run_service "admin-service" "./bin/admin-service"
sleep 1
run_service "api-gateway" "./bin/api-gateway"

echo ""
echo "====== All Services Started! ======"
echo "API Gateway is listening on: http://localhost:2003"
echo "To view logs in real-time, run: tail -f logs/*.log"
echo "To stop all services, run: ./stop-dev.sh"
