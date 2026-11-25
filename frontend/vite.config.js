import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { fileURLToPath, URL } from 'node:url'

// https://vite.dev/config/
export default defineConfig(() => {
  return {
    plugins: [react()],
    // .env chung của repo nằm ở thư mục gốc (cạnh backend/, frontend/); Vite chỉ đưa biến VITE_* vào web
    envDir: fileURLToPath(new URL('..', import.meta.url)),
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    server: {
      port: 5173,
      proxy: {
        // Backend Go (API nghiệm thu /api/v1, chung với app CTV)
        '/api': {
          target: 'http://localhost:8080',
          changeOrigin: true,
        },
        // Ô nền bản đồ (máy chủ lấy giùm từ OpenStreetMap)
        '/tiles': {
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
