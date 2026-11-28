import path from 'node:path'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig } from 'vite'

// The xTeVe the dev server forwards to, e.g. XTEVE=http://127.0.0.1:34400 npm run dev
const xteve = process.env.XTEVE ?? 'http://127.0.0.1:34400'

// https://vite.dev/config/
export default defineConfig({
  // xTeVe serves the built web UI at /web/
  base: '/web/',
  plugins: [tailwindcss(), svelte()],
  // dist/ keeps placeholder.html, which xTeVe shows when the web UI was not built
  build: { outDir: 'dist/app', emptyOutDir: true },
  resolve: {
    alias: { $lib: path.resolve('./src/lib') },
  },
  server: {
    host: true,
    // The Host header stays the one of the dev server, which xTeVe compares with the Origin of requests
    proxy: Object.fromEntries(
      ['/web/api', '/data', '/images', '/data_images', '/download', '/m3u', '/xmltv'].map((path) => [
        path,
        { target: xteve, changeOrigin: false, ws: path === '/data' },
      ]),
    ),
  },
})
