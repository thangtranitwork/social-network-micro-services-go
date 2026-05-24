#!/bin/bash

# ========================================================
# VPS DEPLOYMENT CONFIGURATION (Adjust to your host setup)
# ========================================================
SERVER_IP="127.0.0.1"    # Default IP matching your dev reference
SSH_USER="ubuntu"             # Target SSH User
SSH_PORT="22"                 # SSH port (default 22, matching your reference port if needed)
SERVICE_DIR="social-network"  # Subdirectory on remote host
SSH_TARGET="$SSH_USER@$SERVER_IP"

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEMPLATE_PATH="$PROJECT_ROOT/scripts/deploy.sh.template"

# List of valid microservices
VALID_SERVICES=("api-gateway" "auth-service" "user-service" "post-service" "chat-service" "notification-service" "ai-service" "file-service" "admin-service")


usage() {
    echo "Usage: $0 {service_name|all} [deploy_message]"
    echo "Valid services: ${VALID_SERVICES[*]} or 'all'"
    exit 1
}

# Check arguments
if [ -z "$1" ]; then
    usage
fi

TARGET_SERVICE="$1"
DEPLOY_MESSAGE="${2:-"Manual deploy via deploy-vps.sh"}"

# Verify service name
is_valid=0
if [ "$TARGET_SERVICE" == "all" ]; then
    is_valid=1
else
    for s in "${VALID_SERVICES[@]}"; do
        if [ "$s" == "$TARGET_SERVICE" ]; then
            is_valid=1
            break
        fi
    fi
fi

if [ $is_valid -eq 0 ]; then
    echo "Error: Invalid service name '$TARGET_SERVICE'"
    usage
fi

deploy_single_service() {
    local name=$1
    local remote_path="/home/$SSH_USER/$SERVICE_DIR/$name"
    
    echo "=========================================================="
    echo "🚀 DEPLOYING MICROSERVICE: $name"
    echo "   Target Server: $SSH_TARGET:$SSH_PORT"
    echo "   Remote Path:   $remote_path"
    echo "=========================================================="
    
    # 1. Ensure remote directory exists
    echo "Creating remote directory..."
    ssh -p $SSH_PORT $SSH_TARGET "mkdir -p $remote_path"
    
    # 2. Copy and set up the dynamic deploy.sh script
    echo "Syncing deploy runner..."
    scp -P $SSH_PORT "$TEMPLATE_PATH" $SSH_TARGET:$remote_path/deploy.sh
    ssh -p $SSH_PORT $SSH_TARGET "chmod +x $remote_path/deploy.sh"
    
    # 3. Stop running instance safely if exists
    echo "Stopping current process..."
    ssh -p $SSH_PORT $SSH_TARGET "cd $remote_path && ./deploy.sh $name stop" || true
    
    # 4. Compile Go binary locally for Linux Target OS
    echo "Compiling Go binary locally for Target OS (Linux/amd64)..."
    GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o "bin/linux/$name" "$name/main.go"
    
    # 5. Push the compiled binary to the VPS
    echo "Uploading binary to VPS..."
    scp -P $SSH_PORT "bin/linux/$name" $SSH_TARGET:$remote_path/
    
    # 6. Mark executable and start the process in background via PID tracker
    echo "Starting service on VPS..."
    ssh -p $SSH_PORT $SSH_TARGET "chmod +x $remote_path/$name"
    ssh -p $SSH_PORT $SSH_TARGET "cd $remote_path && ./deploy.sh $name start"
    
    # 7. Print latest startup logs
    echo "Polling logs..."
    sleep 1
    ssh -p $SSH_PORT $SSH_TARGET "cd $remote_path && tail -n 10 service.log"
    echo "Done deploying $name!"
    echo ""
}

# Create output folder for compiled binaries
mkdir -p bin/linux

if [ "$TARGET_SERVICE" == "all" ]; then
    echo "Deploying ALL microservices sequentially..."
    for s in "${VALID_SERVICES[@]}"; do
        deploy_single_service "$s"
    done
else
    deploy_single_service "$TARGET_SERVICE"
fi

echo "=========================================================="
echo "🎉 Deployment completed successfully!"
echo "   Message: $DEPLOY_MESSAGE"
echo "=========================================================="
