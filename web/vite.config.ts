import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";

import pkg from "./package.json" with { type: "json" };

export default defineConfig({
  plugins: [react(), tailwindcss()],
  // Nhãn phiên bản ở thanh trái lấy từ package.json.
  define: { __APP_VERSION__: JSON.stringify(pkg.version.split(".").slice(0, 2).join(".")) },
  resolve: {
    alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) },
  },
  // Giữ cổng 3000: ALLOWED_ORIGINS mặc định của api là http://localhost:3000.
  server: { port: 3000, strictPort: true },
  preview: { port: 3000, strictPort: true },
});
