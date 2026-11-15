import { ref } from 'vue'

// 全站下拉浮层共享状态：同一时刻只允许一个展开（互斥）。
export const activeOpenId = ref('')

let seq = 0
export function nextUid(): string {
  seq += 1
  return `ss-${seq}`
}
