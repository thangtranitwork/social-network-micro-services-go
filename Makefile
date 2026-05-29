.PHONY: all tidy build test run-gateway run-auth run-user run-post run-chat run-notif run-ai infra-up infra-down clean dev-restart

# Environment defaults
export PORT ?= 11111
export AUTH_GRPC_PORT ?= 10051
export AUTH_HTTP_PORT ?= 10081
export USER_GRPC_PORT ?= 10052
export USER_HTTP_PORT ?= 10082
export POST_HTTP_PORT ?= 10083
export CHAT_HTTP_PORT ?= 10084
export NOTIF_HTTP_PORT ?= 10085
export FILE_GRPC_PORT ?= 10057
export FILE_HTTP_PORT ?= 10087
export ADMIN_HTTP_PORT ?= 10088

all: tidy build

# Tidy Go dependencies
tidy:
	@echo "====== Tidying Go Modules ======"
	go mod tidy

# Run all unit tests
test:
	@echo "====== Running Unit Tests ======"
	go test -v -count=1 ./...

# Compile all microservices
build:
	@echo "====== Building Microservices ======"
	go build -o bin/api-gateway api-gateway/main.go
	go build -o bin/auth-service auth-service/main.go
	go build -o bin/user-service user-service/main.go
	go build -o bin/post-service post-service/main.go
	go build -o bin/chat-service chat-service/main.go
	go build -o bin/notification-service notification-service/main.go
	go build -o bin/ai-service ai-service/main.go
	go build -o bin/file-service file-service/main.go
	go build -o bin/admin-service admin-service/main.go
	@echo "====== Build Complete! Binaries placed in bin/ ======"

dev:
	@echo "====== Building Microservices ======"
	go build -o bin/api-gateway api-gateway/main.go
	go build -o bin/auth-service auth-service/main.go
	go build -o bin/user-service user-service/main.go
	go build -o bin/post-service post-service/main.go
	go build -o bin/chat-service chat-service/main.go
	go build -o bin/notification-service notification-service/main.go
	go build -o bin/ai-service ai-service/main.go
	go build -o bin/file-service file-service/main.go
	go build -o bin/admin-service admin-service/main.go
	@echo "====== Build Complete! Binaries placed in bin/ ======"
	@echo "====== Stopping Go Microservices ======"
	-@pkill -f "bin/api-gateway" || true
	-@pkill -f "bin/auth-service" || true
	-@pkill -f "bin/user-service" || true
	-@pkill -f "bin/post-service" || true
	-@pkill -f "bin/chat-service" || true
	-@pkill -f "bin/notification-service" || true
	-@pkill -f "bin/ai-service" || true
	-@pkill -f "bin/file-service" || true
	-@pkill -f "bin/admin-service" || true
	@echo "====== All Services Stopped! ======"
	@echo "====== Starting Go Microservices ======"
	@mkdir -p logs
	@nohup ./bin/api-gateway > /dev/null 2> logs/api-gateway.log &
	@nohup ./bin/auth-service > /dev/null 2> logs/auth-service.log &
	@nohup ./bin/user-service > /dev/null 2> logs/user-service.log &
	@nohup ./bin/post-service > /dev/null 2> logs/post-service.log &
	@nohup ./bin/chat-service > /dev/null 2> logs/chat-service.log &
	@nohup ./bin/notification-service > /dev/null 2> logs/notification-service.log &
	@nohup ./bin/ai-service > /dev/null 2> logs/ai-service.log &
	@nohup ./bin/file-service > /dev/null 2> logs/file-service.log &
	@nohup ./bin/admin-service > /dev/null 2> logs/admin-service.log &
	@echo "====== All Services Started in Background (check logs/ for output)! ======"

# Infrastructure services
infra-up:
	@echo "====== Spinning Up Infrastructure (Databases, Message Broker) ======"
	docker compose up -d

infra-down:
	@echo "====== Stopping Infrastructure ======"
	docker compose down

# Run services locally
run-gateway:
	@echo "====== Starting API Gateway on port ${PORT} ======"
	go run api-gateway/main.go

run-auth:
	@echo "====== Starting Auth Service ======"
	go run auth-service/main.go

run-user:
	@echo "====== Starting User & Graph Service ======"
	go run user-service/main.go

run-post:
	@echo "====== Starting Post & Feed Service ======"
	go run post-service/main.go

run-chat:
	@echo "====== Starting Chat & Call Service ======"
	go run chat-service/main.go

run-notif:
	@echo "====== Starting Notification Service ======"
	go run notification-service/main.go

run-ai:
	@echo "====== Starting AI & Media Service ======"
	go run ai-service/main.go

run-admin:
	@echo "====== Starting Admin Service ======"
	go run admin-service/main.go

clean:
	@echo "====== Cleaning Binaries ======"
	rm -rf bin/

dev-restart:
	@if [ -z "$(svc)" ]; then \
		echo "Error: Please specify the service using 'svc=...', e.g. 'make dev-restart svc=auth-service'"; \
		exit 1; \
	fi
	@echo "====== Rebuilding and Restarting $(svc) ======"
	go build -o bin/$(svc) $(svc)/main.go
	-@pkill -f "bin/$(svc)" || true
	@mkdir -p logs
	@nohup ./bin/$(svc) > /dev/null 2> logs/$(svc).log &
	@echo "====== $(svc) rebuilt and started in background! ======"
