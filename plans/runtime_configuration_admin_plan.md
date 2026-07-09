# Runtime Configuration Admin Plan

## Overview

Hiện codebase đang có nhiều tham số vận hành bị hard-code trong service, ví dụ độ dài nội dung bài viết/bình luận, số file đính kèm tối đa, số bạn bè/block/request tối đa, page limit, rate limit gateway, timeout/TTL auth, timeout publisher, weight newsfeed ranking. Mục tiêu là chuyển các tham số này sang cấu hình runtime: DB là nguồn sự thật, Redis là cache đọc nhanh không TTL, admin có trang quản lý để cập nhật có kiểm soát và có nút sync để clear cache cũ rồi nạp config mới từ DB vào Redis.

## Current Hard-Code Inventory

### Post service

- `post-service/service/post_service.go`
  - `MaxPostContentLength = 5000`
  - `MaxPostAttachFiles = 10`
  - `MaxCommentContentLength = 1000`
  - `normalizeLimit`: default `20`, max `100`
- `post-service/service/newsfeed_scorer.go`
  - newsfeed weight: friend relationship, second-degree/requested, view profile, like/comment/share, loaded penalty
- `post-service/service/*publisher.go`
  - Kafka batch/write timeout and publish timeout

### User service

- `user-service/model/model.go`
  - `MaxFriendCount = 100`
  - `MaxBlockCount = 100`
  - `MaxSentRequestCount = 100`
  - `MaxReceivedRequestCount = 100`
  - name/username max length
  - cooldown day for name, username, birthdate
  - `MinAge = 16`

### API gateway / auth / chat

- `api-gateway/router/router.go`
  - public/auth rate limit values
- `api-gateway/middleware/rate_limit.go`
  - temporary ban threshold and ban TTL
- `auth-service/service/login.go`
  - login max attempts, lock TTL, attempt TTL
- `auth-service/config/config.go`
  - email rate limit already env-based, can be migrated later into DB-backed config
- `chat-service/service/chat_service.go`
  - message length and grpc/http timeouts

## Architecture Decisions

- DB source of truth lives in `admin-service` PostgreSQL because admin-service already owns admin APIs and has Postgres + Redis connections.
- Redis stores effective config for fast reads by all services and does not use TTL for runtime config keys.
- Admin page has a manual Sync action that clears old runtime config cache keys and rebuilds Redis from DB. This is the explicit recovery path after Redis flush, deployment, or suspected cache drift.
- Each service keeps safe built-in defaults so startup does not fail when DB/Redis is unavailable.
- Config reads must be typed and validated. Do not let services parse arbitrary strings directly at call sites.
- Admin updates are audited and versioned. Runtime updates should be reversible.
- First rollout should support integer, boolean, string, duration, and JSON config values. JSON is reserved for grouped weights like newsfeed ranking.
- Config keys use stable namespaced keys: `post.max_content_length`, `user.max_friend_count`, `gateway.public_rate_limit_per_minute`.
- Backward compatibility: existing hard-coded constants remain as defaults during migration, then call sites move to config provider one-by-one.

## Proposed Data Model

### PostgreSQL table: `runtime_configs`

Fields:
- `key` text primary key
- `scope` text not null, examples: `post-service`, `user-service`, `api-gateway`, `global`
- `type` text not null, one of `INT`, `BOOL`, `STRING`, `DURATION`, `JSON`
- `value` text not null
- `default_value` text not null
- `description` text
- `category` text not null, examples: `limits`, `rate_limits`, `newsfeed`, `timeouts`, `security`
- `is_sensitive` boolean default false
- `is_editable` boolean default true
- `validation_json` jsonb, examples: `{ "min": 1, "max": 10000 }`, `{ "enum": ["NONE", "DAILY", "WEEKLY"] }`
- `version` bigint not null default 1
- `updated_by` text
- `created_at`, `updated_at`

### PostgreSQL table: `runtime_config_audits`

Fields:
- `id` uuid primary key
- `key` text not null
- `old_value` text
- `new_value` text
- `version` bigint not null
- `updated_by` text
- `reason` text
- `created_at`

### Redis keys

- `runtime_config:all` hash: field is config key, value is serialized typed value envelope. No TTL.
- `runtime_config:version` string/integer: global version. No TTL.
- Optional per-service hash: `runtime_config:{scope}` for smaller reads. No TTL.
- Pub/sub channel: `runtime_config:changed` with payload `{ key, scope, version }`.
- Sync lock key: `runtime_config:sync_lock`, short TTL only for lock safety, not for config data.

Sync behavior:
- Admin calls sync from the UI.
- Admin-service acquires `runtime_config:sync_lock`.
- Admin-service deletes only runtime config cache keys: `runtime_config:all`, `runtime_config:version`, `runtime_config:{scope}`.
- Admin-service reloads all active config rows from DB.
- Admin-service writes fresh Redis hashes/keys without TTL.
- Admin-service publishes `runtime_config:synced` with `{ version, syncedBy, syncedAt }`.
- Services keep their local fallback snapshot while sync is running and refresh after event or next local refresh.

## API Contract

All admin APIs stay behind the existing gateway admin guard.

### List configs

`GET /v1/admin/runtime-configs?scope=post-service&category=limits&q=max`

Response body:

```json
{
  "items": [
    {
      "key": "post.max_content_length",
      "scope": "post-service",
      "category": "limits",
      "type": "INT",
      "value": "5000",
      "defaultValue": "5000",
      "description": "Maximum characters allowed in a post",
      "validation": { "min": 1, "max": 20000 },
      "version": 3,
      "updatedBy": "admin-1",
      "updatedAt": "2026-07-06T10:00:00+07:00"
    }
  ]
}
```

### Update config

`PATCH /v1/admin/runtime-configs/:key`

Request:

```json
{
  "value": "8000",
  "reason": "Increase post length after product review",
  "expectedVersion": 3
}
```

Behavior:
- Validate type and validation range.
- Reject stale `expectedVersion` with conflict.
- Write DB transaction.
- Write audit row.
- Refresh Redis key/hash.
- Publish `runtime_config:changed`.

### Reset config

`POST /v1/admin/runtime-configs/:key/reset`

Behavior:
- Reset `value` to `default_value`.
- Audit like normal update.
- Refresh Redis and publish change.

### Sync Redis cache

`POST /v1/admin/runtime-configs/sync`

Request:

```json
{
  "reason": "Manual sync after changing config values"
}
```

Behavior:
- Admin-only.
- Clear old runtime config cache from Redis.
- Load all active configs from DB.
- Write complete config snapshot into Redis with no TTL.
- Publish `runtime_config:synced`.
- Return counts and version.

Response:

```json
{
  "synced": true,
  "configCount": 42,
  "version": 18,
  "clearedKeys": ["runtime_config:all", "runtime_config:version", "runtime_config:post-service"],
  "syncedAt": "2026-07-06T10:00:00+07:00"
}
```

### Service-side internal endpoint, optional

`GET /internal/runtime-configs?scope=post-service`

Only needed if a service cannot access Redis directly. Prefer Redis direct read first because all services already have Redis configuration.

## Config Provider Interface

Create shared package:

- `internal/runtimeconfig`

Suggested API:

```go
type Provider interface {
    Int(ctx context.Context, key string, fallback int) int
    Bool(ctx context.Context, key string, fallback bool) bool
    String(ctx context.Context, key string, fallback string) string
    Duration(ctx context.Context, key string, fallback time.Duration) time.Duration
    JSON(ctx context.Context, key string, fallback any, out any) error
    Refresh(ctx context.Context) error
}
```

Provider behavior:
- Read Redis first.
- If Redis miss, use in-memory defaults immediately and optionally trigger async refresh.
- Keep a local in-memory snapshot with a short refresh interval to avoid Redis call on every validation. Redis config keys themselves do not expire.
- Subscribe to `runtime_config:changed` / `runtime_config:synced` when practical; otherwise refresh local snapshot periodically.
- Never block critical request path on DB.
- Log config parse errors without exposing sensitive values.

## Task List

### Phase 1: Inventory and schema foundation

#### Task 1: Build complete config inventory

**Description:** Scan service constants and classify which values should become runtime config, env config, or remain code constants.

**Acceptance criteria:**
- [ ] Inventory document lists key, current value, service, call sites, type, default, range, category.
- [ ] Non-runtime constants are explicitly excluded with reason.
- [ ] First migration batch is selected.

**Verification:**
- [ ] `rg` checks cover service constants in `auth-service`, `user-service`, `post-service`, `api-gateway`, `chat-service`.
- [ ] Inventory reviewed against current code.

**Files likely touched:**
- `plans/runtime_configuration_inventory.md`

**Estimated scope:** M

#### Task 2: Add runtime config PostgreSQL models and migration

**Description:** Add `RuntimeConfig` and `RuntimeConfigAudit` models in admin-service and include them in admin-service AutoMigrate.

**Acceptance criteria:**
- [ ] `runtime_configs` table exists.
- [ ] `runtime_config_audits` table exists.
- [ ] Unique key constraint exists.
- [ ] Audit model can record old/new value.

**Verification:**
- [ ] `go test ./admin-service/...`
- [ ] Local admin-service startup migrates without error.

**Files likely touched:**
- `admin-service/model/runtime_config.go`
- `admin-service/db/db.go`

**Estimated scope:** S-M

#### Task 3: Seed initial config defaults

**Description:** Seed runtime config rows for the first batch using existing hard-coded values as defaults.

**Initial keys:**
- `post.max_content_length = 5000`
- `post.max_attach_files = 10`
- `post.max_comment_content_length = 1000`
- `post.default_page_limit = 20`
- `post.max_page_limit = 100`
- `user.max_friend_count = 100`
- `user.max_block_count = 100`
- `user.max_sent_request_count = 100`
- `user.max_received_request_count = 100`
- `user.max_given_name_length = 64`
- `user.max_family_name_length = 64`
- `user.max_username_length = 32`
- `user.min_age = 16`
- `newsfeed.score_weights` JSON, matching current scorer defaults

**Acceptance criteria:**
- [ ] Seed is idempotent.
- [ ] Existing values are preserved when rows already exist.
- [ ] Default values match current code behavior.

**Verification:**
- [ ] `go test ./admin-service/...`
- [ ] Manual DB check shows seeded rows.

**Files likely touched:**
- `admin-service/service/runtime_config_seed.go`
- `admin-service/db/db.go`

**Estimated scope:** M

### Checkpoint: DB source of truth ready

- [ ] Admin-service starts with config tables.
- [ ] Seeded defaults match current behavior.
- [ ] No consuming service behavior changed yet.

### Phase 2: Admin API and Redis cache

#### Task 4: Implement admin repository/service for runtime configs

**Description:** Add list/get/update/reset operations with validation, version conflict checks, Redis sync, and audit write.

**Acceptance criteria:**
- [ ] List supports scope/category/search.
- [ ] Update validates type and range.
- [ ] Update requires `expectedVersion`.
- [ ] Update writes audit row.
- [ ] Redis cache updates after DB commit.
- [ ] Reset restores default.

**Verification:**
- [ ] Unit tests for validation and version conflict.
- [ ] `go test ./admin-service/...`

**Files likely touched:**
- `admin-service/repository/runtime_config_repository.go`
- `admin-service/service/runtime_config.go`
- `admin-service/service/runtime_config_test.go`

**Estimated scope:** M

#### Task 5: Expose admin runtime config API

**Description:** Add admin-service handlers and route them through existing gateway admin-only routes.

**Acceptance criteria:**
- [ ] `GET /v1/admin/runtime-configs` works through gateway.
- [ ] `PATCH /v1/admin/runtime-configs/:key` works through gateway.
- [ ] `POST /v1/admin/runtime-configs/:key/reset` works through gateway.
- [ ] Non-admin users are forbidden by gateway.

**Verification:**
- [ ] `go test ./admin-service/... ./api-gateway/...`
- [ ] Manual curl with admin token.
- [ ] Manual curl with user token returns 403.

**Files likely touched:**
- `admin-service/handler/runtime_config_handler.go`
- `admin-service/handler/handler.go`
- `api-gateway/router/router.go` if explicit route is needed; existing `/v1/admin/*any` may already cover it.

**Estimated scope:** S-M

#### Task 6: Add Redis read contract and manual sync behavior

**Description:** Store effective config in Redis with stable serialization and no TTL. Add admin sync endpoint that clears old runtime config cache keys and rebuilds Redis from DB.

**Acceptance criteria:**
- [ ] Redis contains all active runtime configs after admin-service startup.
- [ ] Runtime config Redis keys are written without TTL.
- [ ] Update/reset refreshes the affected Redis config value without setting TTL.
- [ ] Sync operation clears old runtime config keys and rebuilds Redis from DB.
- [ ] Sync is protected by a short-lived lock to avoid concurrent rebuilds.
- [ ] Config changed event is published.
- [ ] Config synced event is published.

**Verification:**
- [ ] `go test ./admin-service/...`
- [ ] Manual Redis check: `HGET runtime_config:all post.max_content_length`
- [ ] Manual Redis check: `TTL runtime_config:all` returns `-1`
- [ ] Manual sync: update DB row, run sync endpoint, verify Redis reflects DB value.

**Files likely touched:**
- `admin-service/service/runtime_config_cache.go`
- `admin-service/handler/runtime_config_handler.go`

**Estimated scope:** M

### Checkpoint: Admin backend ready

- [ ] Admin API can list/update/reset configs.
- [ ] Admin API can sync DB config into Redis on demand.
- [ ] Redis reflects DB values.
- [ ] Redis runtime config keys have no TTL.
- [ ] Audit records exist.
- [ ] Existing service behavior is still unchanged.

### Phase 3: Shared config provider and first service integration

#### Task 7: Create shared runtime config provider package

**Description:** Implement `internal/runtimeconfig` provider with Redis-first read, local cache, fallback defaults, typed getters, and refresh.

**Acceptance criteria:**
- [ ] Typed getters exist for int/bool/string/duration/json.
- [ ] Redis miss returns fallback safely.
- [ ] Invalid cached value returns fallback and logs warning.
- [ ] Local cache prevents per-request Redis hits.

**Verification:**
- [ ] `go test ./internal/runtimeconfig/...`

**Files likely touched:**
- `internal/runtimeconfig/provider.go`
- `internal/runtimeconfig/provider_test.go`

**Estimated scope:** M

#### Task 8: Integrate post-service limits

**Description:** Replace post-service validation limits with runtime config provider while keeping existing constants as fallback defaults.

**Acceptance criteria:**
- [ ] Post content length reads `post.max_content_length`.
- [ ] Attach file max reads `post.max_attach_files`.
- [ ] Comment content length reads `post.max_comment_content_length`.
- [ ] Page limit reads `post.default_page_limit` and `post.max_page_limit`.
- [ ] If Redis unavailable, existing behavior remains.

**Verification:**
- [ ] `go test ./post-service/...`
- [ ] Manual update config then create post/comment around boundary values.

**Files likely touched:**
- `post-service/service/post_service.go`
- `post-service/service/post.go`
- `post-service/service/comment.go`
- `post-service/main.go`

**Estimated scope:** M

#### Task 9: Integrate newsfeed scorer weights

**Description:** Load `newsfeed.score_weights` JSON from runtime config and pass effective weights into scorer.

**Acceptance criteria:**
- [ ] Existing default weights remain fallback.
- [ ] Admin can change weight without code deploy.
- [ ] Invalid JSON falls back to default weights.
- [ ] Score breakdown shows effective components.

**Verification:**
- [ ] `go test ./post-service/service`
- [ ] Manual update weight and verify debug score breakdown changes.

**Files likely touched:**
- `post-service/service/newsfeed_scorer.go`
- `post-service/service/post.go`
- `post-service/service/newsfeed_scorer_test.go`

**Estimated scope:** M

#### Task 10: Integrate user-service limits

**Description:** Replace friend/block/request/name/min-age/cooldown hard-coded limits with runtime config provider.

**Acceptance criteria:**
- [ ] Friend count max reads `user.max_friend_count`.
- [ ] Block count max reads `user.max_block_count`.
- [ ] Sent/received request max read runtime config.
- [ ] Name/username length validations use runtime config.
- [ ] Min age and cooldown days use runtime config.
- [ ] Existing defaults remain fallback.

**Verification:**
- [ ] `go test ./user-service/...`
- [ ] Manual update friend limit and send request around boundary.

**Files likely touched:**
- `user-service/model/model.go`
- `user-service/service/*.go`
- `user-service/handler/*.go`
- `user-service/main.go`

**Estimated scope:** M-L

### Checkpoint: Core services consume runtime config

- [ ] Post limits configurable from admin.
- [ ] User limits configurable from admin.
- [ ] Newsfeed weights configurable from admin.
- [ ] Redis outage does not break requests.

### Phase 4: Admin UI

#### Task 11: Add admin dashboard navigation item

**Description:** Add “Cấu hình hệ thống” to admin sidebar and title mapping.

**Acceptance criteria:**
- [ ] Sidebar links to `/admin/dashboard/runtime-configs`.
- [ ] Active state works.
- [ ] Page title/subtitle match admin style.

**Verification:**
- [ ] `npm run lint` or existing UI check.
- [ ] Manual browser check.

**Files likely touched:**
- `social-network-ui/src/app/admin/dashboard/layout.js`

**Estimated scope:** S

#### Task 12: Build runtime config list page

**Description:** Create admin UI to list configs, filter by service/category, search by key/description, and show current/default value/version.

**Acceptance criteria:**
- [ ] Loads configs from `/v1/admin/runtime-configs`.
- [ ] Filters by scope/category.
- [ ] Search works client-side or server-side.
- [ ] Loading/error/empty states exist.
- [ ] Values marked sensitive are masked.

**Verification:**
- [ ] Manual browser check.
- [ ] Existing frontend lint/build if available.

**Files likely touched:**
- `social-network-ui/src/app/admin/dashboard/runtime-configs/page.js`

**Estimated scope:** M

#### Task 13: Add edit/reset/sync workflow

**Description:** Allow admin to edit config values with type-aware controls, reset values to default, and manually sync DB config into Redis after clearing old cache.

**Acceptance criteria:**
- [ ] INT uses numeric input with min/max hints.
- [ ] BOOL uses switch.
- [ ] STRING/DURATION use text input.
- [ ] JSON uses textarea with JSON validation.
- [ ] Update sends `expectedVersion`.
- [ ] Conflict error asks admin to reload.
- [ ] Reset asks confirmation.
- [ ] Page has a “Sync Redis” button.
- [ ] Sync button asks confirmation and explains it will clear old runtime config cache then reload from DB.
- [ ] Sync success shows config count/version/synced time.
- [ ] Sync failure shows actionable error and does not mutate UI optimistically.

**Verification:**
- [ ] Manual edit/reset happy path.
- [ ] Manual stale version conflict.
- [ ] Manual sync after editing DB/config, then verify Redis has new value and no TTL.

**Files likely touched:**
- `social-network-ui/src/app/admin/dashboard/runtime-configs/page.js`

**Estimated scope:** M

### Checkpoint: Admin can operate configs

- [ ] Admin can view, filter, edit, reset configs.
- [ ] Admin can click Sync Redis to clear old cache and reload DB config into Redis.
- [ ] Audit row is created after UI edit.
- [ ] Redis value changes after UI edit.
- [ ] Redis value changes after manual sync.

### Phase 5: Rollout, observability, and cleanup

#### Task 14: Add audit/history UI

**Description:** Add per-key audit history drawer/table so admins can see who changed what and why.

**Acceptance criteria:**
- [ ] UI shows old/new value, version, updated by, reason, timestamp.
- [ ] Sensitive values are masked.
- [ ] API supports paging history.

**Verification:**
- [ ] `go test ./admin-service/...`
- [ ] Manual UI check.

**Estimated scope:** M

#### Task 15: Add observability for config usage

**Description:** Add profiler/log/metrics around config cache hit/miss, parse errors, Redis refresh errors.

**Acceptance criteria:**
- [ ] Profiler or logs show config cache miss/error.
- [ ] Invalid runtime config is visible without breaking request path.
- [ ] Admin update logs key/version, not sensitive values.

**Verification:**
- [ ] `go test ./admin-service/... ./post-service/... ./user-service/...`
- [ ] Manual invalid config value test.

**Estimated scope:** S-M

#### Task 16: Migrate remaining service constants

**Description:** After first batch is stable, migrate gateway/auth/chat/notification timeout and rate-limit knobs selectively.

**Acceptance criteria:**
- [ ] Gateway public/auth rate limits configurable.
- [ ] Gateway ban TTL/threshold configurable.
- [ ] Auth login attempt lock config configurable.
- [ ] Chat message limits configurable.
- [ ] Only runtime-safe configs are migrated; deployment/env secrets stay env-based.

**Verification:**
- [ ] `go test ./api-gateway/... ./auth-service/... ./chat-service/...`
- [ ] Manual rate limit boundary checks.

**Estimated scope:** L, split into smaller service-specific tasks when starting implementation.

#### Task 17: Remove obsolete hard-coded reads

**Description:** Once all call sites use provider fallback defaults, remove direct business validation dependency on old constants where safe.

**Acceptance criteria:**
- [ ] Constants remain only as default values or tests.
- [ ] No duplicated values across services and seed data without a single source-of-default.
- [ ] Inventory doc marks all first-batch keys migrated.

**Verification:**
- [ ] `rg` checks for migrated constant usages.
- [ ] Full service tests for touched services.

**Estimated scope:** M

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Redis outage causes request failures | High | Provider always falls back to local snapshot/defaults |
| Bad admin config breaks production behavior | High | Type/range validation, expected version, audit, reset |
| Config reads add latency | Medium | Local in-memory cache with TTL and pub/sub refresh |
| Too many configs make UI noisy | Medium | Category/scope filters, descriptions, first-batch migration |
| Secrets accidentally moved to DB | High | `is_sensitive` masking and explicit rule: secrets remain env/secret manager |
| Services diverge in config parsing | Medium | Shared `internal/runtimeconfig` provider |
| AutoMigrate changes production schema unexpectedly | Medium | Review deployment policy; add explicit migration path if needed |

## Open Questions

- Ngài muốn DB source of truth đặt ở PostgreSQL của `admin-service` như plan này, hay muốn Neo4j node `SystemConfig` để đồng bộ cùng graph data?
- Có cần per-environment/per-tenant config không, hay hiện tại chỉ cần global config?
- Có cần approval workflow hai bước cho config nguy hiểm như rate limit/security không?
- Runtime config update có cần hot-reload ngay qua Redis pub/sub, hay chấp nhận local cache TTL 30-60 giây?
- Các key nào là first batch bắt buộc ngoài post/user/newsfeed limits?

## Suggested Implementation Order

1. Task 1-3: Inventory + DB schema + seed.
2. Task 4-6: Admin backend API + Redis cache.
3. Task 7-9: Shared provider + post-service + newsfeed scorer.
4. Task 10: User-service limits.
5. Task 11-13: Admin UI list/edit/reset.
6. Task 14-17: Audit UI, observability, broader rollout, cleanup.
