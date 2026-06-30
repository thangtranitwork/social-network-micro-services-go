# Ý tưởng Phát triển Tính năng Mới cho Social Network (Go Microservices)

Sau khi hoàn thành tối ưu hóa hệ thống, dưới đây là các hướng phát triển tính năng mới nhằm tăng tính tương tác, bảo mật và trải nghiệm người dùng cho hệ thống.

---

## 1. Global Search Service (Dịch vụ Tìm kiếm Toàn cầu)
- **Công nghệ**: Elasticsearch hoặc Meilisearch.
- **Mô tả**: Hiện tại việc tìm kiếm post hoặc user có thể đang dựa vào query trực tiếp trên SQL/Neo4j, điều này không hiệu quả khi dữ liệu lớn và không hỗ trợ tìm kiếm mờ (fuzzy search).
- **Cơ chế**:
    - Tạo một microservice mới `search-service`.
    - Lắng nghe Kafka events (`PostCreated`, `UserUpdated`, `CommentCreated`) để đồng bộ dữ liệu vào index của Elasticsearch.
    - Cung cấp API tìm kiếm siêu tốc, hỗ trợ gợi ý (auto-suggest) và tìm kiếm theo nội dung, hashtag, tên người dùng.

## 2. Story Service (Nội dung Biến mất sau 24h)
- **Công nghệ**: Go, Redis (cho metadata nhanh), MinIO (lưu media).
- **Mô tả**: Tính năng tương tự Instagram/Facebook Stories, cho phép người dùng chia sẻ ảnh/video ngắn biến mất sau 24 giờ.
- **Cơ chế**:
    - Quản lý danh sách story của bạn bè trong Redis để truy xuất nhanh.
    - Sử dụng một cron job hoặc TTL trong database để tự động ẩn/xóa các story đã hết hạn.

## 3. AI-Powered Content Moderation (Kiểm duyệt Nội dung bằng AI)
- **Công nghệ**: Mở rộng `ai-service`, tích hợp Gemini API (Multimodal).
- **Mô tả**: Tự động phát hiện và xử lý các nội dung vi phạm (toxic, spam, hình ảnh không phù hợp).
- **Cơ chế**:
    - Khi có bài viết hoặc comment mới, `ai-service` sẽ phân tích nội dung thông qua Gemini.
    - Nếu phát hiện vi phạm, hệ thống tự động gắn flag "Pending Review" hoặc ẩn nội dung và gửi thông báo cho Admin.
    - Phân tích cả hình ảnh (Sử dụng khả năng Vision của Gemini) để phát hiện ảnh nhạy cảm.

## 4. Real-time Presence & Activity Tracking (Trạng thái Hoạt động Real-time)
- **Công nghệ**: Redis, WebSocket (trong `chat-service` hoặc một `presence-service` riêng).
- **Mô tả**: Hiển thị trạng thái "Online/Offline", "Đang soạn tin nhắn..." (Typing indicator) và "Xem lần cuối".
- **Cơ chế**:
    - Khi client kết nối/ngắt kết nối WebSocket, cập nhật trạng thái vào Redis với TTL ngắn.
    - Sử dụng Pub/Sub của Redis để thông báo cho bạn bè về trạng thái thay đổi của người dùng ngay lập tức.

## 5. User Analytics & Engagement Dashboard (Thống kê Tương tác)
- **Công nghệ**: Kafka, ClickHouse (cho OLAP) hoặc đơn giản là Postgres.
- **Mô tả**: Cung cấp cho người dùng các số liệu thống kê về bài viết của họ (lượt xem, lượt tương tác theo thời gian).
- **Cơ chế**:
    - Thu thập event "PostViewed" qua Kafka.
    - Tổng hợp dữ liệu theo giờ/ngày.
    - Hiển thị biểu đồ tăng trưởng tương tác trên profile của người dùng.

## 6. Community/Group Service (Hội nhóm & Cộng đồng)
- **Mô tả**: Ngoài Chat nhóm (đã có kế hoạch), hệ thống cần các "Cộng đồng" nơi người dùng có thể đăng bài viết vào một chủ đề chung (giống Facebook Groups hoặc Subreddit).
- **Cơ chế**:
    - Quản lý quyền hạn (Admin, Moderator, Member) trong Group.
    - Feed riêng cho từng Group.

## 7. Centralized Logging & Observability Stack (Logging/Monitoring Tập trung)
- **Công nghệ**: Loki + Grafana (lightweight), hoặc ELK Stack (Elasticsearch + Logstash + Kibana).
- **Mô tả**: Hiện tại hệ thống đã có `logger` package với structured logging và `profiler` package đo latency per-route. Tuy nhiên logs vẫn đang ghi ra stdout của từng container riêng lẻ — không có cách tìm kiếm, filter hay alert tập trung.
- **Tính cần thiết**: ⭐⭐⭐⭐⭐ **Cực kỳ cần thiết** — đây là nền tảng để debug production.
- **Cơ chế**:
    - **Log Aggregation**: Dùng Promtail (agent) scrape logs từ Docker containers và đẩy vào Loki. Grafana làm UI query.
    - **Metrics**: Expose `/metrics` endpoint (Prometheus format) từ mỗi service, Prometheus scrape định kỳ.
    - **Alerting**: Cấu hình Grafana Alert khi error rate vượt ngưỡng, latency p99 > 500ms, hoặc service DOWN.
    - **Trace correlation**: Tận dụng `X-Trace-ID` đã có (từ tối ưu #5) để filter toàn bộ log của một request xuyên services.
- **Khả thi**: Cao — Loki + Grafana rất nhẹ, có thể thêm vào `docker-compose.yml` chỉ ~30 phút cấu hình.

## 8. Admin: Quản lý Nội dung & Hành động Điều phối (Content Moderation Actions)
- **Mô tả**: `admin-service` hiện chỉ có thống kê (users/posts count) và quản lý Docker containers. **Thiếu hoàn toàn** các công cụ điều phối nội dung thực tế.
- **Tính cần thiết**: ⭐⭐⭐⭐⭐ **Bắt buộc** cho một mạng xã hội production.
- **Các action cần thêm**:
    - **Xóa/Ẩn Post**: `DELETE /v1/admin/posts/:id` — Admin xóa bài vi phạm.
    - **Suspend/Ban User**: `POST /v1/admin/users/:id/suspend` với `duration` (tạm thời) hoặc permanent ban. Ghi vào Neo4j node `User {suspended: true, suspendedUntil: datetime}`.
    - **Xem Report Queue**: Danh sách các bài/comment bị user report, sắp xếp theo số lượt report.
    - **Audit Log**: Mọi hành động của Admin đều được ghi vào bảng `admin_audit_log` (adminID, action, targetID, timestamp, reason).
- **Khả thi**: Cao — chỉ cần mở rộng `admin-service` hiện tại.

## 9. User Report System (Tính năng Báo cáo Vi phạm)
- **Công nghệ**: PostgreSQL hoặc MongoDB (lưu report), Kafka (trigger moderation queue).
- **Mô tả**: Cho phép người dùng báo cáo bài viết, comment, hoặc tài khoản vi phạm.
- **Tính cần thiết**: ⭐⭐⭐⭐ **Rất cần thiết** — không có tính năng này, Admin không biết nội dung nào cần xử lý.
- **Cơ chế**:
    - `POST /v1/reports` với `{targetType, targetId, reason}`.
    - Nếu một target nhận ≥ N reports → tự động publish `ContentFlagged` event lên Kafka → `ai-service` ưu tiên review → `admin-service` đưa vào queue.
    - Anti-abuse: Mỗi user chỉ report một target 1 lần, rate-limit report API.
- **Khả thi**: Cao — độc lập với các service hiện tại.

## 10. Two-Factor Authentication (2FA)
- **Công nghệ**: TOTP (Google Authenticator, RFC 6238), `auth-service`.
- **Mô tả**: Bảo mật tài khoản bằng xác thực 2 bước ngoài mật khẩu.
- **Tính cần thiết**: ⭐⭐⭐⭐ **Rất cần thiết** cho bảo mật tài khoản người dùng.
- **Cơ chế**:
    - User kích hoạt 2FA: Hệ thống generate TOTP secret, trả về QR code để scan bằng Authenticator app.
    - Khi login: Sau khi verify email/password thành công, yêu cầu nhập thêm 6-digit OTP.
    - Lưu `totp_secret` (encrypted) trong DB của `auth-service`. Dùng thư viện `github.com/pquerna/otp`.
- **Khả thi**: Trung bình — cần thay đổi login flow trong `auth-service` và UI.

## 11. Rate Limiting & DDoS Protection (Giới hạn tốc độ tại Gateway)
- **Công nghệ**: Redis Sliding Window Counter, tại `api-gateway`.
- **Mô tả**: Hiện tại API Gateway **không có** rate limiting. Bất kỳ client nào cũng có thể gửi không giới hạn request, gây nguy cơ DDoS hoặc làm quá tải service.
- **Tính cần thiết**: ⭐⭐⭐⭐⭐ **Cực kỳ cần thiết** cho môi trường production.
- **Cơ chế**:
    - **Global Rate Limit**: 100 req/phút/IP cho các endpoint public (login, register).
    - **Per-User Rate Limit**: 300 req/phút/userID cho các endpoint authenticated.
    - **Endpoint-specific**: Stricter cho các action tốn tài nguyên (upload, search): 10 req/phút.
    - Sử dụng Redis `INCR` + `EXPIRE` (Sliding Window) để đếm. Trả về `429 Too Many Requests` với header `Retry-After`.
- **Khả thi**: Cao — implement một Gin middleware trong `api-gateway`, tận dụng Redis đã có.

## 12. Hashtag & Trending System (Thẻ Hashtag & Xu hướng)
- **Công nghệ**: Neo4j (graph), Redis Sorted Set (trending), `ai-service` (tag extraction).
- **Mô tả**: `ai-service` đã extract keywords từ posts (qua Gemini API) nhưng chưa lưu vào graph DB. Cần hoàn thiện vòng lặp để build tính năng trending.
- **Tính cần thiết**: ⭐⭐⭐ **Cần thiết** — tăng khả năng khám phá nội dung.
- **Cơ chế**:
    - `ai-service` sau khi extract tags → ghi vào Neo4j: `(Post)-[:HAS_TAG]->(Tag {name})` (hiện tại chỉ đang log, chưa ghi thật).
    - Dùng Redis Sorted Set `trending:tags` với score = số post trong 24h gần nhất. Cron job cập nhật mỗi 15 phút.
    - API `GET /v1/trending/tags` trả về top 10 hashtag đang trending.
- **Khả thi**: Cao — `ai-service` đã có sẵn infrastructure, chỉ cần thêm Neo4j write thật.

## 13. Push Notification cho Mobile (Firebase Cloud Messaging)
- **Công nghệ**: Firebase Admin SDK, `notification-service`.
- **Mô tả**: Hiện tại notification chỉ hỗ trợ WebSocket (real-time khi app đang mở). Khi user tắt app, **không có cách nào** push notification đến mobile.
- **Tính cần thiết**: ⭐⭐⭐⭐ **Rất cần thiết** nếu có mobile app.
- **Cơ chế**:
    - User login trên mobile → gửi FCM device token lên server, lưu vào Redis `fcm_token:{userID}`.
    - `notification-service`: Khi `PushNotification()` được gọi mà user không có WebSocket connection → fallback sang FCM push.
    - Hỗ trợ notification grouping (gộp nhiều like vào 1 notification) để tránh spam.
- **Khả thi**: Trung bình — cần thêm FCM SDK và endpoint đăng ký token.

## 14. Notification Preferences & Digest (Tùy chỉnh thông báo)
- **Mô tả**: Hiện tại hệ thống gửi notification cho **mọi action** mà không có filter. User không thể tắt loại thông báo nào.
- **Tính cần thiết**: ⭐⭐⭐ **Cần thiết** cho UX — không có tính năng này user sẽ bị spam notification.
- **Cơ chế**:
    - Lưu `NotificationPreferences` vào Neo4j/Redis: `{friendRequest: true, postLike: false, comment: true, ...}`.
    - `notification-service` check preferences trước khi tạo notification.
    - **Email Digest**: Cron job gửi email tóm tắt hoạt động hàng ngày/tuần cho user offline lâu ngày.
- **Khả thi**: Trung bình — cần thêm schema và check logic vào `notification-service`.

---

## 📊 Ma trận Ưu tiên

| # | Tính năng | Độ cần thiết | Khả thi | Ưu tiên |
|---|-----------|-------------|---------|---------|
| 7 | Logging & Observability Stack | ⭐⭐⭐⭐⭐ | Cao | 🟢 **Hoàn thành** |
| 11 | Rate Limiting tại Gateway | ⭐⭐⭐⭐⭐ | Cao | 🟢 **Hoàn thành** |
| 8 | Admin: Content Moderation Actions | ⭐⭐⭐⭐⭐ | Cao | 🟢 **Hoàn thành** |
| 9 | User Report System | ⭐⭐⭐⭐ | Cao | 🟠 **Q1** |
| 12 | Hashtag & Trending (hoàn thiện) | ⭐⭐⭐ | Cao | 🟠 **Q1** |
| 1 | Global Search Service | ⭐⭐⭐⭐ | Trung bình | 🟢 **Hoàn thành** |
| 3 | AI Content Moderation | ⭐⭐⭐⭐ | Trung bình | 🟡 **Q2** |
| 10 | Two-Factor Authentication | ⭐⭐⭐⭐ | Trung bình | 🟢 **Hoàn thành** |
| 13 | Mobile Push (FCM) | ⭐⭐⭐⭐ | Trung bình | 🟡 **Q2** |
| 4 | Real-time Presence | ⭐⭐⭐ | Cao | 🟡 **Q2** |
| 14 | Notification Preferences | ⭐⭐⭐ | Trung bình | 🟢 **Hoàn thành** |
| 2 | Story Service | ⭐⭐⭐ | Trung bình | 🟢 **Hoàn thành** |
| 5 | User Analytics Dashboard | ⭐⭐ | Thấp | 🟢 **Q3** |
| 6 | Community/Group Service | ⭐⭐ | Thấp | 🔵 **Q4** |

---
**Người thực hiện:** Antigravity AI
**Cập nhật lần cuối:** 2026-05-31 (Triển khai thành công Search và Story Services)

## 10. Two-Factor Authentication (2FA) - IMPLEMENTED
Implemented 2FA with TOTP (Generate, Verify, Disable). Added to User settings and Auth Login form.

## 11. Google OAuth2 - IMPLEMENTED
Implemented Google OAuth2 backend structure and frontend UI with i18n support.

## 1. Global Search Service - IMPLEMENTED
- **Trạng thái**: 🟢 Hoàn thành.
- **Chi tiết đã triển khai**:
  - Xây dựng microservice độc lập `search-service` kết nối với Neo4j để tìm kiếm mờ không phân biệt chữ hoa chữ thường.
  - Hỗ trợ tìm kiếm người dùng (theo username, givenName, familyName) và bài viết công khai (theo content).
  - Tích hợp gRPC với `user-service` để đồng bộ thông tin tác giả và tự động làm giàu (enrich) thông tin avatar thông qua `file-service`.
  - Mở route `/v1/search` trên Gateway chuyển tiếp request trực tiếp tới `search-service`.

## 2. Story Service - IMPLEMENTED
- **Trạng thái**: 🟢 Hoàn thành.
- **Chi tiết đã triển khai**:
  - Xây dựng microservice độc lập `story-service` quản lý nội dung story của người dùng và bạn bè.
  - Sử dụng cơ chế lưu trữ Neo4j cho các Node `Story` liên kết với `User` thông qua quan hệ `POSTED_STORY`, kết quả lọc tự động các story trong vòng 24 giờ.
  - Phát triển component UI `StoryFeed.jsx` dạng bong bóng trượt ngang ở đầu trang chủ, hỗ trợ:
    - Click "+" để đăng tải ảnh/video mới (tải qua `file-service` sử dụng presigned URL).
    - Xem story bằng trình xem toàn màn hình (Instagram-style) chạy tuần tự (progress segments) 5 giây/story, có thể chuyển đổi tiếp/lùi linh hoạt.
    - Xóa story trực tiếp đối với story của bản thân.
