import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  // Preserve the browser Host header on the WebSocket upgrade. The backend's
  // same-origin check then accepts Vite's local proxy just like production.
  server: { proxy: { '/api': 'http://localhost:8080', '/ws': { target: 'ws://localhost:8080', ws: true } } },
})
