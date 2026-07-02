# Project Index for AI Agents

Muc dich: day la file doc nhanh cho AI/agent moi vao repo `social-network-go` co the hieu kien truc, noi can sua, cach chay, cach test va cac canh bao an toan truoc khi trien khai thay doi.

## 1. Tong Quan

`social-network-go` la he thong mang xa hoi dang microservices viet bang Go, co frontend Next.js trong `social-network-ui/`.

Luon xem day la repo gom nhieu service doc lap:

- Public entrypoint: `api-gateway/`
- Auth/account: `auth-service/`
- User/profile/social graph: `user-service/`
- Post/comment/feed: `post-service/`
- Chat/WebSocket/WebRTC call: `chat-service/`
- File upload/download/MinIO: `file-service/`
- Notification worker/realtime: `notification-service/`
- Admin/statistics/moderation/ads: `admin-service/`
- Search: `search-service/`
- Stories: `story-service/`
- AI/Kafka content processing: `ai-service/`
- Shared protobuf contracts: `pb/`
- Shared logging/profiling: `logger/`, `profiler/`

Frontend:

- `social-network-ui/` la Next.js app, chay o port `10000`.
- Frontend goi backend qua API Gateway mac dinh `http://localhost:11111`.

## 2. Kien Truc Runtime

Request tu browser/client di qua:

```text
Next.js UI -> API Gateway :11111 -> domain services
```

Gateway dam nhiem:

- CORS
- rate limit
- JWT validation qua `auth-service` gRPC
- proxy HTTP/WebSocket toi service dich
- admin-only observability route

Backing services:

- PostgreSQL: account/auth/admin relational data
- Neo4j: social graph, users, posts, comments, files, keywords
- Redis: cache, token/OTP state, online status, signaling
- MongoDB: chat messages, call history, notifications
- Kafka: domain events
- MinIO: object storage

Kafka topics dang dung trong code:

- `user-events`: auth publish account events, notification consumes user events
- `notification-events`: post-service publish notification events, notification-service consumes
- `post-events`: ai-service consumes post events

## 3. Thu Muc Can Doc Theo Tac Vu

Khi sua mot tinh nang, doc theo thu tu nay:

1. `README.md` de nam route/command/tong quan.
2. File `main.go` cua service lien quan de xem wiring, middleware, health, dependencies.
3. `config/config.go` cua service de biet env vars va default.
4. `handler/` de xem HTTP contract.
5. `service/` de xem business logic.
6. `repository/` hoac `db/` neu co truy van DB.
7. `model/` de xem request/response/domain structs.
8. `api-gateway/router/router.go` neu can them/sua public route.
9. `pb/*.proto` va generated `pb/*.pb.go` neu thay doi gRPC contract.
10. Test gan nhat: `*_test.go` trong service lien quan.

Khong doc/noi dung cac file `.env` tru khi Ngai yeu cau ro. Cac file nay co the chua secret local.

## 4. Ban Do Service Nhanh

### API Gateway

- Path: `api-gateway/`
- Routes chinh: `api-gateway/router/router.go`
- Middleware: `api-gateway/middleware/`
- Proxy helper: `api-gateway/proxy/proxy.go`
- Dashboards: `api-gateway/handler/*dashboard.html`, `*_handler.go`
- Luu y: route public/auth/admin duoc quyet dinh tai gateway. Neu service co endpoint moi ma frontend can goi qua gateway, thuong phai them route o day.

### Auth Service

- Path: `auth-service/`
- DB: PostgreSQL qua GORM, Redis, gRPC client toi user-service
- gRPC server: `auth-service/grpc/server.go`
- Handlers: login/register/password/2FA/OAuth
- Event publisher: `auth-service/service/publisher.go` -> Kafka `user-events`

### User Service

- Path: `user-service/`
- DB: Neo4j, Redis
- gRPC server: `user-service/grpc/server.go`
- Domains: profile, friend, friend request, block
- File integration: `user-service/service/file_client.go`

### Post Service

- Path: `post-service/`
- DB: Neo4j, Redis
- gRPC server: `post-service/grpc/server.go`
- Domains: posts, comments, likes, feeds, shares
- File integration: `post-service/service/file_client.go`
- Notification publisher: `post-service/service/notification_publisher.go` -> Kafka `notification-events`

### Chat Service

- Path: `chat-service/`
- DB: MongoDB, Redis, Neo4j
- Domains: 1-1 chat, group chat, WebSocket, voice messages, WebRTC signaling/calls
- Extra doc: `docs/calling_system.md`

### File Service

- Path: `file-service/`
- Storage: MinIO
- gRPC server: `file-service/grpc/server.go`
- Domains: upload, download, presigned URLs, delete

### Notification Service

- Path: `notification-service/`
- Single-file service in `notification-service/main.go`
- Uses Kafka, MongoDB/Redis-style realtime flows depending on configured code path
- Consumes `user-events` and `notification-events`

### Admin Service

- Path: `admin-service/`
- Domains: statistics, moderation, ads, announcements, operational controls
- DB/repo layout: `db/`, `repository/`, `service/`, `handler/`

### Search Service

- Path: `search-service/`
- DB: Neo4j, Redis
- Domain: search users/content

### Story Service

- Path: `story-service/`
- DB: Neo4j, Redis
- Domain: story publish/retrieve

### AI Service

- Path: `ai-service/`
- Consumes Kafka `post-events`
- Uses Gemini key if configured
- Do not hardcode provider keys.

## 5. Shared Conventions

Backend stack:

- Go module: `social-network-go`
- Go version in `go.mod`: `1.25.0`
- HTTP framework: Gin
- gRPC: `google.golang.org/grpc`
- PostgreSQL ORM: GORM
- Neo4j driver: `neo4j-go-driver/v5`
- Redis: `redis/go-redis/v9`
- Kafka: `segmentio/kafka-go`
- MongoDB: official MongoDB Go driver
- MinIO: `minio-go/v7`

Common service pattern:

```text
config.LoadConfig()
init DB/cache/client dependencies
construct service
construct handler
setup Gin routes
register /health
register /debug/profiler guarded endpoint
r.Run(...)
```

Shared request context conventions:

- Gateway auth middleware forwards trusted user headers to downstream services.
- Handlers should validate input and return clear JSON errors with proper HTTP status.
- DB/network calls should use `context.Context` and timeout where practical.
- Do not log secrets, tokens, passwords, OAuth credentials, SMTP credentials, MinIO keys, Gemini keys.

## 6. Commands

Backend infra only:

```bash
make infra-up
make infra-down
```

Build and test backend:

```bash
make tidy
make build
make test
```

Run all Go services locally:

```bash
./start-dev.sh
./stop-dev.sh
```

Run one service:

```bash
make run-gateway
make run-auth
make run-user
make run-post
make run-chat
make run-notif
make run-ai
make run-admin
```

Restart one built binary:

```bash
make dev-restart svc=auth-service
```

Full Docker development stack:

```bash
docker compose -f docker-compose.dev.yml up --build
```

Frontend:

```bash
cd social-network-ui
npm install
npm run dev
npm run build
```

## 7. Ports

Common defaults:

- API Gateway: `11111`
- Frontend: `10000`
- Auth HTTP/gRPC: `10081` / `10051`
- User HTTP/gRPC: `10082` / `10052`
- Post HTTP: `10083`
- Chat HTTP/WebSocket: `10084`
- Notification HTTP: `10085`
- File HTTP/gRPC: `10087` / `10057`
- Admin HTTP: `10088`
- Search HTTP: `10089`
- Story HTTP: `10090`
- PostgreSQL: `5432`
- Neo4j browser/Bolt: `7474` / `7687`
- Redis: `6379`
- MongoDB: `27017`
- Kafka: `9092`
- MinIO API/console: `9000` / `9001`

## 8. Route Orientation

Gateway public routes include:

- `GET /health`
- `POST /v1/auth/login`
- `POST /v1/auth/login-admin`
- `POST /v1/auth/refresh`
- `POST /v1/auth/forgot-password`
- `POST /v1/auth/reset-password`
- `GET /v1/auth/google/login`
- `GET /v1/auth/google/callback`
- `GET /v1/announcement`
- `GET /v1/files/:id`

Authenticated route groups include:

- `/v1/auth/*`
- `/v1/users/*`
- `/v1/friends/*`
- `/v1/blocks/*`
- `/v1/friend-request/*`
- `/v1/posts/*`
- `/v1/comments/*`
- `/v1/chat/*`
- `/v1/call/*`
- `/v1/notifications/*`
- `/v1/files/*`
- `/v1/search/*`
- `/v1/stories/*`
- `/v1/ads/*`

Admin-only route groups include:

- `/v1/admin/*`
- `/v2/statistics/*`
- selected observability/debug APIs

Always confirm exact routes in `api-gateway/router/router.go` and target service handlers before editing frontend calls.

## 9. Frontend Orientation

Frontend path: `social-network-ui/`.

Important folders:

- `src/app/`: Next.js app router pages/layouts
- `src/components/social-app-component/`: domain UI components
- `src/components/ui-components/`: shared UI components
- `src/hooks/`: query/socket/domain hooks
- `src/utils/axios.js`: API client setup
- `src/utils/socket.js`: socket setup
- `src/providers/`: app providers
- `src/i18n/`, `messages/`: localization

Important dependencies:

- Next.js 15
- React 18
- TanStack Query
- Zustand
- axios
- lucide-react
- framer-motion
- next-intl
- sockjs-client / STOMP

## 10. Testing Guidance

Default backend verification:

```bash
go test ./...
```

Project Makefile uses:

```bash
go test -v -count=1 ./...
```

For service-specific changes, prefer focused test first:

```bash
go test ./auth-service/...
go test ./user-service/...
go test ./post-service/...
go test ./chat-service/...
```

Then run broader `make test` if the change touches shared packages, gateway routing, protobuf contracts, or cross-service flows.

Frontend verification:

```bash
cd social-network-ui
npm run build
```

Some integration paths require infra from `make infra-up` or full Docker stack.

## 11. Change Checklist for AI

Before editing:

- Check `git status --short`; do not overwrite unrelated user changes.
- Identify service owner folder from route/domain.
- Read service `main.go`, `config/config.go`, handler, service, model, and tests.
- If endpoint is exposed to browser, inspect `api-gateway/router/router.go`.
- If changing gRPC, update `.proto`, generated stubs, server, client callers, and tests.
- If changing data shape, check `data_structure.md` and related model/repository code.

During editing:

- Keep changes scoped to the requested task.
- Do not edit `.env` files or print secrets.
- Do not change DB schema/destructive data operations without explicit approval.
- Preserve existing response shapes unless the task explicitly asks for API change.
- Add or adjust focused tests for behavior changes.

Before final response:

- Run the smallest meaningful test command.
- If tests require Docker/DB/network and cannot run, say exactly what was skipped.
- Summarize changed files and behavioral impact.
- Mention risk if gateway route, auth, DB, Kafka, or realtime behavior changed.

## 12. Docs Worth Reading

- `README.md`: main project overview, commands, routes, observability.
- `data_structure.md`: database/data model architecture.
- `docs/calling_system.md`: WebRTC/call signaling details.
- `docs/project_review.md`: project review notes.
- `docs/microservices_util_review_vi.md`: Vietnamese microservice utility review.
- `plans/*.md`: feature plans and implementation notes.
- `INFRASTRUCTURE_ACCESS.md`: infrastructure access notes. Treat as sensitive-adjacent; do not expose secrets.

## 13. Known Safety Notes

- Local `.env` files exist in several service folders and root. Do not read or quote them by default.
- `logs/`, `bin/`, frontend build outputs, and dependency folders are local/generated.
- `pb/*.pb.go` files are committed because services import generated protobuf Go stubs.
- `docker-compose.yml` starts infra only. `docker-compose.dev.yml` starts infra plus services and Nginx gateway load balancer.
- `make infra-down` runs `docker compose down`; do not remove volumes unless explicitly requested.
- `Makefile clean` removes `bin/`; only run when requested or clearly harmless for the task.

