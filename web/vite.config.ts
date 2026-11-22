import { readFileSync } from 'node:fs'
import path from 'node:path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

const apiTarget = process.env.PMAIL_API_TARGET || 'http://localhost:8080'
const mockEnabled = process.env.VITE_MOCK === '1'

export default defineConfig({
  base: '/',
  define: {
    'import.meta.env.VITE_MOCK': JSON.stringify(mockEnabled ? '1' : '0'),
  },
  plugins: [
    react(),
    tailwindcss(),
    {
      name: 'pmail-public-assets',
      apply: 'build',
      generateBundle() {
        const assets = mockEnabled ? ['favicon.svg', 'mockServiceWorker.js'] : ['favicon.svg']
        for (const fileName of assets) {
          this.emitFile({
            type: 'asset',
            fileName,
            source: readFileSync(path.resolve(__dirname, 'public', fileName)),
          })
        }
      },
    },
  ],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    host: '0.0.0.0',
    port: 5173,
    proxy: {
      '/api': {
        target: apiTarget,
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    // Public assets are emitted explicitly above. A normal build must not
    // publish an unused service worker from the development public directory.
    copyPublicDir: false,
    assetsDir: 'assets',
    sourcemap: false,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return undefined
          if (id.includes('@tiptap') || id.includes('prosemirror')) {
            return 'vendor-editor'
          }
          if (
            id.includes('/react/') ||
            id.includes('/react-dom/') ||
            id.includes('/scheduler/') ||
            id.includes('/react-router')
          ) {
            return 'vendor-react'
          }
          if (id.includes('/@tanstack/')) {
            return 'vendor-tanstack'
          }
          return undefined
        },
      },
    },
  },
})
