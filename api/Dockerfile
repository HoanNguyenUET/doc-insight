# ---- build ----
FROM golang:1.22-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---- runtime ----
FROM alpine:3.20
# ca-certificates: TLS tới Neon (sslmode=require) và API Gemini.
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app && adduser -S -G app app \
    && mkdir -p /data/uploads && chown app:app /data/uploads

COPY --from=build /out/server /usr/local/bin/server

USER app
# Chỉ để tài liệu hoá; cổng thực tế lấy từ env PORT (Render cấp động).
ENV PORT=8080
# File PDF tải lên. Trên Render đĩa là tạm thời (mất khi deploy lại) trừ khi gắn persistent disk.
ENV STORAGE_DIR=/data/uploads
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/server"]
