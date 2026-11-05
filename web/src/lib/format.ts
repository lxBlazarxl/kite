import type { Sample } from './types'

const UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']

function scaled(n: number): string {
  let i = 0
  while (Math.abs(n) >= 1024 && i < UNITS.length - 1) {
    n /= 1024
    i++
  }
  return i === 0 ? `${Math.round(n)} ${UNITS[i]}` : `${n.toFixed(1)} ${UNITS[i]}`
}

export function formatPct(v?: number): string {
  return v === undefined ? 'none' : `${Math.round(v)}%`
}

export function formatBytes(n?: number): string {
  return n === undefined ? 'none' : scaled(n)
}

/** Hub rates are bytes per second. */
export function formatRate(bps?: number): string {
  return bps === undefined ? 'none' : `${scaled(bps)}/s`
}

export function formatWatts(w?: number): string {
  return w === undefined ? 'none' : `${w.toFixed(1)} W`
}

export function formatAgo(sec: number): string {
  if (sec < 60) return `${Math.max(0, Math.round(sec))} s ago`
  if (sec < 3600) return `${Math.floor(sec / 60)} m ago`
  if (sec < 86400) return `${Math.floor(sec / 3600)} h ago`
  return `${Math.floor(sec / 86400)} d ago`
}

export function formatUptime(sec?: number): string {
  if (sec === undefined) return ''
  if (sec >= 86400) return `${Math.floor(sec / 86400)} d`
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  return h > 0 ? `${h} h ${m} m` : `${m} m`
}

export function platformLabel(s: Sample): string {
  const p = s.platform
  return p ? p.charAt(0).toUpperCase() + p.slice(1) : s.os
}
