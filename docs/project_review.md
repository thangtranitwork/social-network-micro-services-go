# Project Review

Date: 2026-06-30

Scope: backend services, API Gateway, Docker configuration, logging/observability, file service, and deployment configuration in the root Go repository.

## Summary

The project has grown into a fairly complete social-network microservice system: gateway, auth, user graph, posts, chat/calls, file storage, admin tooling, search, story, observability dashboards, and deployment scripts. The main risks are currently around secrets handling, token leakage, production compose drift, and trust boundaries between the gateway and downstream services.

Priority order:

1. Stop leaking JWTs through logs and URLs.
2. Make production fail fast when required secrets are missing.
3. Fix production compose so services can actually reach their dependencies.
4. Add authorization checks to file delete operations.
5. Tighten query-token usage and internal service exposure assumptions.

## Findings

### Critical: JWT access token is logged by auth gRPC validation

Location: `auth-service/grpc/server.go:26`

`ValidateToken` logs the raw token:

```go
logger.WithContext(ctx).Field("token", req.Token).Info("Validating token")
```

Every authenticated gateway request calls this method, so service logs can contain valid bearer tokens. Anyone with log access can impersonate users or admins until those tokens expire.

Recommended fix:

- Remove the token field entirely.
- If correlation is needed, log a short SHA-256 fingerprint such as the first 8-12 hex chars.
- Do not log refresh tokens, access tokens, OTP secrets, or OAuth codes.

### Critical: OAuth callback redirects access token in URL query

Location: `auth-service/handler/oauth_handler.go:26`

The Google callback redirects to:

```text
<frontend>/register?token=<accessToken>&userId=<id>&userName=<name>
```

Access tokens in URLs can leak through browser history, reverse proxy logs, analytics, crash reports, screenshots, and `Referer` headers.

Recommended fix:

- Prefer `HttpOnly`, `Secure`, `SameSite=Lax/Strict` cookies for access/session state.
- Or issue a short-lived one-time code and let the frontend exchange it for tokens via POST.
- Avoid putting bearer tokens in query strings.

### Critical: Production can run with public default JWT secrets

Location:

- `auth-service/config/config.go:65`
- `auth-service/config/config.go:66`
- `docker-compose.prod.yml:80`

The auth config falls back to hardcoded JWT secrets:

```go
JWTSecret:        getEnv("JWT_ACCESS_TOKEN_KEY", "your-access-secret-key-very-long-and-secure")
JWTRefreshSecret: getEnv("JWT_REFRESH_TOKEN_KEY", "your-refresh-secret-key-very-long-and-secure")
```

`docker-compose.prod.yml` does not set those variables. If deployed as-is, tokens may be forgeable using secrets visible in source control.

Recommended fix:

- In production mode, fail startup if `JWT_ACCESS_TOKEN_KEY` or `JWT_REFRESH_TOKEN_KEY` is missing.
- Set both secrets through deployment secrets or environment variables.
- Consider separate secret validation for dev/prod so local development stays convenient.

### High: Production auth service uses the wrong database env contract

Location:

- `docker-compose.prod.yml:81`
- `auth-service/config/config.go:61`
- `auth-service/db/db.go:17`

Prod compose sets `DB_HOST`, `DB_USER`, `DB_PASSWORD`, and `DB_NAME`, but the code only reads `POSTGRES_DSN`. Because `POSTGRES_DSN` is missing, auth-service falls back to:

```text
host=localhost user=postgres password=postgres dbname=auth_db port=5432 sslmode=disable
```

Inside the auth container, `localhost:5432` is not the postgres container, so auth-service will fail to start.

Recommended fix:

- Set `POSTGRES_DSN=host=postgres user=postgres password=... dbname=auth_db port=5432 sslmode=disable` in prod compose.
- Or update config loading to build a DSN from `DB_HOST` style variables.
- Keep dev and prod compose aligned so this does not drift again.

### High: Production gateway routes point to localhost for missing services

Location:

- `api-gateway/config/config.go:42`
- `api-gateway/config/config.go:43`
- `api-gateway/config/config.go:44`
- `docker-compose.prod.yml:185`

Prod compose sets gateway addresses for auth/user/post/chat/search/story, but does not set:

- `NOTIFICATION_HTTP_ADDR`
- `FILE_HTTP_ADDR`
- `ADMIN_HTTP_ADDR`

The gateway then falls back to `http://localhost:<port>`, which points inside the gateway container, not to the target services. Prod compose also does not define `file-service`, `admin-service`, or MinIO in the same way the code expects.

Affected route families include:

- `/v1/files/*`
- `/v1/admin/*`
- `/v2/statistics/*`
- `/v1/notifications/*`

Recommended fix:

- Add missing prod services and dependencies, including MinIO where file-service is used.
- Set all gateway upstream env vars explicitly in prod compose.
- Add a startup validation step in gateway for required upstream addresses.

### High: File deletion does not check ownership or admin role

Location:

- `api-gateway/router/router.go:155`
- `file-service/handler/file_handler.go:142`
- `file-service/service/file_service.go:144`

The gateway requires authentication for delete routes, but file-service deletes by ID only. It does not verify that the current user uploaded the file, owns a resource referencing that file, or has an admin role.

Impact: any authenticated user who knows or guesses a file ID can delete another user's object.

Recommended fix:

- Pass current user ID and role into delete service methods.
- Store and read uploader metadata reliably.
- Allow delete only when uploader matches, caller is admin, or a domain service authorizes the delete.
- Add tests for deleting someone else's file.

### Medium: Token query fallback is too broad

Location:

- `api-gateway/middleware/auth_middleware.go:27`
- `api-gateway/middleware/auth_middleware.go:29`
- `api-gateway/handler/log_dashboard.html:909`
- `api-gateway/handler/containers_dashboard.html:621`
- `api-gateway/handler/containers_dashboard.html:780`

The auth middleware accepts `?token=` for any authenticated route, not only WebSocket/SSE endpoints. Dashboard streams also put token values in URLs.

Query tokens are more likely to leak via logs, browser history, monitoring, and referrer paths.

Recommended fix:

- Accept query tokens only on specific WebSocket/SSE routes that cannot send headers.
- Prefer short-lived stream tokens for observability dashboards.
- Strip token query values from logs if they must exist.

### Medium: Downstream services trust gateway-injected headers

Location examples:

- `user-service/handler/handler.go:37`
- `post-service/handler/post_handler.go:243`
- `post-service/handler/post_handler.go:250`
- `file-service/handler/file_handler.go:38`
- `admin-service/handler/ad_handler.go:142`

The downstream services trust `X-User-ID` and `X-User-Role`. This is acceptable only if those services are never reachable except through the gateway. Native development scripts start services directly on local ports, and future deployment changes could accidentally expose them.

Recommended fix:

- Bind internal service ports only to private networks.
- In production, expose only gateway/load balancer ports.
- Consider an internal gateway signature header or mTLS between gateway and services.
- At minimum, document this trust boundary and add deployment checks.

### Medium: Admin dashboards include default credentials in HTML

Location:

- `api-gateway/handler/log_dashboard.html:517`
- `api-gateway/handler/log_dashboard.html:521`
- `api-gateway/handler/containers_dashboard.html:92`
- `api-gateway/handler/containers_dashboard.html:96`

The admin dashboard login forms contain default values:

```html
value="admin@admin.com"
value="123456Aa@"
```

Even if these are dev-only credentials, shipping them in public HTML encourages credential reuse and makes brute-force targets obvious.

Recommended fix:

- Remove default values.
- Use placeholders only.
- If demo credentials are needed, keep them in local docs, not in runtime HTML.

## Verification

Command run:

```bash
go test ./...
```

Result: fails only in `chat-service/service.TestGroupChat` because local MongoDB and Neo4j are not running.

Observed failure:

```text
Failed to ping MongoDB: dial tcp 127.0.0.1:27017: connect: connection refused
Neo4j ... dial tcp 127.0.0.1:7687: connect: connection refused
```

Other packages passed, including:

- `admin-service/handler`
- `auth-service/handler`
- `chat-service/handler`
- `file-service/handler`
- `logger`
- `post-service/handler`
- `profiler`
- `user-service/handler`

## Follow-up Checklist

- [x] Remove raw token logging from auth gRPC validation.
- [x] Replace OAuth token-in-query redirect with cookie or one-time code flow.
- [x] Require JWT secrets in production.
- [x] Fix `docker-compose.prod.yml` auth database DSN.
- [x] Add missing prod services/upstream addresses for file/admin/notification.
- [x] Enforce file delete authorization.
- [x] Restrict `?token=` auth fallback to WS/SSE routes only.
- [x] Remove default admin credentials from dashboard HTML.
- [ ] Document or enforce the gateway-to-service trust boundary for `X-User-*` headers.
- [ ] Decide whether `social-network-ui/` should be a submodule or regular source folder.
- [ ] Run full integration tests with `make infra-up` or a compose stack.
