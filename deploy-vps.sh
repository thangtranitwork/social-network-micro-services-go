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
SERVER_IP=$(echo "${SERVER_IP:-127.0.0.1}" | tr -d '\r\n ')    # Target VPS IP
SSH_USER=$(echo "${SSH_USER:-ubuntu}" | tr -d '\r\n ')        # Target SSH User
SSH_PORT=$(echo "${SSH_PORT:-22}" | tr -d '\r\n ')            # SSH port
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_rsa}" # SSH Identity file
SERVICE_DIR="social-network"           # Subdirectory on remote host
SSH_TARGET="$SSH_USER@$SERVER_IP"

ssh_exec() {
    if [ -n "$SSH_KEY" ] && [ -f "$SSH_KEY" ]; then
        ssh -i "$SSH_KEY" -p "$SSH_PORT" -o StrictHostKeyChecking=no "$SSH_TARGET" "$1"
    else
        ssh -p "$SSH_PORT" -o StrictHostKeyChecking=no "$SSH_TARGET" "$1"
    fi
}

scp_exec() {
    if [ -n "$SSH_KEY" ] && [ -f "$SSH_KEY" ]; then
        scp -i "$SSH_KEY" -P "$SSH_PORT" -o StrictHostKeyChecking=no "$1" "$SSH_TARGET:$2"
    else
        scp -P "$SSH_PORT" -o StrictHostKeyChecking=no "$1" "$SSH_TARGET:$2"
    fi
}

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
    ssh_exec "mkdir -p $remote_path"
    
    # 2. Sync master .env to remote base directory
    if [ -f "$PROJECT_ROOT/.env" ]; then
        echo "Syncing .env configuration to VPS..."
        scp_exec "$PROJECT_ROOT/.env" "$remote_base/$SERVICE_DIR/.env"
    fi

    # 3. Copy and set up the dynamic deploy.sh script
    echo "Syncing deploy runner..."
    scp_exec "$TEMPLATE_PATH" "$remote_path/deploy.sh"
    ssh_exec "chmod +x $remote_path/deploy.sh"
    
    # 4. Stop running instance safely if exists
    echo "Stopping current process..."
    ssh_exec "cd $remote_path && ./deploy.sh $name stop" || true
    
    # 5. Compile Go binary locally for Linux Target OS
    echo "Compiling Go binary locally for Target OS (Linux/amd64)..."
    if ! GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-w -s" -o "bin/linux/$name" "$name/main.go"; then
        echo "❌ Compilation failed for $name! Aborting deployment."
        exit 1
    fi
    
    # 6. Push the compiled binary to the VPS
    echo "Uploading binary to VPS..."
    scp_exec "bin/linux/$name" "$remote_path/"
    
    # 7. Mark executable and start the process in background via PID tracker
    echo "Starting service on VPS..."
    ssh_exec "chmod +x $remote_path/$name"
    ssh_exec "cd $remote_path && ./deploy.sh $name start"
    
    # 8. Print latest startup logs
    echo "Polling logs..."
    sleep 1
    ssh_exec "cd $remote_path && tail -n 10 service.log"
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
