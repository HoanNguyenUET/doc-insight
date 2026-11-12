#!/usr/bin/env bash
# Chạy toàn bộ test backend (kèm test tích hợp với Postgres thật, phát hiện
# data race) và in bảng độ phủ code để demo. Thêm --web để chạy cả test frontend.
#
#   ./scripts/test-backend.sh
#   ./scripts/test-backend.sh --web
set -euo pipefail
cd "$(dirname "$0")/.."

step() { printf '\n\033[1;34m▶ %s\033[0m\n' "$*"; }
ok() { printf '\033[1;32m✓ %s\033[0m\n' "$*"; }
fail() { printf '\033[1;31m✗ %s\033[0m\n' "$*"; exit 1; }

export TEST_DATABASE_URL="postgres://docinsight:docinsight@localhost:${DB_PORT:-5432}/docinsight?sslmode=disable"
if ! docker compose exec -T db pg_isready -U docinsight >/dev/null 2>&1; then
  step "DB chưa chạy: bật db"
  docker compose up -d db
  for i in $(seq 1 30); do docker compose exec -T db pg_isready -U docinsight >/dev/null 2>&1 && break; sleep 1; done
fi

cd api
step "go vet + gofmt"
go vet ./...
[ -z "$(gofmt -l .)" ] || fail "File chưa gofmt: $(gofmt -l .)"
ok "Không có cảnh báo"

step "go test -race -cover (kèm test tích hợp với Postgres)"
out=$(mktemp)
start=$(date +%s)
if ! go test -race -cover -count=1 ./... 2>&1 | tee "$out" | grep -E "^(FAIL|---)" ; then :; fi
grep -q "^FAIL" "$out" && fail "Có test trượt (xem ở trên)"
secs=$(( $(date +%s) - start ))

step "Độ phủ code theo package"
printf '%-22s %s\n' "PACKAGE" "COVERAGE"
grep -E "^ok" "$out" | sed -E 's#.*/internal/##' | awk '{printf "%-22s %s\n", $1, $(NF-2)}'
tests=$(grep -rhE "^func Test" --include='*_test.go' . | wc -l | tr -d ' ')
pkgs=$(grep -cE "^ok" "$out")
echo
ok "$pkgs package qua · $tests hàm test · chạy với -race · ${secs}s"
rm -f "$out"

if [ "${1:-}" = "--web" ]; then
  cd ../web
  step "Frontend: Vitest + lint + typecheck"
  npm test --silent 2>&1 | grep -E "Test Files|Tests"
  npm run lint --silent && npx tsc --noEmit && ok "Lint và typecheck sạch"
fi
