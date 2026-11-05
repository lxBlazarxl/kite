import { go } from './router'
import type { MachineInfo, Resolution, SeriesResponse, SessionInfo } from './types'

const MAX_KEYS = 64 // hub limit per series request

export class HttpError extends Error {
  constructor(public status: number, message: string) {
    super(message)
  }
}

async function req(path: string, init: RequestInit = {}, redirectOn401 = true): Promise<Response> {
  const r = await fetch(path, { credentials: 'same-origin', ...init })
  if (r.status === 401 && redirectOn401) {
    const here = window.location.hash || '#/'
    if (!here.startsWith('#/login')) go(`#/login?next=${encodeURIComponent(here)}`)
  }
  if (!r.ok) throw new HttpError(r.status, (await r.text()).trim() || r.statusText)
  return r
}

const enc = encodeURIComponent

export const api = {
  async session(): Promise<SessionInfo> {
    return (await req('api/v1/session', {}, false)).json()
  },
  async getMachines(): Promise<MachineInfo[]> {
    return (await req('api/v1/machines')).json()
  },
  async getKeys(name: string): Promise<string[]> {
    return (await req(`api/v1/machines/${enc(name)}/keys`)).json()
  },
  /** Fetches in batches of 64 keys (the hub's limit) and merges the results. */
  async getSeries(name: string, keys: string[], from: number, to: number, res?: Resolution): Promise<SeriesResponse> {
    const out: SeriesResponse = { resolution: res ?? 'raw', step: 0, series: {} }
    for (let i = 0; i < keys.length; i += MAX_KEYS) {
      const q = new URLSearchParams({ from: String(Math.floor(from)), to: String(Math.ceil(to)) })
      if (res) q.set('res', res)
      for (const k of keys.slice(i, i + MAX_KEYS)) q.append('key', k)
      const part: SeriesResponse = await (await req(`api/v1/machines/${enc(name)}/series?${q}`)).json()
      out.resolution = part.resolution
      out.step = part.step
      Object.assign(out.series, part.series)
    }
    return out
  },
  async deleteMachine(name: string): Promise<void> {
    await req(`api/v1/machines/${enc(name)}`, { method: 'DELETE' })
  },
  async login(password: string): Promise<void> {
    await req('api/v1/login', { method: 'POST', body: JSON.stringify({ password }), headers: { 'Content-Type': 'application/json' } }, false)
  },
  async logout(): Promise<void> {
    await req('api/v1/logout', { method: 'POST' }, false)
  },
}
