import { registerSW } from 'virtual:pwa-register'

/**
 * PWA 状态。
 *
 * 重要前提：Service Worker 只在**安全上下文**注册。
 * 本项目的默认部署是 http://<tailnet-ip>:8090，那里 navigator.serviceWorker
 * 存在但 register() 一定失败——所以这里对失败只做 console.warn，
 * **绝不抛错、绝不挡住应用**（PWA 是增强，不是依赖）。
 */
class PwaStore {
  /** 有新版本在等待激活 */
  needRefresh = $state(false)
  /** 首次安装完成，离线可用 */
  offlineReady = $state(false)
  /** 注册失败的原因（http 访问下属正常现象） */
  error = $state<string | null>(null)
  supported = $state(false)
  registered = $state(false)

  #update: ((reload?: boolean) => Promise<void>) | null = null

  init() {
    if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) return
    // 非安全上下文下不尝试注册：省掉一次必然失败的请求与一条控制台报错
    if (!window.isSecureContext) {
      this.error = 'insecure_context'
      return
    }
    this.supported = true
    try {
      this.#update = registerSW({
        immediate: true,
        onNeedRefresh: () => {
          this.needRefresh = true
        },
        onOfflineReady: () => {
          this.offlineReady = true
        },
        onRegisteredSW: () => {
          this.registered = true
        },
        onRegisterError: (err: unknown) => {
          // 只记录，不影响使用
          this.error = err instanceof Error ? err.message : String(err)
          console.warn('[pwa] Service Worker 注册失败（不影响使用）:', err)
        },
      })
    } catch (err) {
      this.error = err instanceof Error ? err.message : String(err)
      console.warn('[pwa] Service Worker 初始化失败（不影响使用）:', err)
    }
  }

  applyUpdate() {
    this.needRefresh = false
    void this.#update?.(true)
  }

  dismiss() {
    this.needRefresh = false
  }
}

export const pwa = new PwaStore()
