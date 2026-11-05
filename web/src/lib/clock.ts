import { ref } from 'vue'

/** Browser time in unix seconds, ticking every 5 s for "x ago" labels. */
export const nowSec = ref(Date.now() / 1000)
setInterval(() => {
  if (!document.hidden) nowSec.value = Date.now() / 1000
}, 5000)
document.addEventListener('visibilitychange', () => {
  nowSec.value = Date.now() / 1000
})
