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
SERVER_IP=$(echo "${SERVER_IP:-127.0.0.1}" | tr -d '\r\n ')        # Target VPS IP
SSH_USER=$(echo "${SSH_USER:-ubuntu}" | tr -d '\r\n ')           # Target SSH User
SSH_PORT=$(echo "${SSH_PORT:-22}" | tr -d '\r\n ')               # SSH port
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_rsa}"  # SSH Identity file
SERVICE_DIR="social-network"             # Subdirectory on remote host
UI_PORT="${UI_PORT:-10000}"              # Host UI Port on VPS
SSH_TARGET="$SSH_USER@$SERVER_IP"

remote_base="/root"
if [ "$SSH_USER" != "root" ]; then
    remote_base="/home/$SSH_USER"
fi
REMOTE_UI_PATH="$remote_base/$SERVICE_DIR/social-network-ui"

ssh_exec() {
    if [ -n "$SSH_KEY" ] && [ -f "$SSH_KEY" ]; then
        ssh -i "$SSH_KEY" -p "$SSH_PORT" -o StrictHostKeyChecking=no "$SSH_TARGET" "$1"
    else
        ssh -p "$SSH_PORT" -o StrictHostKeyChecking=no "$SSH_TARGET" "$1"
    fi
}

rsync_exec() {
    if [ -n "$SSH_KEY" ] && [ -f "$SSH_KEY" ]; then
        rsync -avz -e "ssh -i $SSH_KEY -p $SSH_PORT -o StrictHostKeyChecking=no" "$@"
    else
        rsync -avz -e "ssh -p $SSH_PORT -o StrictHostKeyChecking=no" "$@"
    fi
}

echo "=========================================================="
echo "🚀 DEPLOYING NEXT.JS FRONTEND VIA DOCKER"
echo "   Target Server: $SSH_TARGET:$SSH_PORT"
echo "   Remote Path:   $REMOTE_UI_PATH"
echo "   Host UI Port:  $UI_PORT"
echo "=========================================================="

# 1. Ensure remote directory exists
echo "Creating remote directory on VPS..."
ssh_exec "mkdir -p $REMOTE_UI_PATH"

# 2. Sync UI source code to VPS (excluding node_modules and build cache)
echo "Syncing UI source code to VPS..."
rsync_exec \
    --exclude='node_modules' \
    --exclude='.next' \
    --exclude='.git' \
    "$UI_DIR/" "$SSH_TARGET:$REMOTE_UI_PATH/"

# 3. Build & Run Docker container on VPS
echo "Building and starting UI Docker container on VPS..."
ssh_exec "set -e; cd $REMOTE_UI_PATH
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
ssh_exec "docker logs --tail 15 sn-ui-app"

echo "=========================================================="
echo "🎉 UI Deployment completed successfully!"
echo "   Access UI at: http://$SERVER_IP:$UI_PORT"
echo "=========================================================="
