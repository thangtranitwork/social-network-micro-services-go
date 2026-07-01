# Review bổ sung Microservices và Utilities

Ngày: 2026-06-30

Phạm vi: các microservice chưa đi sâu ở vòng review trước, phần observability/logging/profiler, dashboard admin, service WebSocket, script vận hành và script seed dữ liệu.

## Tóm tắt

Dự án đã có nhiều mảnh vận hành hữu ích như log dashboard, profiler, Docker dashboard, service chat/notification realtime và script chạy native scale. Tuy nhiên các tiện ích này đang nằm sát biên production, nên rủi ro chính không còn là thiếu tính năng mà là **lộ dữ liệu qua observability**, **service nội bộ bị gọi trực tiếp**, và **script có tác dụng phá dữ liệu nếu chạy nhầm môi trường**.

Thứ tự ưu tiên đề xuất:

1. Redact hoặc tắt ghi request/response body mặc định trong logger.
2. Khóa WebSocket origin và bỏ fallback `userId` query cho luồng realtime nếu không có token đã xác thực.
3. Chỉ cho truy cập `/debug/profiler` qua gateway hoặc mạng nội bộ đáng tin.
4. Thêm guard bắt buộc cho script tạo dữ liệu test trước khi xóa/ghi DB.
5. Giảm tải log search/profiler để tránh DoS nội bộ khi log lớn.

## Findings

### Critical: Logger ghi nguyên request/response body vào log file

Vị trí:

- `logger/logger.go:621`
- `logger/logger.go:660`
- `logger/logger.go:663`
- `api-gateway/handler/log_handler.go:98`
- `api-gateway/handler/log_handler.go:295`

`logger.GinMiddleware` đọc toàn bộ request body, response body rồi ghi vào field `req_body` và `resp_body`. Log dashboard sau đó stream nguyên dòng log và search trả lại cả `RawLine` lẫn `Fields`.

Tác động:

- Login/register/refresh hoặc các API chứa token, mật khẩu, email, nội dung chat/comment có thể bị ghi vào file log.
- Người có quyền xem log dashboard hoặc đọc file log có thể thấy dữ liệu người dùng.
- Response body cũng có thể chứa access token hoặc thông tin profile đầy đủ.

Khuyến nghị:

- Mặc định không log body ở production.
- Nếu cần debug body, bật bằng env riêng như `LOG_HTTP_BODY=true` và chỉ dùng local/staging.
- Redact các key nhạy cảm: `password`, `token`, `accessToken`, `refreshToken`, `authorization`, `cookie`, `otp`, `secret`, `credential`.
- Giới hạn kích thước body được log, ví dụ tối đa 2-4KB.
- Log dashboard/search nên redact trước khi gửi ra UI, kể cả khi log file cũ đã chứa dữ liệu nhạy cảm.

### High: WebSocket cho phép mọi Origin ở chat, notification và admin

Vị trí:

- `chat-service/handler/chat_handler.go:49`
- `chat-service/handler/chat_handler.go:52`
- `notification-service/main.go:75`
- `notification-service/main.go:78`
- `admin-service/handler/docker.go:39`
- `admin-service/handler/docker.go:42`

Các WebSocket upgrader đang `CheckOrigin: return true`. Với chat và notification, service còn chấp nhận user từ header hoặc query. Với admin Docker dashboard, WebSocket stream stats/log container cũng dùng cùng upgrader mở origin.

Tác động:

- Nếu token bị lộ hoặc service port bị expose, site khác có thể mở kết nối WebSocket tới backend.
- Luồng notification dùng `userId` query ở `notification-service/main.go:469`, nên nếu gọi trực tiếp vào service có thể subscribe notification theo user ID mà không cần gateway xác thực.
- Admin WebSocket đọc stats/log container là bề mặt nhạy cảm, nhất là khi admin-service có quyền Docker daemon.

Khuyến nghị:

- Cấu hình allowlist origin theo env, ví dụ `ALLOWED_ORIGINS`.
- Không nhận `userId` từ query ở service nội bộ nếu không có chữ ký/gateway token kèm theo.
- Với WebSocket qua gateway, xác thực token ở gateway rồi truyền user ID bằng header nội bộ có ký hoặc mTLS.
- Admin WebSocket nên chỉ chạy sau auth admin và chỉ bind service trong private network.

### High: Profiler/debug endpoints mở trực tiếp trên từng service

Vị trí:

- `auth-service/main.go:136`
- `user-service/main.go:109`
- `notification-service/main.go:461`
- `api-gateway/router/router.go:85`
- `api-gateway/router/router.go:91`

Gateway đã bọc `/debug/profiler` bằng `authMiddleware` và `adminOnly`, nhưng từng service vẫn tự mount `/debug/profiler` và `/debug/profiler/reset` không có auth nội bộ.

Tác động:

- Nếu service port bị expose trực tiếp, người gọi có thể đọc thống kê command, pending request, memory/goroutine hoặc reset profiler.
- Dữ liệu profiler có thể làm lộ route nội bộ, service name, pattern lưu lượng và tình trạng hệ thống.

Khuyến nghị:

- Chỉ bật profiler endpoint khi `ENABLE_PROFILER_ENDPOINT=true`.
- Bind service HTTP port vào private network; public chỉ expose gateway.
- Nếu vẫn cần gọi trực tiếp, thêm middleware kiểm tra shared internal token hoặc mTLS.
- Giữ gateway aggregator là đường truy cập chính cho admin.

### High: Script tạo dữ liệu test có thao tác xóa dữ liệu nhưng không có guard môi trường

Vị trí:

- `scripts/gen_test_data.go:46`
- `scripts/gen_test_data.go:59`
- `scripts/gen_test_data.go:236`
- `scripts/gen_test_data.go:247`
- `scripts/gen_test_data.go:253`

Script dùng DSN hard-code tới Postgres/Neo4j local, sau đó xóa account theo danh sách email và `DETACH DELETE` user/post/comment trong Neo4j trước khi seed lại dữ liệu.

Tác động:

- Nếu chạy nhầm với port-forward hoặc env local trỏ vào database thật, script có thể xóa dữ liệu hàng loạt.
- Dữ liệu test dùng mật khẩu hash chung và nhiều email cố định; nếu lẫn vào staging/prod sẽ tạo tài khoản dự đoán được.

Khuyến nghị:

- Bắt buộc flag xác nhận, ví dụ `--confirm-reset-test-data`.
- In rõ target DB và yêu cầu người chạy gõ lại database name trước khi xóa.
- Chỉ cho chạy khi `APP_ENV=local` hoặc `ALLOW_DESTRUCTIVE_SEED=true`.
- Đọc DSN từ env thay vì hard-code, nhưng vẫn phải chặn môi trường không phải local.

### Medium: Log search chạy `tail` song song trên nhiều file và trả kết quả không giới hạn theo request

Vị trí:

- `api-gateway/handler/log_handler.go:265`
- `api-gateway/handler/log_handler.go:275`
- `api-gateway/handler/log_handler.go:282`
- `api-gateway/handler/log_handler.go:346`

Mỗi request search log spawn goroutine cho từng service, gọi external command `tail -n 100000`, đọc toàn bộ output vào memory rồi scan. Không có rate limit, timeout command hoặc limit số kết quả trả về.

Tác động:

- Admin dashboard hoặc token admin bị lộ có thể tạo tải I/O/CPU lớn.
- Khi log lớn hoặc nhiều request song song, gateway dễ bị nghẽn.
- Phụ thuộc binary `tail`, giảm tính portable.

Khuyến nghị:

- Dùng đọc file streaming trong Go thay vì `exec.Command("tail", ...)`.
- Thêm context timeout, limit kết quả và rate limit cho endpoint.
- Cho phép chọn service cụ thể thay vì luôn quét tất cả service.
- Trả về dữ liệu đã redact, không trả `RawLine` nguyên bản nếu không cần.

### Medium: Profiler có rủi ro data race quanh map/slice thống kê

Vị trí:

- `profiler/profiler.go:139`
- `profiler/profiler.go:144`
- `profiler/profiler.go:154`
- `profiler/profiler.go:404`
- `profiler/profiler.go:421`

`recordExecution` đọc/sửa `globalProfiler.stats` và append `LastExecutions` nhưng bản thân hàm không tự lock; hiện middleware và `TrackExecution` lock trước khi gọi. Cách này phụ thuộc caller nhớ lock đúng. Ngoài ra `GetStatsLightweight` đọc `LastExecutions` dưới lock, nhưng nếu có caller mới gọi `recordExecution` trực tiếp hoặc sai lock thì race rất dễ xuất hiện.

Tác động:

- Có thể phát sinh data race hoặc panic `concurrent map writes` khi mở rộng profiler.
- Percentile và `LastExecutionCount` có thể không nhất quán nếu lock contract bị phá.

Khuyến nghị:

- Đưa lock vào trong `recordExecution`, hoặc đổi tên thành hàm private yêu cầu lock rõ ràng như `recordExecutionLocked`.
- Thêm test chạy `go test -race ./profiler`.
- Tránh expose pointer/slice nội bộ nếu không copy dưới lock.

### Medium: Search service dùng `CONTAINS` trên text, dễ full scan khi dữ liệu tăng

Vị trí:

- `search-service/service/search_service.go:97`
- `search-service/service/search_service.go:99`
- `search-service/service/search_service.go:137`
- `search-service/service/search_service.go:145`

Search user/post dùng `toLower(... ) CONTAINS toLower($query)` trên username/name/content. Dù có `LIMIT 15`, database vẫn có thể phải scan nhiều node trước khi lấy kết quả.

Tác động:

- Khi số user/post tăng, API search có thể chậm và kéo Neo4j CPU cao.
- Query ngắn như một ký tự có thể đụng rất nhiều node.

Khuyến nghị:

- Chặn query quá ngắn, ví dụ tối thiểu 2-3 ký tự.
- Dùng Neo4j full-text index cho user/post content.
- Thêm timeout context riêng cho query search.
- Theo dõi latency search trong profiler sau khi thêm dữ liệu lớn.

### Medium: AI service gọi Gemini bằng `http.Post` không timeout

Vị trí:

- `ai-service/main.go:74`
- `ai-service/main.go:90`
- `ai-service/main.go:164`

Worker Kafka gọi Gemini bằng `http.Post` mặc định, không có client timeout/context. Nếu API chậm hoặc treo TCP, goroutine xử lý message có thể bị kẹt lâu.

Tác động:

- Consumer xử lý topic `post-events` có thể bị backlog.
- Lỗi mạng kéo dài làm worker mất khả năng xử lý các event tiếp theo.

Khuyến nghị:

- Dùng `http.Client{Timeout: ...}` hoặc `http.NewRequestWithContext`.
- Thêm retry có backoff và circuit breaker nhẹ.
- Ghi metric cho số lần fallback rule-based.

### Low: Script chạy native scale dùng `pkill -f` theo pattern rộng

Vị trí:

- `scripts/manage-native-scaled.sh:23`
- `scripts/manage-native-scaled.sh:33`
- `scripts/manage-native-scaled.sh:75`

Script stop service bằng `pkill -f "bin/<service>"`. Pattern này tiện cho local nhưng có thể kill nhầm process nếu cùng host có process khác chứa chuỗi tương tự. Script cũng chạy Nginx Docker với `--network host`.

Tác động:

- Rủi ro vận hành local/staging nếu nhiều instance hoặc nhiều repo cùng chạy.
- Khó audit chính xác process nào bị stop.

Khuyến nghị:

- Ghi PID file khi start, stop theo PID file.
- Kiểm tra process path nằm trong `$PROJECT_ROOT/bin`.
- Ghi rõ script chỉ dành cho local/dev, không dùng production.

## Checklist đề xuất

- [x] Thêm redaction và feature flag cho HTTP body logging.
- [x] Redact dữ liệu nhạy cảm trong log dashboard/search.
- [x] Cấu hình allowlist origin cho chat, notification và admin WebSocket.
- [x] Bỏ hoặc ký xác thực các query `userId` realtime khi gọi service trực tiếp.
- [x] Đóng profiler endpoint ở service nội bộ bằng env/internal auth.
- [x] Thêm guard destructive cho `scripts/gen_test_data.go`.
- [x] Thay log search bằng streaming reader có timeout, limit và rate limit.
- [x] Làm rõ lock contract trong `profiler.recordExecution`.
- [x] Chuyển search sang full-text index hoặc tối thiểu chặn query quá ngắn.
- [x] Thêm timeout cho Gemini HTTP request trong AI service.

## Ghi chú kiểm chứng

Vòng review này chỉ đọc source và ghi nhận rủi ro, chưa sửa code runtime.

Không chạy lại test sau khi tạo file này vì thay đổi chỉ là tài liệu.
