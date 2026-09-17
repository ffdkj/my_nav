/**
 * UUIDv7 生成器。
 *
 * ⚠️ 不能用 crypto.randomUUID()：它被规范限制在**安全上下文**里，
 * 而本项目按决策是 http://<tailnet-ip>:8090 访问（非安全上下文），
 * 此时 crypto.randomUUID 是 undefined。crypto.getRandomValues 在非安全上下文可用。
 *
 * 主键由客户端生成，是"整板声明式提交"能成立的前提（见 docs/spec.md §5.1）。
 */
export function uuidv7(): string {
  const bytes = new Uint8Array(16)
  crypto.getRandomValues(bytes)

  const ts = Date.now()
  bytes[0] = Math.floor(ts / 2 ** 40) & 0xff
  bytes[1] = Math.floor(ts / 2 ** 32) & 0xff
  bytes[2] = Math.floor(ts / 2 ** 24) & 0xff
  bytes[3] = Math.floor(ts / 2 ** 16) & 0xff
  bytes[4] = Math.floor(ts / 2 ** 8) & 0xff
  bytes[5] = ts & 0xff
  bytes[6] = (bytes[6] & 0x0f) | 0x70 // version 7
  bytes[8] = (bytes[8] & 0x3f) | 0x80 // variant 10

  const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, '0'))
  return [
    hex.slice(0, 4).join(''),
    hex.slice(4, 6).join(''),
    hex.slice(6, 8).join(''),
    hex.slice(8, 10).join(''),
    hex.slice(10, 16).join(''),
  ].join('-')
}
