import type { Unit } from './series'
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

/** Formats a chart value in its unit; null or undefined gives ''. */
export function formatUnit(unit: Unit, v: number | null | undefined): string {
  if (v === null || v === undefined) return ''
  switch (unit) {
    case 'pct':
      return `${v.toFixed(v < 10 ? 1 : 0)}%`
    case 'bytes':
      return formatBytes(v)
    case 'bps':
      return formatRate(v)
    case 'watts':
      return formatWatts(v)
    case 'celsius':
      return `${v.toFixed(0)}°C`
  }
}

/** Top of a chart's y axis: fixed for percentages, with a floor elsewhere so flat lines get sensible ticks. */
export function yMax(unit: Unit, max: number): number {
  if (unit === 'pct') return 100
  const floor = unit === 'bytes' || unit === 'bps' ? 1024 : 1
  return Math.max(max * 1.1, floor)
}
