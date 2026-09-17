import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'

// 说明：
// - 端口固定 5180：本机 5173 已被其他进程占用（实测）。
// - /api、/icons、/wallpapers 代理到 Go 后端；开发期不要 rewrite 前缀，
//   后端路由本身就注册在 /api/... 之下。
// - 生产构建直接产出到 ../internal/web/dist，由 Go 的 go:embed 打包进单二进制。
export default defineConfig({
  plugins: [svelte(), tailwindcss()],
  resolve: {
    alias: {
      $lib: fileURLToPath(new URL('./src/lib', import.meta.url)),
    },
  },
  server: {
    port: 5180,
    strictPort: true,
    proxy: {
      '/api': { target: 'http://127.0.0.1:8080', changeOrigin: true },
      '/icons': { target: 'http://127.0.0.1:8080', changeOrigin: true },
      '/wallpapers': { target: 'http://127.0.0.1:8080', changeOrigin: true },
    },
  },
  build: {
    outDir: '../internal/web/dist',
    emptyOutDir: true,
    target: 'es2022',
    sourcemap: false,
  },
})
