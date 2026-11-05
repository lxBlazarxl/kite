import { reactive } from 'vue'
import { api, HttpError } from './api'
import { streamErrorAction } from './overview'
import { go } from './router'
import type { MachineInfo, SampleFrame, SessionInfo, StatusFrame } from './types'

export const store = reactive({
  machines: {} as Record<string, MachineInfo>,
  connected: false,
  /** Hub time (unix seconds) of the newest sample seen. */
  lastUpdate: 0,
  session: null as SessionInfo | null,
  /** Bumped each time the stream reopens after a drop, so views can refetch what they missed. */
  reconnects: 0,
})

const STALE_MS = 20_000 // status frames arrive every 5 s, so 20 s of silence means a dead connection

/** True when messages have been arriving but none for longer than limitMs (a half-open connection after sleep). */
export function isStale(lastMessageAt: number, nowMs: number, limitMs = STALE_MS): boolean {
  return lastMessageAt > 0 && nowMs - lastMessageAt > limitMs
}

/** Startup errors other than "login required" are worth retrying (hub restarting, proxy hiccup). */
export function shouldRetryConnect(err: unknown): boolean {
  return !(err instanceof HttpError && err.status === 401)
}

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
  let opened = false
  let lastMessageAt = 0

  const open = () => {
    es?.close()
    lastMessageAt = 0
    es = new EventSource('api/v1/stream')
    es.addEventListener('open', () => {
      store.connected = true
      lastMessageAt = Date.now()
      if (opened) store.reconnects++
      opened = true
      loadMachines().catch(() => {}) // resync anything missed while disconnected
    })
    es.addEventListener('sample', (e) => {
      lastMessageAt = Date.now()
      const f: SampleFrame = JSON.parse((e as MessageEvent).data)
      applySample(f)
      listeners.forEach((fn) => fn(f))
    })
    es.addEventListener('status', (e) => {
      lastMessageAt = Date.now()
      applyStatus(JSON.parse((e as MessageEvent).data))
    })
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
  // After sleep or a network change the socket can stay half-open without any
  // error event; reopen it when nothing has arrived for a while.
  const checkStale = () => {
    if (!stopped && isStale(lastMessageAt, Date.now())) {
      store.connected = false
      open()
    }
  }
  const watchdog = setInterval(checkStale, 15_000)
  const onVisible = () => {
    if (!document.hidden) checkStale()
  }
  document.addEventListener('visibilitychange', onVisible)

  open()
  return () => {
    stopped = true
    clearTimeout(retry)
    clearInterval(watchdog)
    document.removeEventListener('visibilitychange', onVisible)
    es?.close()
  }
}
