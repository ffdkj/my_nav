/** 轻量 UI 状态：toast 与确认框。 */

export type ToastKind = 'info' | 'success' | 'error'

export interface Toast {
  id: number
  kind: ToastKind
  message: string
}

class UiStore {
  toasts = $state<Toast[]>([])
  #seq = 0

  push(kind: ToastKind, message: string, ttlMs = 4000) {
    const id = ++this.#seq
    this.toasts = [...this.toasts, { id, kind, message }]
    if (ttlMs > 0) {
      setTimeout(() => this.dismiss(id), ttlMs)
    }
    return id
  }

  dismiss(id: number) {
    this.toasts = this.toasts.filter((t) => t.id !== id)
  }

  info = (m: string) => this.push('info', m)
  success = (m: string) => this.push('success', m)
  error = (m: string) => this.push('error', m, 7000)
}

export const ui = new UiStore()
