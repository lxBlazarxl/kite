import { flattenSample } from './flatten'
import { formatAgo, formatWatts, platformLabel } from './format'
import type { MachineInfo, SampleFrame, SessionInfo } from './types'

export type Level = 'ok' | 'warn' | 'crit'

/** Color band for a percentage: below 80 ok, 80 to 90 warn, above 90 crit. */
export function level(pct?: number): Level {
  if (pct === undefined || pct < 80) return 'ok'
  return pct <= 90 ? 'warn' : 'crit'
}

export interface OverviewRow {
  name: string
  online: boolean
  platform: string
  cpu?: number
  ram?: number
  gpu?: number
  gpuNote?: string
  power?: string
  disk?: { pct: number; mount: string }
  lastSeenAgo: string
}

export function overviewRow(m: MachineInfo, nowSec: number): OverviewRow {
  const s = m.latest
  const row: OverviewRow = {
    name: m.name,
    online: m.online,
    platform: platformLabel(s),
    lastSeenAgo: formatAgo(nowSec - m.last_seen),
  }
  row.cpu = s.cpu?.total_pct
  if (s.memory && s.memory.total_bytes > 0) row.ram = (s.memory.used_bytes / s.memory.total_bytes) * 100

  const utils = (s.gpus ?? []).map((g) => g.util_pct).filter((v): v is number => v !== undefined)
  if (utils.length) row.gpu = Math.max(...utils)
  else if (s.unavailable?.['gpu.nvidia']) row.gpuNote = s.unavailable['gpu.nvidia']

  const gpuWatts = (s.gpus ?? []).map((g) => g.power_w).filter((v): v is number => v !== undefined)
  const bw = s.battery?.watts
  if (bw !== undefined && bw < 0) row.power = `${formatWatts(-bw)} batt`
  else if (bw !== undefined && bw > 0) row.power = 'charging'
  else if (gpuWatts.length) row.power = `${Math.round(gpuWatts.reduce((a, b) => a + b, 0))} W GPU`
  else if (s.power?.cpu_w !== undefined) row.power = `${formatWatts(s.power.cpu_w)} CPU`

  for (const d of s.disks ?? []) {
    if (d.total_bytes <= 0) continue
    const pct = Math.round((d.used_bytes / d.total_bytes) * 100)
    if (!row.disk || pct > row.disk.pct) row.disk = { pct, mount: d.mount }
  }
  return row
}

/** Online machines first, then by name (case-insensitive). */
export function sortMachines(ms: MachineInfo[]): MachineInfo[] {
  return [...ms].sort((a, b) => {
    if (a.online !== b.online) return a.online ? -1 : 1
    return a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }) || a.name.localeCompare(b.name)
  })
}

/** Live points use the hub's receive time so they line up with history. */
export function livePointFromFrame(f: SampleFrame): { ts: number; values: Record<string, number> } {
  return { ts: f.ts, values: flattenSample(f.sample) }
}

export function streamErrorAction(info: SessionInfo): 'login' | 'retry' {
  return info.password_required && !info.logged_in ? 'login' : 'retry'
}
