import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('/react') || id.includes('/react-dom')) return 'vendor'
          if (id.includes('/recharts')) return 'charts'
          if (id.includes('/@tanstack') || id.includes('/zustand')) return 'query'
        },
      },
    },
  },
  server: {
    proxy: {
      '/webhooks': 'http://localhost:8080',
      '/events': 'http://localhost:8080',
      '/deliveries': 'http://localhost:8080',
      '/stream': 'http://localhost:8080',
      '/health': 'http://localhost:8080',
      '/config': 'http://localhost:8080',
    },
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: './src/test/setup.ts',
  },
})
