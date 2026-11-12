#!/usr/bin/env bash
# Bật lại toàn bộ app (db + api + web) và nạp dữ liệu mẫu.
#
#   ./scripts/restart.sh           # bật lại, GIỮ dữ liệu
#   ./scripts/restart.sh --reset   # XOÁ sạch dữ liệu rồi bật lại từ đầu
#
# Dùng khi app lỗi giữa buổi demo: dừng hết, build lại, chờ tới khi khoẻ.
set -euo pipefail
cd "$(dirname "$0")/.."

API=http://localhost:${API_PORT:-8080}
WEB=http://localhost:${WEB_PORT:-3000}
step() { printf '\n\033[1;34m▶ %s\033[0m\n' "$*"; }
ok() { printf '\033[1;32m✓ %s\033[0m\n' "$*"; }
fail() { printf '\033[1;31m✗ %s\033[0m\n' "$*"; exit 1; }

docker info >/dev/null 2>&1 || fail "Docker chưa chạy. Mở Docker Desktop rồi chạy lại."

step "Dừng app cũ"
if [ "${1:-}" = "--reset" ]; then
  docker compose down -v --remove-orphans
  ok "Đã xoá dữ liệu (DB + file PDF)"
else
  docker compose down --remove-orphans
fi

# Cổng 3000/8080 bị chiếm (vd. npm run dev còn chạy) thì báo rõ.
for port in "${WEB_PORT:-3000}" "${API_PORT:-8080}"; do
  if lsof -iTCP:"$port" -sTCP:LISTEN -n -P >/dev/null 2>&1; then
    fail "Cổng $port đang bị chiếm: $(lsof -iTCP:"$port" -sTCP:LISTEN -n -P | awk 'NR==2{print $1" (pid "$2")"}'). Tắt nó rồi chạy lại."
  fi
done

step "Build và bật db + api + web"
docker compose up -d --build

step "Chờ API khoẻ"
for i in $(seq 1 60); do
  if curl -sf "$API/healthz" >/dev/null; then ok "API sẵn sàng ($API)"; break; fi
  [ "$i" = 60 ] && { docker compose logs --tail=30 api; fail "API không lên sau 60 giây (log ở trên)"; }
  sleep 1
done
curl -sf "$WEB" >/dev/null && ok "Web sẵn sàng ($WEB)" || fail "Web không phản hồi"

step "Nạp dữ liệu mẫu"
(cd api && go run ./cmd/seed -api "$API")

step "Trạng thái"
docker compose ps --format 'table {{.Service}}\t{{.State}}\t{{.Status}}'
provider=$(docker compose exec -T api sh -c 'echo $LLM_PROVIDER')
echo
ok "Xong. Mở $WEB  (LLM_PROVIDER=$provider)"
