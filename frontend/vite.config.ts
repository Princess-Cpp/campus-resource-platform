import path from "node:path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

// 开发时 Vite 独立运行并把 /api 与 /health 代理到 Go 应用；生产由 Go 提供 dist 产物。
const backend = process.env.VITE_BACKEND_URL ?? "http://localhost:8080"

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { "@": path.resolve(import.meta.dirname, "src") },
  },
  server: {
    proxy: {
      "/api": backend,
      "/health": backend,
    },
  },
})
