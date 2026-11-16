import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import { fileURLToPath, URL } from 'node:url'

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  // Máy chủ Nghiệm thu (chung với app CTV). Dev: /nt/* được chuyển tới đây, khỏi vướng CORS.
  const nt = env.NT_PROXY_TARGET || 'https://nghiemthu.vuathau.com.vn'
  return {
    plugins: [react()],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    server: {
      port: 5173,
      proxy: {
        '/nt': {
          target: nt,
          changeOrigin: true,
          secure: true,
          rewrite: (p) => p.replace(/^\/nt/, ''),
        },
        // Backend Go của repo (chưa dùng cho nghiệm thu)
        '/api': {
          target: 'http://localhost:8080',
          changeOrigin: true,
        },
        '/ws': {
          target: 'ws://localhost:8080',
          ws: true,
          changeOrigin: true,
        },
      },
    },
  }
})
