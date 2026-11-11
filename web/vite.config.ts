import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) },
  },
  // Giữ cổng 3000: ALLOWED_ORIGINS mặc định của api là http://localhost:3000.
  server: { port: 3000, strictPort: true },
  preview: { port: 3000, strictPort: true },
});
