# Kế hoạch Triển khai: Hệ thống Ví Advertiser & Tích hợp Thanh toán PayOS

Tài liệu này kết hợp toàn bộ kế hoạch về phân quyền, kiến trúc ví (wallet) và tích hợp cổng thanh toán thật (PayOS) để phục vụ tính năng quảng cáo trong hệ thống microservices.

## 1. Phân quyền tài khoản Advertiser (Auth Service)

### 1.1 Thêm Role `ADVERTISER`
- **File:** `auth-service/model/account.go`
- **Thay đổi:** Mở rộng enum/role mặc định để hỗ trợ giá trị `ADVERTISER` (bên cạnh `USER`, `ADMIN`).
- **Guard API:** Tại `admin-service/handler/ad_handler.go` (`CreateAdCampaign`), kiểm tra header `X-User-Role` đảm bảo chỉ `ADVERTISER` hoặc `ADMIN` mới được tạo quảng cáo.

### 1.2 API Cấp quyền
- **Endpoint:** `PATCH /v1/admin/users/:id/set-advertiser` (Yêu cầu quyền ADMIN).
- **Luồng:** Gọi gRPC sang `auth-service` để update `Role` của account, đồng thời trigger tạo ví mới cho user này.

---

## 2. Hệ thống Ví Tiền (Advertiser Wallet)

### 2.1 Database Migration (PostgreSQL - admin-service)
Tạo 3 bảng mới:
1. `advertiser_wallets`: id, advertiser_id, balance, currency.
2. `wallet_transactions`: id, wallet_id, amount, type (DEPOSIT, AD_CHARGE), ref_id, balance_before, balance_after.
3. `pending_deposits`: id, order_code (PayOS), wallet_id, amount, payos_link_id, status (PENDING, PAID).

### 2.2 Luồng Trừ tiền tự động (Ad Interactions)
- Khi có lượt View/Click, hàm `LogAdInteraction` phải thực hiện trừ tiền (`DeductBalance`) **TRƯỚC KHI** ghi log.
- Nếu không đủ số dư (`INSUFFICIENT_WALLET_BALANCE`), hệ thống tự động đổi trạng thái Campaign sang `PAUSED` và báo gRPC cho `post-service` xóa quảng cáo khỏi cache.
- Sử dụng Row-level lock (`SELECT ... FOR UPDATE`) để tránh race condition khi trừ tiền liên tục.

---

## 3. Tích hợp Thanh toán PayOS (Nạp tiền thật)

PayOS được lựa chọn để thử nghiệm vì thủ tục đăng ký cá nhân nhanh chóng, hỗ trợ SDK Golang chính thức (`github.com/payOSHQ/payos-lib-golang/v2`) và có thể test trực tiếp bằng tiền thật (mệnh giá nhỏ).

### 3.1 Luồng Nạp tiền (Deposit Flow)
1. Advertiser gọi `POST /v1/ads/wallet/deposit` (kèm amount >= 10,000 VND).
2. Hệ thống tạo mã `order_code` duy nhất và lưu vào bảng `pending_deposits`.
3. Gọi API PayOS `PaymentRequests.Create()` để lấy `checkoutUrl`.
4. Trả `checkoutUrl` về Frontend để Advertiser quét QR thanh toán.

### 3.2 Luồng Webhook (PayOS Callback)
1. PayOS gọi `POST /v1/ads/wallet/payos-webhook` sau khi user thanh toán.
2. **Security:** Verify Data bằng `ChecksumKey` từ PayOS.
3. **Idempotency:** Kiểm tra mã `order_code` trong `pending_deposits` xem trạng thái còn `PENDING` không.
4. Giao dịch Database nguyên tử:
   - Cập nhật status thành `PAID`.
   - Cộng tiền vào `AdvertiserWallet`.
   - Ghi log vào `wallet_transactions` (type = `DEPOSIT`).

---

## 4. Cập nhật API Gateway Routing

Các route mới cần được thêm vào `api-gateway/router/router.go` và trỏ về `admin-service`:

- `GET /v1/ads/wallet` (Auth)
- `GET /v1/ads/wallet/transactions` (Auth)
- `POST /v1/ads/wallet/deposit` (Auth)
- `PATCH /v1/admin/users/:id/set-advertiser` (Auth, Admin-only)
- `POST /v1/ads/wallet/payos-webhook` (Public - PayOS Webhook)

## 5. Các biến môi trường cần bổ sung (.env)
```env
PAYOS_CLIENT_ID=...
PAYOS_API_KEY=...
PAYOS_CHECKSUM_KEY=...
PAYOS_RETURN_URL=...
PAYOS_CANCEL_URL=...
```
