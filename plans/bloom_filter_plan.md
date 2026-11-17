# Kế hoạch triển khai Bloom Filter cho `social-network-go`

Ngày lập: 2026-11-17. Trạng thái: Đã triển khai (Phase 1, Phase 2, Phase 3).

## 1. Mục tiêu & Hạ tầng

- **Hạ tầng Redis**: Nâng cấp image Redis từ `redis:7.2-alpine` lên `redis:8-alpine` (tích hợp sẵn module **RedisBloom**: `BF.*`, `CF.*` trong nhân Redis chính thức) tại:
  - `docker-compose.yml`
  - `docker-compose.dev.yml`
  - `docker-compose.prod.yml`
- **Thư viện dùng chung**: Tạo package `internal/bloom` (đồng bộ với `internal/events`, `internal/moderation`, `internal/runtimeconfig`) gồm:
  - Interface `Filter`: `Reserve`, `Add`, `AddBatch`, `Exists`, `ExistsBatch`, `MarkReady`, `IsReady`.
  - `RedisBloomFilter`: Bọc các lệnh RedisBloom (`BF.RESERVE`, `BF.ADD`, `BF.MADD`, `BF.EXISTS`, `BF.MEXISTS`) qua `go-redis/v9`, hỗ trợ TTL cho rotating bucket và cờ `:ready` để đảm bảo an toàn trước khi warmup.
  - `MemoryBloomFilter`: Bitset in-memory thread-safe (`[]uint64` + `sync.RWMutex` + FNV-1a double hashing) phục vụ unit test và pre-filter local (Phase 3).

---

## 2. Cơ chế an toàn (Zero-Regression & Fail-Open)

1. **Cờ `ready` (`<key>:ready`)**:
   - Bloom Filter chỉ được phép **short-circuit** (trả `NotFound` / bỏ qua query DB) khi `<key>:ready == "1"` (đã warmup xong dữ liệu từ DB).
   - Nếu chưa warmup hoặc Redis gặp lỗi/timeout → **fail-open** (cho phép query xuống DB như bình thường, đồng thời log warning).
2. **Xử lý False Positive**:
   - Khi Bloom Filter trả về `true` (có thể tồn tại), luồng xử lý luôn đi tiếp xuống cache/DB để kiểm tra chính xác → **không bao giờ sai dữ liệu**.

---

## 3. Lộ trình triển khai theo Phase

### Phase 1: Chống Cache Penetration (Existence Check) — Ưu tiên cao
1. **`auth-service` (`bf:accounts:email`)**:
   - `AuthService.Register` (`auth-service/service/register.go`): Kiểm tra `bf:accounts:email` trước khi gọi `AccountRepo.FindByEmail`. Khi tạo account thành công → `Add(email)`.
2. **`user-service` (`bf:users:id`, `bf:users:username`)**:
   - `UserService.GetUserProfile` (`user-service/service/profile.go`): Phân biệt UUID (`bf:users:id`) và username (`bf:users:username`); nếu filter đã ready và báo chắc chắn không tồn tại → trả `ErrUserNotFound` ngay, tránh query PostgreSQL/Neo4j.
   - `EnsureProfile` / `UpdateUsername`: Tự động `Add` ID và username mới vào filter.
3. **`post-service` (`bf:posts:id`)**:
   - `PostService.GetPost` (`post-service/service/post.go`): Kiểm tra `bf:posts:id` trước `Repo.GetPost`.
   - `CreatePost` / `SharePost`: Tự động `Add(postID)` khi tạo bài thành công.
4. **Warmup dữ liệu ban đầu**:
   - Tạo script `scripts/warmup-bloom/main.go` (hoặc hàm warmup theo batch) đọc dữ liệu hiện có từ PostgreSQL/Neo4j (`accounts.email`, `users.id`, `users.username`, `posts.id`) nạp vào RedisBloom và bật cờ `:ready`.

### Phase 2: Lọc bài viết đã xem trên Newsfeed & Recommendation — Ưu tiên cao
- Dùng **Rotating (Time-Bucketed) Redis Bloom Filter** theo ngày/tuần cho từng user: `bf:seen:{userID}:{YYYY-MM-DD}` với TTL 7–14 ngày.
- Khi user xem bài (`view_event` / `history_consumer`), gọi `BF.ADD` vào bucket ngày hiện tại.
- Khi `post-service` / `recommendation-service` chấm điểm newsfeed, gọi `BF.MEXISTS` cho danh sách ứng viên qua 2–3 bucket gần nhất (1 round-trip/bucket) để hạ điểm hoặc loại bài đã xem mà không phình Neo4j `VIEWED` hay tốn RAM lưu `SET` từng user.

### Phase 3: Pre-filter Token Blacklist tại `auth-service` / `api-gateway`
- Dùng `MemoryBloomFilter` (in-memory) lưu danh sách token bị revoke trong chu kỳ TTL của access token.
- Nếu token không nằm trong `MemoryBloomFilter` → bỏ qua lệnh `EXISTS auth:blacklist:{token}` trên Redis.
- Đồng bộ multi-instance qua Redis Pub/Sub khi có `Logout` / `RevokeToken`.

---

## 4. Kiểm thử & Nghiệm thu
- Unit test cho `internal/bloom`: kiểm tra tỷ lệ *false positive* thực tế so với `errorRate` cấu hình, concurrency safety, fail-open khi Redis nil/lỗi, và cơ chế `:ready`.
- Unit test cho `auth-service`, `user-service`, `post-service` với mock `bloom.Filter` xác minh:
  - Khi filter `ready=true` & `Exists=false` → DB không bị gọi.
  - Khi filter `ready=false` hoặc lỗi → vẫn fallback DB bình thường.
  - Khi tạo record mới → `Add` được gọi.
