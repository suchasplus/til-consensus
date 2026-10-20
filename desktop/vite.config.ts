import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { resolve } from 'node:path'

export default defineConfig({
  root: resolve(import.meta.dirname, 'apps/desktop/renderer'),
  base: './',
  plugins: [
    react(),
    {
      name: 'tcview-development-csp',
      transformIndexHtml: {
        order: 'pre',
        handler(html, context) {
          return context.server ? html.replace("script-src 'self'", "script-src 'self' 'unsafe-inline'") : html
        },
      },
    },
  ],
  server: { host: '127.0.0.1', port: 5173, strictPort: true },
  build: { outDir: resolve(import.meta.dirname, 'dist/renderer'), emptyOutDir: true },
})

