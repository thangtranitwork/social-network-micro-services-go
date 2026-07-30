# Implementation Plan: Feature Flags, Targeting, and A/B Testing

## Overview

Build a feature-control layer that lets admins turn features on/off across backend and frontend, target specific users, and run deterministic A/B tests. This should reuse the runtime-configuration foundation already in `admin-service`: PostgreSQL as source of truth, Redis as explicit admin-synced cache, and no TTL for feature state. Runtime config remains for typed operational values; feature flags become a separate domain because targeting, variants, exposure logging, and user overrides are different concerns.

## Goals

- Admin can create, edit, enable, disable, and archive feature flags.
- Admin can restrict a feature to specific users or groups before broad rollout.
- Backend services can evaluate a flag with `userID` and request context.
- Frontend can ask which feature gates are enabled for the current user.
- A/B experiments use stable user assignment and record exposure events.
- Redis cache updates only when admin syncs, matching the runtime-config behavior.

## Non-Goals for First Release

- No real-time experimentation dashboard with full BI slicing.
- No automatic statistical significance engine.
- No PII-heavy targeting rules such as raw email domains unless explicitly approved.
- No direct feature evaluation from unauthenticated public clients without backend mediation.

## Architecture Decisions

- **Separate tables from `runtime_configs`:** Feature flags need rules, variants, overrides, and exposure metadata. Keeping them separate avoids turning runtime config JSON into an unreviewable rules engine.
- **Admin-service owns source of truth:** It already owns admin routes, PostgreSQL, Redis sync, and audit history.
- **Redis is the hot path:** Backend services and gateway evaluate from Redis-backed local cache, with safe defaults if Redis is unavailable.
- **Deterministic bucketing:** Use stable hash of `featureKey + salt + userID` for rollout percentage and variant selection. This prevents users from jumping variants between sessions.
- **Privacy-first targeting:** Store exact user ID overrides; avoid sensitive profile attributes unless a concrete feature needs them.
- **Exposure logging is explicit:** Only record an exposure when code actually branches on a flag or variant, not merely when the admin page lists flags.

## Data Model

### `feature_flags`

Stores one flag or experiment.

Suggested columns:

- `key` string primary key, lowercase namespace format, for example `newsfeed.new_ranker`.
- `name` string.
- `description` text.
- `scope` string, for example `post-service`, `user-service`, `frontend`, `global`.
- `category` string, for example `release`, `experiment`, `ops`, `permission`.
- `status` enum: `DRAFT`, `ACTIVE`, `PAUSED`, `ARCHIVED`.
- `default_enabled` bool.
- `rollout_percentage` int, 0-100.
- `salt` string for deterministic hashing.
- `rules_json` jsonb for coarse rules such as roles, service scopes, or allow authenticated only.
- `variants_json` jsonb for A/B variants, for example `[{ "key": "control", "weight": 50 }, { "key": "treatment", "weight": 50 }]`.
- `fallback_variant` string.
- `version` int64.
- `created_by`, `updated_by`.
- `created_at`, `updated_at`.

### `feature_flag_user_overrides`

Stores exact user allow/deny/variant overrides.

Suggested columns:

- `id` string primary key.
- `feature_key` string indexed.
- `user_id` string indexed.
- `enabled` nullable bool. `true` forces on, `false` forces off, `null` only pins variant.
- `variant` nullable string.
- `reason` text.
- `expires_at` nullable timestamp.
- `created_by`, `updated_by`.
- `created_at`, `updated_at`.

Unique index: `(feature_key, user_id)`.

### `feature_flag_audits`

Stores every admin change.

Suggested columns:

- `id` string primary key.
- `feature_key` string indexed.
- `action` string: `CREATE`, `UPDATE`, `PAUSE`, `ARCHIVE`, `SYNC`, `OVERRIDE_ADD`, `OVERRIDE_REMOVE`.
- `old_value` jsonb.
- `new_value` jsonb.
- `version` int64.
- `actor_id`.
- `reason`.
- `created_at`.

### `feature_flag_exposures`

Stores experiment exposure events.

Suggested columns:

- `id` string primary key.
- `feature_key` string indexed.
- `user_id_hash` string indexed.
- `variant` string.
- `enabled` bool.
- `reason` string.
- `version` int64.
- `surface` string, for example `backend:post-service` or `frontend:newsfeed`.
- `request_id` string nullable.
- `created_at`.

Use a hash for user ID in exposure logs unless raw user ID is explicitly needed for debugging.

## Redis Design

Keep no TTL on feature state, same as runtime config.

- `feature_flags:all`: hash, field is flag key, value is serialized flag envelope.
- `feature_flags:version`: string/integer global version.
- `feature_flags:targets:{featureKey}:allow`: set of user IDs forced on.
- `feature_flags:targets:{featureKey}:deny`: set of user IDs forced off.
- `feature_flags:targets:{featureKey}:variant:{variant}`: set of user IDs pinned to a variant.
- `feature_flags:sync_lock`: short TTL only for sync safety.
- `feature_flags:changed` / `feature_flags:synced`: pub/sub channels.

The sync endpoint rebuilds all `feature_flags:*` cache keys from PostgreSQL.

## Evaluation Contract

### Go API

Add package `internal/featuregate`.

```go
type Context struct {
	UserID string
	Role   string
	Scope  string
}

type Decision struct {
	Key     string
	Enabled bool
	Variant string
	Reason  string
	Version int64
}

type Evaluator interface {
	IsEnabled(ctx context.Context, key string, input Context, fallback bool) Decision
	Variant(ctx context.Context, key string, input Context, fallbackVariant string) Decision
}
```

Evaluation order:

1. Missing flag or Redis error returns fallback.
2. `ARCHIVED` or `PAUSED` returns disabled unless explicit fallback is used by caller.
3. User deny override wins.
4. User allow override wins.
5. User pinned variant wins if variant exists.
6. Rules must match.
7. Rollout percentage applies by deterministic hash.
8. Variant is assigned by deterministic weighted bucket.

### Frontend API

Add user-facing endpoint through gateway:

`GET /v1/features`

Response:

```json
{
  "features": {
    "newsfeed.new_ranker": {
      "enabled": true,
      "variant": "treatment",
      "version": 7
    }
  }
}
```

Only return frontend-safe flags. Do not expose backend-only operational gates.

### Admin APIs

- `GET /v1/admin/feature-flags`
- `POST /v1/admin/feature-flags`
- `GET /v1/admin/feature-flags/:key`
- `PATCH /v1/admin/feature-flags/:key`
- `POST /v1/admin/feature-flags/:key/pause`
- `POST /v1/admin/feature-flags/:key/archive`
- `GET /v1/admin/feature-flags/:key/overrides`
- `POST /v1/admin/feature-flags/:key/overrides`
- `DELETE /v1/admin/feature-flags/:key/overrides/:userId`
- `POST /v1/admin/feature-flags/sync`

## Task List

### Phase 1: Backend Foundation

#### Task 1: Define feature flag models and migrations

**Description:** Add PostgreSQL models for flags, user overrides, audits, and exposures in `admin-service`.

**Acceptance criteria:**
- [ ] AutoMigrate creates all feature flag tables.
- [ ] Unique index prevents duplicate `(feature_key, user_id)` overrides.
- [ ] Models serialize JSON rules and variants consistently.

**Verification:**
- [ ] `go test ./admin-service/...`
- [ ] Manual DB check confirms tables exist after admin-service boot.

**Dependencies:** None.

**Files likely touched:**
- `admin-service/model/feature_flag.go`
- `admin-service/db/db.go`

**Estimated scope:** M.

#### Task 2: Implement admin repository and audit writes

**Description:** Add repository operations for listing, creating, updating, pausing, archiving, and managing overrides.

**Acceptance criteria:**
- [ ] Every mutation writes an audit row.
- [ ] Updates use expected version to prevent lost updates.
- [ ] Archive does not hard delete flags.

**Verification:**
- [ ] `go test ./admin-service/repository ./admin-service/service`

**Dependencies:** Task 1.

**Files likely touched:**
- `admin-service/repository/feature_flag_repository.go`
- `admin-service/service/feature_flag.go`
- `admin-service/service/feature_flag_test.go`

**Estimated scope:** M.

#### Task 3: Add Redis sync for feature flags

**Description:** Mirror feature flag state to Redis with no TTL and explicit admin sync, matching runtime config.

**Acceptance criteria:**
- [ ] `POST /v1/admin/feature-flags/sync` rebuilds all feature flag Redis keys.
- [ ] Redis state has no TTL except the sync lock.
- [ ] Sync publishes `feature_flags:synced`.

**Verification:**
- [ ] `go test ./admin-service/service`
- [ ] Manual Redis check: `TTL feature_flags:all` returns `-1`.

**Dependencies:** Task 2.

**Files likely touched:**
- `admin-service/service/feature_flag_cache.go`
- `admin-service/handler/feature_flag_handler.go`

**Estimated scope:** M.

### Checkpoint: Foundation

- [ ] `go test ./admin-service/...` passes.
- [ ] Admin can create a flag and sync it into Redis.
- [ ] Audit rows are created for create/update/sync.

### Phase 2: Evaluation Library and Backend Consumption

#### Task 4: Add `internal/featuregate` evaluator

**Description:** Build a shared evaluator that loads Redis-backed flag envelopes, applies user overrides, rollout percentage, and variant assignment.

**Acceptance criteria:**
- [ ] Missing Redis state returns caller fallback.
- [ ] User deny beats allow, and explicit override beats rollout.
- [ ] Percentage and variant assignment are deterministic for the same user.

**Verification:**
- [ ] `go test ./internal/featuregate`

**Dependencies:** Task 3.

**Files likely touched:**
- `internal/featuregate/evaluator.go`
- `internal/featuregate/evaluator_test.go`

**Estimated scope:** M.

#### Task 5: Wire evaluator into backend services

**Description:** Add feature evaluator dependencies to services that need feature control first, starting with `post-service` and `user-service`.

**Acceptance criteria:**
- [ ] Each service has a local helper like `s.featureEnabled(ctx, key, userID, fallback)`.
- [ ] Backend-only flags are evaluated server-side, never trusted from frontend.
- [ ] Redis failures degrade to explicit fallback behavior.

**Verification:**
- [ ] `go test ./post-service/... ./user-service/...`

**Dependencies:** Task 4.

**Files likely touched:**
- `post-service/service/post_service.go`
- `user-service/service/user_service.go`
- selected service files using first gated behavior.

**Estimated scope:** M.

#### Task 6: Add first backend-gated feature slice

**Description:** Choose one low-risk feature path and gate it end-to-end, for example `newsfeed.new_ranker` or `post.ai_moderation_v2`.

**Acceptance criteria:**
- [ ] Disabled flag uses current behavior.
- [ ] Enabled flag uses new behavior only for targeted users or rollout bucket.
- [ ] Logs include flag key, enabled state, variant, and version without sensitive data.

**Verification:**
- [ ] Unit tests for off/on/targeted user paths.
- [ ] Manual API check with two users assigned to different decisions.

**Dependencies:** Task 5.

**Files likely touched:** Depends on selected feature.

**Estimated scope:** M.

### Checkpoint: Backend Evaluation

- [ ] `go test ./...` passes.
- [ ] A backend feature can be enabled for one user only.
- [ ] A backend feature can be rolled out by percentage.

### Phase 3: Frontend Evaluation and Admin UI

#### Task 7: Add frontend-safe feature API

**Description:** Expose current user's frontend-safe feature decisions through the gateway.

**Acceptance criteria:**
- [ ] Authenticated user gets a map of allowed frontend flags.
- [ ] Backend-only flags are filtered out.
- [ ] Response includes `enabled`, `variant`, and `version`.

**Verification:**
- [ ] `go test ./api-gateway/... ./admin-service/...`
- [ ] Manual request with user token.

**Dependencies:** Task 4.

**Files likely touched:**
- `api-gateway/router/router.go`
- `api-gateway/proxy/*`
- `admin-service/handler/feature_flag_handler.go`

**Estimated scope:** M.

#### Task 8: Add frontend feature provider

**Description:** Add a small client-side provider/hook so UI code can ask `isEnabled("feature.key")` or `variant("feature.key")`.

**Acceptance criteria:**
- [ ] Provider fetches `/v1/features` after auth is ready.
- [ ] Missing flag returns safe fallback.
- [ ] UI does not flicker into restricted features before fetch completes.

**Verification:**
- [ ] Frontend build succeeds.
- [ ] Manual browser check for disabled, enabled, and variant states.

**Dependencies:** Task 7.

**Files likely touched:**
- `social-network-ui/src/contexts/FeatureFlagContext.*`
- `social-network-ui/src/hooks/useFeatureFlag.*`
- app layout/provider wiring.

**Estimated scope:** M.

#### Task 9: Build admin feature flag UI

**Description:** Add admin screens for feature list, create/edit drawer, rollout controls, variant editor, user overrides, and sync status.

**Acceptance criteria:**
- [ ] Admin can create a boolean flag.
- [ ] Admin can create an experiment with weighted variants.
- [ ] Admin can add/remove specific user overrides.
- [ ] Admin can sync Redis and see last sync result.

**Verification:**
- [ ] Frontend build succeeds.
- [ ] Manual admin flow works through gateway.

**Dependencies:** Task 3.

**Files likely touched:**
- `social-network-ui/src/app/admin/dashboard/layout.js`
- `social-network-ui/src/app/admin/dashboard/feature-flags/page.js`

**Estimated scope:** L, split into list/create/override sub-tasks if needed.

### Checkpoint: Full Admin Flow

- [ ] Admin creates flag.
- [ ] Admin enables only one user.
- [ ] User sees frontend flag enabled.
- [ ] Another user sees frontend flag disabled.
- [ ] Backend respects the same decision.

### Phase 4: A/B Testing and Observability

#### Task 10: Record exposure events

**Description:** Add explicit exposure recording for backend and frontend decisions when code branches on a flag.

**Acceptance criteria:**
- [ ] Exposure events are deduplicated per request where practical.
- [ ] User ID is hashed in exposure logs.
- [ ] Exposure includes key, variant, version, surface, and reason.

**Verification:**
- [ ] Unit tests for exposure payload shape.
- [ ] Manual DB check after exercising a gated flow.

**Dependencies:** Task 6 and Task 8.

**Files likely touched:**
- `admin-service/repository/feature_exposure_repository.go`
- `internal/featuregate`
- frontend feature provider or analytics helper.

**Estimated scope:** M.

#### Task 11: Add experiment summary endpoint

**Description:** Provide a basic admin summary for exposure counts by variant and version.

**Acceptance criteria:**
- [ ] Admin can view exposure counts grouped by variant.
- [ ] Endpoint supports time range filters.
- [ ] Query uses indexes and avoids loading raw events into memory.

**Verification:**
- [ ] `go test ./admin-service/...`
- [ ] Manual API check with seeded exposures.

**Dependencies:** Task 10.

**Files likely touched:**
- `admin-service/repository/feature_exposure_repository.go`
- `admin-service/service/feature_experiment.go`
- `admin-service/handler/feature_flag_handler.go`

**Estimated scope:** M.

#### Task 12: Add metrics and logs

**Description:** Emit low-cardinality metrics for feature decisions and sync outcomes.

**Acceptance criteria:**
- [ ] Metrics include decision counts by feature key, enabled state, and variant.
- [ ] Logs include version and reason.
- [ ] No raw PII in logs.

**Verification:**
- [ ] `go test ./...`
- [ ] Manual profiler/metrics check if available.

**Dependencies:** Task 4.

**Files likely touched:**
- `profiler/profiler.go`
- `internal/featuregate`
- selected service callers.

**Estimated scope:** S-M.

### Final Checkpoint

- [ ] `go test ./...` passes.
- [ ] Frontend build passes.
- [ ] Admin can create, edit, pause, and sync flags.
- [ ] Specific user targeting works for BE and FE.
- [ ] Percentage rollout is stable per user.
- [ ] A/B variants are stable per user.
- [ ] Exposure events are recorded only when a feature decision is consumed.

## Suggested First Flags

| Feature key | Scope | Type | Why first |
|---|---|---|---|
| `newsfeed.new_ranker` | `post-service` | A/B experiment | Good candidate because ranking can be compared by variant. |
| `post.ai_moderation_v2` | `post-service` | Boolean rollout | Backend-only gate with clear fallback. |
| `frontend.runtime_config_v2` | `frontend` | Boolean rollout | Low-risk frontend-only gate for admin UX improvements. |

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---:|---|
| Redis unavailable makes gates unpredictable | High | Every call must pass explicit fallback; evaluator returns fallback on miss/error. |
| Users switch variants across sessions | High | Deterministic hash with stable salt and user ID. |
| Admin accidentally exposes backend-only flag to frontend | Medium | Add `scope`/`frontendSafe` validation and filter `/v1/features`. |
| A/B metrics over-count exposures | Medium | Record exposure only on consumed decisions, include request ID, and dedupe per request when practical. |
| Targeting grows into PII rules | Medium | Start with exact user IDs and role/scope only; require explicit review for more attributes. |
| Too much logic hidden in JSON | Medium | Keep JSON schema small and validate known fields in service. |

## Open Questions for Ngài

- First gated feature should be `newsfeed.new_ranker`, `post.ai_moderation_v2`, or a frontend-only feature?
- For user targeting, should admin search users by username/email, or only paste exact user IDs in phase 1?
- Should exposure logs keep raw `user_id` for debugging, or only hashed IDs from day one?
- Should percentage rollout apply to anonymous users, or only authenticated users?

