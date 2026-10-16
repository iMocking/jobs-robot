import { fileURLToPath, URL } from 'node:url'

import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

// Tailwind v4 通过 Vite 插件接入，不再需要 postcss.config / tailwind.config：
// 设计令牌写在 src/style.css 的 @theme 里，与组件放在一起更容易保持一致。
export default defineConfig({
  plugins: [vue(), tailwindcss()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5173,
    proxy: {
      // 本地联调：把 /api 与探针转发给 cmd/api 起的 Go 服务。
      // 端口取自 configs/config.yaml 的 server.port（8090），不是 8080。
      '/api': {
        target: 'http://127.0.0.1:8090',
        changeOrigin: true,
      },
      '/healthz': {
        target: 'http://127.0.0.1:8090',
        changeOrigin: true,
      },
      '/readyz': {
        target: 'http://127.0.0.1:8090',
        changeOrigin: true,
      },
    },
  },
})
