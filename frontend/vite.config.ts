import { writeFileSync } from 'node:fs'
import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { fileURLToPath, URL } from 'node:url'
// emptyOutDir wipes dist/.gitkeep, which go:embed needs in a fresh clone.
const keepDist: Plugin = { name: 'keep-dist', apply: 'build', closeBundle() { writeFileSync(fileURLToPath(new URL('./dist/.gitkeep', import.meta.url)), '') } }
export default defineConfig({
  plugins: [react(), tailwindcss(), keepDist],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  server: { port: 5173, strictPort: true, proxy: { '/api': 'http://127.0.0.1:36750' } },
})
