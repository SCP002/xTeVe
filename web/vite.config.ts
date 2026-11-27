import path from 'node:path'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig } from 'vite'

// The xTeVe the dev server forwards to, e.g. XTEVE=http://127.0.0.1:34400 npm run dev
const xteve = process.env.XTEVE ?? 'http://127.0.0.1:34400'

// https://vite.dev/config/
export default defineConfig({
  // xTeVe serves the built web UI at /ui/
  base: '/ui/',
  plugins: [tailwindcss(), svelte()],
  // dist/ keeps placeholder.html, which xTeVe shows when the web UI was not built
  build: { outDir: 'dist/app', emptyOutDir: true },
  resolve: {
    alias: { $lib: path.resolve('./src/lib') },
  },
  server: {
    host: true,
    proxy: {
      '/ui/api': xteve,
      '/data': { target: xteve, ws: true },
      '/images': xteve,
      '/data_images': xteve,
      '/download': xteve,
      '/m3u': xteve,
      '/xmltv': xteve,
    },
  },
})
