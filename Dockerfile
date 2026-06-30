# ==========================================
# Giai đoạn 1: Biên dịch mã nguồn Go siêu tối ưu
# ==========================================
FROM golang:1.25-alpine AS builder

# Cài đặt các gói hệ thống cần thiết cho biên dịch
RUN apk add --no-cache git gcc musl-dev

WORKDIR /app

# Sao chép và tải các Go modules trước để tận dụng cache của Docker
COPY go.mod go.sum ./
RUN go mod download

# Sao chép toàn bộ mã nguồn
COPY . .

# Tham số để xác định service nào cần được build (ví dụ: auth-service, user-service)
ARG SERVICE
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o main ./${SERVICE}/main.go

# ==========================================
# Giai đoạn 2: Runtime Environment siêu nhẹ
# ==========================================
FROM alpine:3.19

# Cài đặt ca-certificates để hỗ trợ kết nối HTTPS/SSL an toàn
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Sao chép file nhị phân đã biên dịch từ Giai đoạn 1
COPY --from=builder /app/main .

# Khởi chạy ứng dụng
CMD ["./main"]
