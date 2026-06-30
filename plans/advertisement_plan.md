# Kế hoạch triển khai tính năng Quảng cáo (Advertisement System)

Bản kế hoạch này mô tả chi tiết giải pháp xây dựng và tích hợp hệ thống quản lý, phân phối và phân tích quảng cáo trong mạng xã hội PocPoc.

---

## 1. Phân tích Yêu cầu & Kiến trúc

Hệ thống quảng cáo sẽ bao gồm 3 phân hệ chính:
1.  **Advertiser Hub (Dành cho nhà quảng cáo - User):** Tạo chiến dịch, nạp ngân sách, cấu hình tệp khách hàng mục tiêu (Targeting), tải lên hình ảnh/nội dung quảng cáo và xem báo cáo hiệu quả.
2.  **Ad Delivery & Feed Integration (Dành cho người xem - End User):** Tự động phân phối quảng cáo xen kẽ bài viết trên Newsfeed (Sponsored Posts) hoặc hiển thị tại thanh bên (Sidebar Ads).
3.  **Admin Portal (Dành cho Quản trị viên):** Phê duyệt/từ chối chiến dịch quảng cáo, đặt cấu hình đơn giá CPC/CPM hệ thống và theo dõi doanh thu tổng.

### Hướng đi kiến trúc đề xuất:
Để tránh phát sinh chi phí vận hành thêm một container microservice mới, chúng ta sẽ tích hợp tính năng này trực tiếp vào các microservice hiện tại:
*   **`post-service`**: Quản lý các chiến dịch quảng cáo, lưu trữ lượt click/lượt xem, phân phối quảng cáo lồng ghép vào Feed của người dùng.
*   **`admin-service`**: Thực hiện duyệt chiến dịch và thống kê doanh thu quảng cáo cho Admin.

---

## 2. Thiết kế Cơ sở Dữ liệu (PostgreSQL)

Chúng ta sẽ sử dụng cơ sở dữ liệu quan hệ PostgreSQL (đã có sẵn kết nối GORM trong hệ thống) để quản lý chiến dịch quảng cáo và lưu nhật ký tương tác để đảm bảo tính nhất quán của giao dịch tài chính.

### 2.1 Bảng `ad_campaigns` (Quản lý chiến dịch)
Lưu trữ thông tin cấu hình, ngân sách và trạng thái của từng chiến dịch quảng cáo.

| Tên trường | Kiểu dữ liệu | Mô tả |
| :--- | :--- | :--- |
| `id` | UUID (Primary Key) | Mã định danh chiến dịch |
| `advertiser_id` | UUID | ID tài khoản nhà quảng cáo (liên kết với User) |
| `title` | VARCHAR(255) | Tiêu đề quảng cáo |
| `description` | TEXT | Nội dung mô tả ngắn |
| `media_url` | VARCHAR(512) | Link hình ảnh hoặc video quảng cáo |
| `target_url` | VARCHAR(512) | Link đích khi người dùng click vào quảng cáo |
| `ad_type` | VARCHAR(50) | Loại hiển thị: `FEED_POST` (bài đăng tài trợ), `SIDEBAR` (thanh bên) |
| `target_gender` | VARCHAR(10) | Giới tính mục tiêu: `ALL`, `MALE`, `FEMALE` |
| `target_min_age`| INT | Độ tuổi tối thiểu |
| `target_max_age`| INT | Độ tuổi tối đa |
| `budget_total` | DECIMAL(15,2)| Tổng ngân sách chiến dịch |
| `budget_spent` | DECIMAL(15,2)| Ngân sách đã tiêu thụ |
| `bid_type` | VARCHAR(10) | Phương thức tính phí: `CPC` (Click), `CPM` (1000 lượt xem) |
| `bid_amount` | DECIMAL(10,2)| Giá thầu cho mỗi Click hoặc 1000 View |
| `start_date` | TIMESTAMP | Ngày bắt đầu chạy quảng cáo |
| `end_date` | TIMESTAMP | Ngày kết thúc |
| `status` | VARCHAR(20) | Trạng thái: `PENDING` (chờ duyệt), `ACTIVE` (đang chạy), `PAUSED` (tạm dừng), `REJECTED` (từ chối), `COMPLETED` (hoàn thành/hết ngân sách) |
| `created_at` | TIMESTAMP | Thời gian tạo |
| `updated_at` | TIMESTAMP | Thời gian cập nhật |

### 2.2 Bảng `ad_interactions` (Log lượt xem/click)
Bảng này lưu log tương tác để trừ tiền chiến dịch và phục vụ thống kê báo cáo.

| Tên trường | Kiểu dữ liệu | Mô tả |
| :--- | :--- | :--- |
| `id` | BIGSERIAL (PK) | ID tự tăng |
| `campaign_id` | UUID | ID chiến dịch |
| `viewer_id` | UUID | ID người dùng tương tác (có thể null nếu khách) |
| `interaction_type` | VARCHAR(10) | Loại tương tác: `VIEW`, `CLICK` |
| `cost` | DECIMAL(10,2)| Số tiền bị trừ từ chiến dịch cho tương tác này |
| `ip_address` | VARCHAR(45) | IP của người dùng (ngăn chặn click tặc/fraud) |
| `user_agent` | VARCHAR(512) | Thiết bị truy cập |
| `created_at` | TIMESTAMP | Thời gian tương tác |

---

## 3. Thiết kế REST API

### 3.1 APIs dành cho Nhà quảng cáo (Advertiser Hub - Tích hợp trong `post-service`)
*   `POST /v1/ads/campaigns`: Tạo chiến dịch mới (Mặc định status = `PENDING`).
*   `GET /v1/ads/campaigns`: Lấy danh sách chiến dịch của nhà quảng cáo hiện tại.
*   `GET /v1/ads/campaigns/:id`: Xem chi tiết và phân tích (Số view, click, CTR, ngân sách còn lại).
*   `PUT /v1/ads/campaigns/:id/status`: Thay đổi trạng thái (`ACTIVE` <=> `PAUSED`).
*   `POST /v1/ads/interactions`: Log lượt tương tác (nhận payload `{campaign_id, type: "VIEW"|"CLICK"}`). Backend sẽ kiểm tra IP & UserAgent để phòng chống click ảo trước khi trừ ngân sách.

### 3.2 APIs dành cho Admin (Tích hợp trong `admin-service`)
*   `GET /v1/admin/ads/pending`: Lấy danh sách chiến dịch đang chờ phê duyệt.
*   `POST /v1/admin/ads/:id/approve`: Phê duyệt chiến dịch (Chuyển sang `ACTIVE`).
*   `POST /v1/admin/ads/:id/reject`: Từ chối chiến dịch kèm lý do.
*   `GET /v2/statistics/ads`: Thống kê tổng doanh thu quảng cáo toàn hệ thống.

---

## 4. Thuật toán Phân phối & Hiển thị Quảng cáo

### 4.1 Thuật toán lồng ghép vào Feed (`post-service`)
Khi người dùng yêu cầu Newsfeed (`GET /v1/posts/newsfeed`), Backend sẽ:
1.  Lấy danh sách các bài đăng thông thường của bạn bè/nhóm.
2.  Truy vấn danh sách quảng cáo đang hoạt động (`ACTIVE`) phù hợp với tệp Target của người dùng hiện tại (Giới tính, Độ tuổi).
    *   *Query gợi ý:* `SELECT * FROM ad_campaigns WHERE status = 'ACTIVE' AND (target_gender = 'ALL' OR target_gender = $userGender) AND $userAge BETWEEN target_min_age AND target_max_age AND budget_spent < budget_total AND NOW() BETWEEN start_date AND end_date`
3.  **Lựa chọn Quảng cáo:** Sắp xếp theo mức độ ưu tiên hoặc chọn ngẫu nhiên có trọng số dựa trên giá thầu (Bid Amount) để tối ưu doanh thu cho mạng xã hội.
4.  **Lồng ghép (Interleaving):** Chèn 1 bài quảng cáo vào mỗi khoảng cách nhất định (ví dụ: cứ mỗi 5 bài viết thông thường sẽ xen kẽ 1 bài quảng cáo có gắn nhãn **"Sponsored"** hoặc **"Được tài trợ"**). Chỗ này nên được cài đặt trong trang quản lý luôn
5.  Frontend nhận diện thuộc tính `isAd: true` trong danh sách bài đăng để hiển thị giao diện bài viết kèm liên kết đích (`target_url`).

### 4.2 Xử lý thanh toán quảng cáo (Billing Logic)
*   **Với CPM (Cost Per Mille):** Mỗi lần API Feed trả về quảng cáo cho người dùng, Backend sẽ ghi nhận 1 lượt `VIEW`. Khi số lượt view đạt 1000, trừ số tiền tương đương giá thầu `bid_amount` vào ngân sách chiến dịch (`budget_spent`).
*   **Với CPC (Cost Per Click):** Khi người dùng click vào link quảng cáo, Frontend gọi API `POST /v1/ads/interactions` với loại `CLICK`. Backend lập tức kiểm tra trùng lặp trong Redis (ví dụ: mỗi user/IP chỉ được tính click hợp lệ 1 lần mỗi 5 phút) và thực hiện trừ tiền trực tiếp vào ngân sách:
    *   `budget_spent = budget_spent + bid_amount`
    *   Nếu `budget_spent >= budget_total`, chuyển trạng thái quảng cáo sang `COMPLETED` để ngừng phân phối.

---

## 5. Kế hoạch triển khai (Roadmap)

### Phase 1: Database Setup & Admin Approval (Backend)
1.  Viết migration tạo bảng `ad_campaigns` và `ad_interactions`.
2.  Xây dựng APIs tạo chiến dịch cho Advertiser và duyệt chiến dịch cho Admin.

### Phase 2: Phân phối & Đấu thầu (Core Delivery)
1.  Tích hợp logic query quảng cáo vào API Newsfeed trong `post-service`.
2.  Thực hiện xử lý logic ghi log tương tác và cơ chế trừ tiền chiến dịch tự động.
3.  Áp dụng Redis để lưu cache các chiến dịch đang chạy nhằm giảm tải cho PostgreSQL.

### Phase 3: Giao diện (Frontend UI)
1.  Xây dựng giao diện tạo chiến dịch quảng cáo cho người dùng chuyên nghiệp (có form nhập ngân sách, target demographic và upload hình ảnh).
2.  Cập nhật UI Newsfeed để kết xuất các thẻ bài đăng được tài trợ nổi bật.
3.  Xây dựng trang Dashboard quản lý của Admin để phê duyệt chiến dịch dễ dàng.
