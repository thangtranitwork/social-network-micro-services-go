# Kế hoạch Chi tiết: Thu thập GPS, IP, Device Telemetry, Người quanh đây & Cấu hình Map qua Admin Runtime Config

## 1. Mục tiêu tổng thể
1. **Thu thập thông tin GPS, IP & Device** một cách có kiểm soát (event-driven + opt-in) và **lưu trữ bền vững** qua Kafka → Cassandra để phục vụ bảo mật & phân tích hành vi/khu vực sau này.
2. **Triển khai tính năng "Người quanh đây" (People Nearby)** dùng **Redis GEO** (không cần H3) kết hợp lọc quan hệ từ **Neo4j**; người dùng **tự chọn có hiển thị mình hay không**.
3. **Xây dựng Map UI trên Web (`social-network-ui`)** *(bỏ qua Mobile ở giai đoạn này)*, hỗ trợ linh hoạt nhiều chế độ bản đồ (Google Tiles kiểu `socket-service-go`, Google Maps SDK chính thức, Carto Voyager/Dark, OpenStreetMap) và **điều khiển động qua Admin Runtime Config** (`admin-service` + `runtime-configs` UI).
4. **Cập nhật Chính sách bảo mật & Điều khoản sử dụng** (`social-network-ui/src/app/privacy/page.jsx`, `/terms`, và `settings/privacy`) phản ánh trung thực cả lưu trữ tạm thời lẫn lưu trữ phân tích dài hạn.

---

## 2. Chiến lược Thu thập Dữ liệu: Thu thập Gì & Khi Nào?

### 2.1. Thông tin IP & Thiết bị (IP & Device Telemetry)

| Nhóm dữ liệu | Chi tiết trường | Nguồn lấy |
| :--- | :--- | :--- |
| **Network / IP** | `ip_address`, `geoip_country`, `geoip_city`, `geoip_lat`, `geoip_lng` | `api-gateway` đọc `CF-Connecting-IP` / `X-Real-IP` / `X-Forwarded-For` + tra cứu offline **MaxMind GeoLite2** |
| **Device** | `device_id` (UUID sinh từ client), `platform` (`web` \| `android` \| `ios`), `os_version`, `app_version`, `device_model`, `user_agent`, `timezone`, `locale` | Client gửi qua headers (`X-Device-Id`, `X-Platform`, `X-App-Version`, `X-Device-Model`) + `User-Agent` |

**Khi nào thu thập IP & Device?**
1. **Auth Events (Bắt buộc — Lưu DB dài hạn)**:
   - Tại `auth-service` khi **Register, Login, Refresh Token, Đổi mật khẩu**.
   - Lưu vào bảng `user_device_sessions` (PostgreSQL): phục vụ bảo mật ("Thiết bị đang đăng nhập", cảnh báo đăng nhập từ IP/thiết bị lạ, thu hồi phiên, chống spam/multi-account).
2. **API Gateway Middleware (Mỗi request — Không ghi DB trực tiếp)**:
   - `api-gateway` trích xuất `ClientIP`, `DeviceID`, `Platform`, `AppVersion` và gắn vào gRPC metadata / HTTP headers nội bộ (`X-Client-IP`, `X-Geo-City`, ...) chuyển xuống các service.
   - Đẩy sự kiện bất đồng bộ qua **Kafka** (`user.telemetry.events`) với cơ chế **dedup/throttle trên Redis** (ví dụ chỉ ghi nhận 1 lần mỗi 15 phút cho cùng cặp `user_id + device_id + ip`) để tránh nghẽn DB.
3. **GeoIP Fallback**:
   - Khi user từ chối cấp quyền GPS hoặc chưa có tọa độ GPS, hệ thống dùng `geoip_city` / `(geoip_lat, geoip_lng)` làm vị trí tương đối cấp thành phố để gợi ý nội dung địa phương.

---

### 2.2. Thông tin GPS (Location Telemetry)

| Trường dữ liệu | Ý nghĩa | Lưu ý |
| :--- | :--- | :--- |
| `lat`, `lng` | Tọa độ WGS84 | Kiểm tra hợp lệ `[-90,90]`, `[-180,180]` |
| `accuracy` | Độ chính xác (mét) | Bỏ qua nếu `accuracy > 500m` cho tính năng cần chính xác |
| `source` | `gps` \| `network` \| `ip` \| `manual` | Phân biệt nguồn vị trí |
| `captured_at` | Thời điểm thiết bị đo | Chống dữ liệu cũ (stale location) |

**Khi nào thu thập GPS? (Phân 4 tầng — Tuyệt đối không chạy ngầm 24/7)**

1. **Tầng 1 — Mở app / Resume Foreground (Throttle 5–10 phút/lần)**:
   - *Điều kiện*: User đã cấp quyền `WhenInUse` và bật cá nhân hóa vị trí.
   - *Hành động*: Client gọi `getCurrentPosition` 1 lần, gửi lên `POST /v1/users/me/location` (chỉ cập nhật vào Redis `user:loc:{user_id}` TTL 24h + cột `last_city`/`last_region` nếu user cho phép).
   - *Mục đích*: Gợi ý Newsfeed & Search địa phương.
2. **Tầng 2 — Theo hành động cụ thể (On-Action)**:
   - Khi user **đăng Post / Story có Check-in** (`post-service`, `story-service`).
   - Khi user **gửi vị trí tĩnh** trong chat (`chat-service`).
   - Khi user **chủ động bấm mở màn hình "Người quanh đây"** hoặc kéo làm mới danh sách (`nearby`).
3. **Tầng 3 — Cập nhật định kỳ ngắn hạn (Khi đang mở màn hình "Người quanh đây")**:
   - Chỉ khi user đang bật chế độ "Cho phép người khác tìm thấy tôi quanh đây" (Opt-in).
   - Cập nhật khi di chuyển `> 50m` hoặc mỗi `60 giây` (khi app đang mở). Dữ liệu trên Redis tự hết hạn sau `location.nearby_ttl_seconds` (mặc định 15–30 phút).
4. **Tầng 4 — Live Location Sharing (Background/Foreground trong Chat)**:
   - *Điều kiện*: Chỉ chạy khi user chủ động bấm "Chia sẻ vị trí trực tiếp (15p / 1h / 8h)" trong `chat-service`.
   - *Tần suất*: Mỗi 5–15 giây hoặc khi di chuyển `> 20m`, bắn qua WebSocket của `chat-service` và lưu vào Redis TTL theo thời gian phiên. Tự động dừng GPS listener ngay khi hết giờ hoặc user bấm "Dừng chia sẻ".

---

## 3. Thiết kế Database & Redis

### 3.1. PostgreSQL (`auth-service` / `user-service`)
1. **Bảng `user_device_sessions`** (lưu IP & Device cho bảo mật & analytics):
   - `id` (UUID, PK), `user_id` (UUID, index), `device_id` (VARCHAR), `platform` (VARCHAR), `device_model` (VARCHAR), `os_version` (VARCHAR), `app_version` (VARCHAR), `ip_address` (INET), `geo_country` (VARCHAR), `geo_city` (VARCHAR), `last_active_at` (TIMESTAMP), `revoked_at` (TIMESTAMP, nullable), `created_at`.
2. **Bảng `user_location_settings`** (lưu trạng thái Opt-in của user):
   - `user_id` (UUID, PK)
   - `discoverable_nearby` (BOOL, default `false`) — Cho phép xuất hiện trong "Người quanh đây"
   - `location_personalization` (BOOL, default `true`) — Dùng vị trí thô để gợi ý bài viết/bạn bè
   - `last_country`, `last_city` (VARCHAR) — Chỉ lưu cấp thành phố
   - `updated_at` (TIMESTAMP)

### 3.2. Redis (Tận dụng Redis 7.2 sẵn có — Không dùng H3)
1. **Geo Index cho Người quanh đây**:
   - `GEOADD nearby:users <lng> <lat> <user_id>`
   - Kèm key heartbeat: `SET nearby:alive:<user_id> 1 EX <nearby_ttl_seconds>`
   - Kèm fuzzing tọa độ nhẹ (±50–150m tùy config `location.privacy_fuzz_meters`) trước khi lưu vào `nearby:users` nếu muốn ẩn vị trí nhà chính xác.
2. **Throttle cập nhật vị trí (Rate Limit)**:
   - `SET loc:rate:<user_id> 1 EX <gps_min_interval_seconds> NX`

### 3.3. Cassandra (Lưu trữ bền vững GPS, IP & Device để phân tích sau này)
Tận dụng **Cassandra 4.1** và **Kafka** đã có sẵn trong `docker-compose.yml` (tương tự luồng `history_consumer.go` của `post-service`):
- **Luồng ghi**: `api-gateway` / `auth-service` / `user-service` publish event vào Kafka topic `user.telemetry.events` → Consumer ghi batch vào Cassandra (không làm chậm API chính).
- **Bảng `user_telemetry_events`** (Partition key: `(user_id, year_month)`, Clustering key: `created_at DESC, event_id`):
  - `user_id` (UUID), `year_month` (TEXT, ví dụ `'2026-11'`), `created_at` (TIMESTAMP), `event_id` (UUID)
  - `event_type` (TEXT: `login` | `app_open` | `nearby_ping` | `post_checkin` | `live_location_start`)
  - `lat` (DOUBLE), `lng` (DOUBLE), `accuracy` (FLOAT), `location_source` (TEXT)
  - `ip_address` (INET), `geo_country` (TEXT), `geo_city` (TEXT)
  - `device_id` (TEXT), `platform` (TEXT), `device_model` (TEXT), `os_version` (TEXT), `app_version` (TEXT), `user_agent` (TEXT)
- **Bảng phụ theo khu vực/ngày `location_analytics_by_city_day`** (Partition key: `(geo_city, event_date)`, Clustering key: `created_at DESC`): phục vụ thống kê mật độ người dùng theo thành phố/ngày mà không cần full-scan theo user.
- **Retention**: Set `default_time_to_live` hoặc TTL theo cấu hình `telemetry.retention_days` (ví dụ `180` ngày = 15,552,000 giây) để tự động dọn dữ liệu quá cũ.

---

## 4. Cấu hình Chế độ Map & Location qua Admin Runtime Config

Tận dụng hệ thống `runtime_configs` hiện có tại [admin-service/model/runtime_config.go](file:///home/thang/coding/social-network-go/admin-service/model/runtime_config.go), [internal/runtimeconfig/provider.go](file:///home/thang/coding/social-network-go/internal/runtimeconfig/provider.go) và giao diện [runtime-configs/page.js](file:///home/thang/coding/social-network-go/social-network-ui/src/app/admin/dashboard/runtime-configs/page.js).

### 4.1. Danh sách Runtime Config Keys đề xuất

| Key | Scope | Category | Type | Default Value | Sensitive | Mô tả |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| `map.provider` | `client` | `map` | `STRING` | `carto` | `false` | Chế độ map: `google_tiles` \| `google_sdk` \| `carto` \| `osm` \| `disabled` |
| `map.style` | `client` | `map` | `STRING` | `auto` | `false` | Style map: `auto` (theo dark/light UI) \| `voyager` \| `positron` \| `dark-matter` \| `roadmap` \| `satellite` |
| `map.google_tiles_url` | `client` | `map` | `STRING` | `https://mt{s}.google.com/vt/lyrs=r&x={x}&y={y}&z={z}&hl=vi` | `false` | Template tile URL khi chọn `google_tiles` (giống `socket-service-go`) |
| `map.google_api_key` | `client` | `map` | `STRING` | `""` | `true` *(hoặc tách key riêng cho client)* | API Key dùng khi bật `google_sdk` |
| `map.default_center` | `client` | `map` | `JSON` | `{"lat":10.7769,"lng":106.7009,"zoom":13}` | `false` | Tọa độ mặc định (TP.HCM) khi chưa có GPS |
| `location.nearby_enabled` | `client` | `location` | `BOOL` | `true` | `false` | Bật/tắt toàn bộ tính năng Người quanh đây |
| `location.nearby_require_mutual_optin` | `backend` | `location` | `BOOL` | `false` | `false` | `true`: user phải bật hiển thị mình mới xem được người khác; `false`: xem tự do |
| `location.nearby_max_radius_km` | `client` | `location` | `INT` | `10` | `false` | Bán kính tìm kiếm tối đa (km) |
| `location.nearby_ttl_seconds` | `backend` | `location` | `INT` | `900` | `false` | Thời gian user tồn tại trên Redis GEO nếu không ping lại (15 phút) |
| `location.gps_min_interval_seconds` | `client` | `location` | `INT` | `30` | `false` | Khoảng thời gian tối thiểu giữa 2 lần client gửi GPS |
| `location.privacy_fuzz_meters` | `backend` | `location` | `INT` | `100` | `false` | Làm nhiễu tọa độ trước khi lưu Redis GEO để chống dò vị trí chính xác |
| `telemetry.collect_ip_device_enabled` | `backend` | `telemetry` | `BOOL` | `true` | `false` | Bật/tắt ghi nhận lịch sử IP & Device telemetry |
| `telemetry.retention_days` | `backend` | `telemetry` | `INT` | `180` | `false` | Số ngày lưu bền vững log GPS/IP/Device trên Cassandra trước khi tự hết hạn |

### 4.2. Endpoint Public cho Client đọc Map Config
Hiện tại `admin-service` chỉ có `/v1/admin/runtime-configs` (yêu cầu quyền Admin). Cần bổ sung:
- **`GET /v1/public/client-configs/map`** (qua `api-gateway` → `admin-service` hoặc đọc trực tiếp từ Redis cache được `SyncRuntimeConfigCache` đồng bộ):
  - Chỉ trả về các key có `scope = "client"` và `is_sensitive = false` (riêng `map.google_api_key` chỉ trả về khi `map.provider == "google_sdk"`).
  - Cache response trên Redis + HTTP `Cache-Control: max-age=60` để Web/Mobile cập nhật gần như tức thì khi Admin bấm **Save & Sync** trên trang Admin.

### 4.3. Cách hoạt động của Map UI ở Frontend (`social-network-ui`)
- **Web (`social-network-ui`)**: Tạo component `<DynamicMap />` dùng **MapLibre GL JS** (`maplibre-gl`):
  - Nếu `map.provider === "google_tiles"`: nạp raster source `mt0..mt3` (hệt `~/work/socket-service-go/web/share/share.js`), hỗ trợ `lyrs=r` (roadmap) hoặc `lyrs=s,h` (satellite).
  - Nếu `map.provider === "carto"`: nạp style JSON của Carto (`voyager`, `positron`, `dark-matter` tự khớp Dark Mode của Next.js).
  - Nếu `map.provider === "osm"`: nạp raster tiles của OpenStreetMap.
  - Nếu `map.provider === "google_sdk"`: render `@vis.gl/react-google-maps` với `google_api_key`.
  - Nếu `map.provider === "disabled"`: ẩn bản đồ, chỉ hiển thị danh sách dạng List (fallback an toàn).
- **Trang Người quanh đây (`social-network-ui`)**: Có thanh trạng thái **Opt-in** rõ ràng ở đầu trang/bản đồ: *"Hiển thị vị trí của tôi trong Người quanh đây"* (bật/tắt tức thì, tắt là gọi `DELETE /v1/users/me/location` xóa ngay khỏi Redis).
- **Admin UI (`social-network-ui/src/app/admin/dashboard/runtime-configs/page.js`)**:
  - Bổ sung quick-preset / dropdown cho `map.provider` và nút preview bản đồ nhỏ ngay trong trang Admin để kiểm tra trước khi Sync.

---

## 5. Cập nhật Điều khoản, Chính sách Bảo mật & Privacy Settings

1. **Cập nhật [social-network-ui/src/app/privacy/page.jsx](file:///home/thang/coding/social-network-go/social-network-ui/src/app/privacy/page.jsx) & `messages/vi.json`, `en.json`**:
   - Bổ sung **Mục Dữ liệu Vị trí (GPS), Địa chỉ IP & Thông tin Thiết bị**:
     - Giải thích rõ: Thu thập IP & Device khi đăng nhập/sử dụng để bảo mật tài khoản, chống spam và xác định khu vực thô.
     - Thu thập GPS: chỉ khi người dùng cấp quyền; phân biệt rõ:
       1. *Hiển thị công khai / Người quanh đây*: Opt-in, lưu tạm trên Redis, tự xóa sau 15–30 phút hoặc ngay khi tắt, chỉ hiển thị khoảng cách làm tròn cho người lạ.
       2. *Lưu trữ phân tích & bảo mật*: Nhật ký kỹ thuật (IP, thiết bị, vị trí tương đối/GPS khi dùng tính năng) được lưu trữ an toàn tối đa `N` ngày (theo cấu hình `telemetry.retention_days`, mặc định 180 ngày) để phân tích hệ thống, chống gian lận và cải thiện gợi ý nội dung; người dùng có quyền yêu cầu xóa dữ liệu.
2. **Tạo trang Điều khoản sử dụng (`/terms`)** và gắn link cùng `/privacy` ở màn hình Login/Register (`social-network-ui/src/app/(auth)/login/page.js`) và Footer (`about/page.jsx`).
3. **Cập nhật [settings/privacy/page.js](file:///home/thang/coding/social-network-go/social-network-ui/src/app/(home)/settings/privacy/page.js)**:
   - Thêm công tắc **"Hiển thị tôi trong Người quanh đây"** (`discoverable_nearby`) và **"Cho phép cá nhân hóa nội dung theo vị trí"** (`location_personalization`), kèm nút **"Ẩn vị trí & xóa khỏi Người quanh đây ngay"** (`DELETE /v1/users/me/location`).

---

## 6. Lộ trình Triển khai (Theo thứ tự ưu tiên)

1. **Bước 1 — Admin Runtime Config & Public Map Config API**:
   - Seed các key `map.*`, `location.*`, `telemetry.*` vào bảng `runtime_configs`.
   - Thêm endpoint `GET /v1/public/client-configs/map` đọc từ Redis cache.
2. **Bước 2 — IP, Device & GPS Telemetry Pipeline (Kafka → Cassandra + Postgres)**:
   - Thêm middleware trích xuất IP/Device ở `api-gateway` + MaxMind GeoLite2.
   - Lưu `user_device_sessions` ở `auth-service` và consumer ghi `user_telemetry_events` vào **Cassandra**.
3. **Bước 3 — Legal & Privacy Settings**:
   - Cập nhật `privacy/page.jsx`, thêm `/terms`, và thêm toggle ở `settings/privacy/page.js`.
4. **Bước 4 — Backend "Người quanh đây" (Redis GEO + Opt-in)**:
   - API `PUT /v1/users/me/location`, `DELETE /v1/users/me/location`, `PATCH /v1/users/me/location-settings`, `GET /v1/recommendations/nearby` (Redis `GEOSEARCH` + Neo4j filter `BLOCK`/`FRIEND` + làm tròn khoảng cách).
5. **Bước 5 — Map UI (`social-network-ui`)**:
   - Tích hợp `maplibre-gl` trên Next.js đọc config từ `GET /v1/public/client-configs/map` (hỗ trợ chuyển đổi `google_tiles` / `carto` / `osm` / `google_sdk` từ Admin) + trang Người quanh đây có nút bật/tắt hiển thị vị trí.
