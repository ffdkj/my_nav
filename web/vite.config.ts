import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'
import { VitePWA } from 'vite-plugin-pwa'

// 说明：
// - 端口固定 5180：本机 5173 已被其他进程占用（实测）。
// - /api、/icons、/wallpapers 代理到 Go 后端；开发期不要 rewrite 前缀，
//   后端路由本身就注册在 /api/... 之下。
// - 生产构建直接产出到 ../internal/web/dist，由 Go 的 go:embed 打包进单二进制。
export default defineConfig({
  plugins: [
    svelte(),
    tailwindcss(),
    VitePWA({
      // 用 prompt 而不是 autoUpdate：autoUpdate 会在你正操作时把新版本推上来并刷新页面，
      // 正在拖拽的布局会丢。改成由用户点「刷新」。
      registerType: 'prompt',
      // 手动注册（见 src/lib/store/pwa.svelte.ts），这样才能把"有新版本"接到 UI 上
      injectRegister: false,
      includeAssets: ['favicon.svg', 'apple-touch-icon.png'],
      manifest: {
        name: 'my_nav',
        short_name: 'my_nav',
        description: '个人导航起始页',
        start_url: '/',
        scope: '/',
        display: 'standalone',
        background_color: '#070b14',
        theme_color: '#070b14',
        icons: [
          { src: 'pwa-192x192.png', sizes: '192x192', type: 'image/png' },
          { src: 'pwa-512x512.png', sizes: '512x512', type: 'image/png' },
          {
            src: 'pwa-maskable-512x512.png',
            sizes: '512x512',
            type: 'image/png',
            purpose: 'maskable',
          },
        ],
      },
      workbox: {
        globPatterns: ['**/*.{js,css,html,svg,png,webmanifest}'],
        navigateFallback: 'index.html',
        // 这些路径是后端接口/媒体，不能落进 SPA 的导航回退
        navigateFallbackDenylist: [/^\/api/, /^\/icons/, /^\/wallpapers/],
        runtimeCaching: [
          {
            // 导航数据必须"在线取最新、离线才用缓存"。
            // 用 CacheFirst/StaleWhileRevalidate 会给你端上昨天的布局，看起来像后端 bug。
            urlPattern: ({ url, request }) =>
              url.pathname.startsWith('/api') && request.method === 'GET',
            handler: 'NetworkFirst',
            options: {
              cacheName: 'my-nav-api',
              // 不设这个的话，离线时要等请求超时才回落，页面像卡死
              networkTimeoutSeconds: 3,
              expiration: { maxEntries: 50, maxAgeSeconds: 86400 },
              cacheableResponse: { statuses: [0, 200] },
            },
          },
          {
            // 写请求绝不重放
            urlPattern: ({ url }) => url.pathname.startsWith('/api'),
            method: 'POST',
            handler: 'NetworkOnly',
          },
          {
            // 图标与壁纸是内容寻址的（路径变则内容必变），可以长期缓存
            urlPattern: ({ url }) =>
              url.pathname.startsWith('/icons/') || url.pathname.startsWith('/wallpapers/'),
            handler: 'CacheFirst',
            options: {
              cacheName: 'my-nav-media',
              expiration: { maxEntries: 300, maxAgeSeconds: 2592000 },
              cacheableResponse: { statuses: [0, 200] },
            },
          },
        ],
      },
      // dev 下 Service Worker 会拦截请求、破坏 HMR，默认关掉
      devOptions: { enabled: false },
    }),
  ],
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
