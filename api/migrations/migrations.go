// Package migrations nhúng các file SQL vào binary để server tự chạy
// migration khi khởi động (Render + Neon không cần bước migrate riêng).
// Tên file theo định dạng golang-migrate: NNNNNN_name.{up,down}.sql.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
