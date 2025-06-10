FROM golang:1.23-alpine AS builder

WORKDIR /app

# Копируем go mod файлы
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходный код
COPY . .

# Собираем приложение
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main server.go

# Финальный stage
FROM alpine:latest

# Устанавливаем ca-certificates для HTTPS
RUN apk --no-cache add ca-certificates openssl

WORKDIR /root/

# Копируем бинарник из builder stage
COPY --from=builder /app/main .

# Копируем статические файлы
COPY --from=builder /app/public ./public
COPY --from=builder /app/assets ./assets
COPY --from=builder /app/pfdSender ./pfdSender
COPY --from=builder /app/*.html ./
COPY --from=builder /app/*.css ./

# Создаем SSL сертификаты если их нет
RUN if [ ! -f cert.pem ] || [ ! -f key.pem ]; then \
    openssl req -x509 -newkey rsa:2048 -keyout key.pem -out cert.pem \
    -days 365 -nodes -subj "/CN=*" \
    -addext "subjectAltName=IP:127.0.0.1,DNS:localhost"; \
    fi

# Открываем порт для HTTPS
EXPOSE 8443

CMD ["./main"] 