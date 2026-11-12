#!/usr/bin/env bash
# Đo chất lượng câu trả lời AI trên bộ 12 câu hỏi chuẩn (gọi API đang chạy,
# tốn 1 lượt model mỗi câu). Lưu kết quả JSON vào scripts/out/ để mở lại khi
# demo nếu lúc đó hết quota.
#
#   ./scripts/eval.sh           # cả bộ
#   ./scripts/eval.sh pl-       # chỉ các câu có id chứa "pl-"
set -euo pipefail
cd "$(dirname "$0")/.."

API=http://localhost:${API_PORT:-8080}
curl -sf "$API/healthz" >/dev/null || { echo "API chưa chạy: chạy ./scripts/restart.sh trước"; exit 1; }

mkdir -p scripts/out
file="scripts/out/eval-$(date +%Y%m%d-%H%M%S).json"
cd api
go run ./cmd/eval -api "$API" -only "${1:-}" -out "../$file" -min 0.9
echo
echo "Kết quả chi tiết: $file"
