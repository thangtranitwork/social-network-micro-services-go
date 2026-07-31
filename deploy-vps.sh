#!/bin/bash

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEMPLATE_PATH="$PROJECT_ROOT/scripts/deploy.sh.template"

# Load local environment if present (Git-ignored)
if [ -f "$PROJECT_ROOT/.env" ]; then
    set -a
    source "$PROJECT_ROOT/.env"
    set +a
fi

# ========================================================
# VPS DEPLOYMENT CONFIGURATION (Adjust to your host setup)
# ========================================================
SERVER_IP="${SERVER_IP:-127.0.0.1}"    # Target VPS IP
SSH_USER="${SSH_USER:-ubuntu}"        # Target SSH User
SSH_PORT="${SSH_PORT:-22}"            # SSH port
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_rsa}" # SSH Identity file
SERVICE_DIR="social-network"           # Subdirectory on remote host
SSH_TARGET="$SSH_USER@$SERVER_IP"

SSH_CMD="ssh -i $SSH_KEY -p $SSH_PORT -o StrictHostKeyChecking=no"
SCP_CMD="scp -i $SSH_KEY -P $SSH_PORT -o StrictHostKeyChecking=no"

# List of valid microservices
VALID_SERVICES=(
    "api-gateway"
    "auth-service"
    "user-service"
    "post-service"
    "chat-service"
    "notification-service"
    "ai-service"
    "file-service"
    "admin-service"
    "search-service"
    "story-service"
    "fcm-service"
    "recommendation-service"
)

usage() {
    echo "Usage: $0 {service_name|all} [deploy_message]"
    echo "Valid services: ${VALID_SERVICES[*]} or 'all'"
    exit 1
}

# Check arguments
if [ -z "$1" ]; then
    usage
fi

TARGET_SERVICE="${1%/}"
DEPLOY_MESSAGE="${2:-Manual deploy via deploy-vps.sh}"

# Verify service name
is_valid=0
if [ "$TARGET_SERVICE" = "all" ]; then
    is_valid=1
else
    for s in "${VALID_SERVICES[@]}"; do
        if [ "$s" = "$TARGET_SERVICE" ]; then
            is_valid=1
            break
        fi
    done
fi

if [ $is_valid -eq 0 ]; then
    echo "Error: Invalid service name '$TARGET_SERVICE'"
    usage
fi

deploy_single_service() {
    local name=$1
    local remote_base="/root"
    if [ "$SSH_USER" != "root" ]; then
        remote_base="/home/$SSH_USER"
    fi
    local remote_path="$remote_base/$SERVICE_DIR/$name"
    
    echo "=========================================================="
    echo "🚀 DEPLOYING MICROSERVICE: $name"
    echo "   Target Server: $SSH_TARGET:$SSH_PORT"
    echo "   Remote Path:   $remote_path"
    echo "=========================================================="
    
    # 1. Ensure remote directory exists
    echo "Creating remote directory..."
    $SSH_CMD $SSH_TARGET "mkdir -p $remote_path"
    
    # 2. Sync master .env to remote base directory
    if [ -f "$PROJECT_ROOT/.env" ]; then
        echo "Syncing .env configuration to VPS..."
        $SCP_CMD "$PROJECT_ROOT/.env" $SSH_TARGET:$remote_base/$SERVICE_DIR/.env
    fi

    # 3. Copy and set up the dynamic deploy.sh script
    echo "Syncing deploy runner..."
    $SCP_CMD "$TEMPLATE_PATH" $SSH_TARGET:$remote_path/deploy.sh
    $SSH_CMD $SSH_TARGET "chmod +x $remote_path/deploy.sh"
    
    # 4. Stop running instance safely if exists
    echo "Stopping current process..."
    $SSH_CMD $SSH_TARGET "cd $remote_path && ./deploy.sh $name stop" || true
    
    # 5. Compile Go binary locally for Linux Target OS
    echo "Compiling Go binary locally for Target OS (Linux/amd64)..."
    if ! GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-w -s" -o "bin/linux/$name" "$name/main.go"; then
        echo "❌ Compilation failed for $name! Aborting deployment."
        exit 1
    fi
    
    # 6. Push the compiled binary to the VPS
    echo "Uploading binary to VPS..."
    $SCP_CMD "bin/linux/$name" $SSH_TARGET:$remote_path/
    
    # 7. Mark executable and start the process in background via PID tracker
    echo "Starting service on VPS..."
    $SSH_CMD $SSH_TARGET "chmod +x $remote_path/$name"
    $SSH_CMD $SSH_TARGET "cd $remote_path && ./deploy.sh $name start"
    
    # 8. Print latest startup logs
    echo "Polling logs..."
    sleep 1
    $SSH_CMD $SSH_TARGET "cd $remote_path && tail -n 10 service.log"
    echo "Done deploying $name!"
    echo ""
}

# Create output folder for compiled binaries
mkdir -p bin/linux

if [ "$TARGET_SERVICE" = "all" ]; then
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
