#!/bin/bash

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
UI_DIR="$PROJECT_ROOT/social-network-ui"

# Load local environment if present (Git-ignored)
if [ -f "$PROJECT_ROOT/.env" ]; then
    set -a
    source "$PROJECT_ROOT/.env"
    set +a
fi

# ========================================================
# VPS UI DOCKER DEPLOYMENT CONFIGURATION (Next.js Frontend)
# ========================================================
SERVER_IP="${SERVER_IP:-127.0.0.1}"        # Target VPS IP
SSH_USER="${SSH_USER:-ubuntu}"           # Target SSH User
SSH_PORT="${SSH_PORT:-22}"               # SSH port
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_rsa}"  # SSH Identity file
SERVICE_DIR="social-network"             # Subdirectory on remote host
UI_PORT="${UI_PORT:-13000}"              # Host UI Port on VPS
SSH_TARGET="$SSH_USER@$SERVER_IP"

remote_base="/root"
if [ "$SSH_USER" != "root" ]; then
    remote_base="/home/$SSH_USER"
fi
REMOTE_UI_PATH="$remote_base/$SERVICE_DIR/social-network-ui"

echo "=========================================================="
echo "🚀 DEPLOYING NEXT.JS FRONTEND VIA DOCKER"
echo "   Target Server: $SSH_TARGET:$SSH_PORT"
echo "   Remote Path:   $REMOTE_UI_PATH"
echo "   Host UI Port:  $UI_PORT"
echo "=========================================================="

# 1. Ensure remote directory exists
echo "Creating remote directory on VPS..."
$SSH_CMD $SSH_TARGET "mkdir -p $REMOTE_UI_PATH"

# 2. Sync UI source code to VPS (excluding node_modules and build cache)
echo "Syncing UI source code to VPS..."
rsync -avz -e "ssh -i $SSH_KEY -p $SSH_PORT -o StrictHostKeyChecking=no" \
    --exclude='node_modules' \
    --exclude='.next' \
    --exclude='.git' \
    "$UI_DIR/" "$SSH_TARGET:$REMOTE_UI_PATH/"

# 3. Build & Run Docker container on VPS
echo "Building and starting UI Docker container on VPS..."
$SSH_CMD $SSH_TARGET "set -e; cd $REMOTE_UI_PATH
docker stop sn-ui-app 2>/dev/null || true
docker rm sn-ui-app 2>/dev/null || true
docker build \
  --build-arg NEXT_PUBLIC_API_URL=http://$SERVER_IP:11111 \
  --build-arg NEXT_PUBLIC_SOCKET_ENDPOINT=http://$SERVER_IP:10085/v1/notifications/ws \
  -t sn-ui-app .
docker run -d --name sn-ui-app \
  --restart unless-stopped \
  -p $UI_PORT:10000 \
  sn-ui-app"

echo "Polling container logs..."
sleep 2
$SSH_CMD $SSH_TARGET "docker logs --tail 15 sn-ui-app"

echo "=========================================================="
echo "🎉 UI Deployment completed successfully!"
echo "   Access UI at: http://$SERVER_IP:$UI_PORT"
echo "=========================================================="
