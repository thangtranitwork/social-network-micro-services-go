 # Plan: AI Content Moderation cho Social Network

  ## Summary

  Mở rộng ai-service từ keyword extraction sang kiểm duyệt nội dung. MVP sẽ tạo moderation pipeline gồm AI review, user report,
  admin review queue và audit log. Hệ thống ưu tiên human-in-the-loop: AI chỉ tự động flag hoặc auto-hide khi confidence cao, không
  auto-ban user trong phase đầu.

  ## Key Changes

  - Thêm Kafka event moderation:
      - content.moderation.requested: {targetType, targetId, authorId, content, mediaIds, source, occurredAt}
      - content.moderation.completed: {targetType, targetId, verdict, categories, confidence, reason, occurredAt}
      - content.reported: {targetType, targetId, reporterId, reason, occurredAt}

  - ai-service xử lý moderation bằng Gemini nếu có GEMINI_KEY, fallback bằng rule-based detector cho spam/toxic keywords.
  - Thêm moderation queue trong admin-service để admin xem nội dung bị flag/report và thực hiện action.
  - Hỗ trợ violation categories: SPAM, TOXIC, HARASSMENT, SEXUAL, VIOLENCE, SCAM, HATE, SELF_HARM.
  - Policy mặc định:
      - safe: không action.
      - needs_review: đưa vào admin queue.
      - violation + confidence cao: auto-hide tạm thời và đưa vào queue.
      - Không auto-suspend/ban trong MVP.

  - Thêm audit log cho mọi AI/admin action: actor, action, target, reason, timestamp.

  ## Implementation Changes

  - post-service publish moderation request khi tạo/cập nhật post và comment.
  - ai-service thêm moderation worker song song keyword worker, chuẩn hóa timeout, retry nhẹ, tag/verdict validation và /health tại
    port 10091.

  - admin-service thêm API:
      - GET /v1/admin/moderation/queue
      - POST /v1/admin/moderation/:id/approve
      - POST /v1/admin/moderation/:id/suspend-author

  - Thêm user report API:
      - POST /v1/reports
      - Chặn duplicate report cùng reporterId + targetType + targetId.
      - Nếu report count vượt ngưỡng mặc định 3, publish moderation request với priority cao.

  - Frontend admin dashboard thêm trang moderation queue tối giản: filter status/category, xem reason, approve/hide/delete/suspend.

  ## Test Plan

  - Backend:
      - go test ./ai-service
      - go test ./admin-service/...
      - go test ./post-service/...
      - go test ./api-gateway/...

  - Integration:
      - Tạo post bình thường, xác nhận không vào queue.
      - Tạo post spam/toxic, xác nhận queue có item needs_review hoặc violation.
      - Report cùng target từ 3 user khác nhau, xác nhận moderation request được publish.
      - Admin approve/hide/delete item, xác nhận target state đổi đúng và audit log được ghi.
      - GET /monitor/health báo ai-service UP khi worker chạy.

  - Frontend:
      - cd social-network-ui && npm run build
      - Kiểm tra admin moderation page hiển thị queue và gọi action đúng.

  ## Assumptions

  - File plan sẽ lưu tại plans/ai_content_moder.md.
  - MVP tập trung post/comment; chat moderation để phase sau vì realtime/private messaging có rủi ro privacy khác.
  - Không auto-ban trong phase đầu; chỉ cho phép admin suspend sau review.
  - Nếu chưa có DB schema riêng cho queue/audit, ưu tiên dùng PostgreSQL trong admin-service vì dữ liệu moderation/audit cần query
    dạng bảng và giữ lịch sử.