import { reactive } from 'vue'
import { api } from './api'
import { streamErrorAction } from './overview'
import { go } from './router'
import type { MachineInfo, SampleFrame, SessionInfo, StatusFrame } from './types'

export const store = reactive({
  machines: {} as Record<string, MachineInfo>,
  connected: false,
  /** Hub time (unix seconds) of the newest sample seen. */
  lastUpdate: 0,
  session: null as SessionInfo | null,
})

export function applySample(f: SampleFrame) {
  const m = store.machines[f.machine]
  if (m) {
    m.latest = f.sample
    m.online = true
    m.last_seen = f.ts
    m.os = f.sample.os
    m.interval_sec = f.sample.interval_sec
  } else {
    store.machines[f.machine] = {
      name: f.machine,
      os: f.sample.os,
      online: true,
      last_seen: f.ts,
      interval_sec: f.sample.interval_sec,
      latest: f.sample,
    }
  }
  store.lastUpdate = Math.max(store.lastUpdate, f.ts)
}

export function applyStatus(s: StatusFrame) {
  for (const { name, online } of s) {
    const m = store.machines[name]
    if (m) m.online = online
  }
}

export function removeMachine(name: string) {
  delete store.machines[name]
}

export async function loadMachines() {
  const ms = await api.getMachines()
  const next: Record<string, MachineInfo> = {}
  for (const m of ms) {
    next[m.name] = m
    store.lastUpdate = Math.max(store.lastUpdate, m.last_seen)
  }
  store.machines = next
}

const listeners = new Set<(f: SampleFrame) => void>()

/** Subscribe to live samples (for the detail page). Returns an unsubscribe. */
export function onSample(fn: (f: SampleFrame) => void): () => void {
  listeners.add(fn)
  return () => listeners.delete(fn)
}

/**
 * Opens the live stream. EventSource reconnects by itself after network
 * drops; when the hub refuses the stream (expired session) it gives up, so
 * on errors we ask the hub whether we are still logged in.
 */
export function startStream(): () => void {
  let es: EventSource | null = null
  let stopped = false
  let retry: ReturnType<typeof setTimeout> | undefined

  const open = () => {
    es = new EventSource('api/v1/stream')
    es.addEventListener('open', () => {
      store.connected = true
      loadMachines().catch(() => {}) // resync anything missed while disconnected
    })
    es.addEventListener('sample', (e) => {
      const f: SampleFrame = JSON.parse((e as MessageEvent).data)
      applySample(f)
      listeners.forEach((fn) => fn(f))
    })
    es.addEventListener('status', (e) => applyStatus(JSON.parse((e as MessageEvent).data)))
    es.addEventListener('error', async () => {
      store.connected = false
      try {
        const info = await api.session()
        store.session = info
        if (streamErrorAction(info) === 'login') {
          es?.close()
          go(`#/login?next=${encodeURIComponent(window.location.hash || '#/')}`)
          return
        }
      } catch {
        // hub unreachable: fall through and retry
      }
      if (es?.readyState === EventSource.CLOSED && !stopped) {
        clearTimeout(retry)
        retry = setTimeout(open, 5000)
      }
    })
  }
  open()
  return () => {
    stopped = true
    clearTimeout(retry)
    es?.close()
  }
}
