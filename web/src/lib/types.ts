// Mirrors the JSON written by the Go hub (internal/protocol and internal/hub).

export interface CPU {
  model?: string
  total_pct: number
  per_core_pct: number[] | null
  freq_mhz?: number
  temp_c?: number
}

export interface Memory {
  total_bytes: number
  used_bytes: number
  swap_total_bytes: number
  swap_used_bytes: number
}

export interface GPU {
  index: number
  vendor: string
  name: string
  util_pct?: number
  vram_used_bytes?: number
  vram_total_bytes?: number
  temp_c?: number
  power_w?: number
}

export interface Power {
  cpu_w?: number
  platform_w?: number
}

/** watts is negative while discharging, positive while charging. */
export interface Battery {
  percent: number
  state: string
  watts?: number
}

export interface Disk {
  mount: string
  device: string
  fs_type: string
  total_bytes: number
  used_bytes: number
  read_bps?: number
  write_bps?: number
}

export interface NetIf {
  name: string
  rx_bps: number
  tx_bps: number
}

export interface Container {
  name: string
  image: string
  state: string
  cpu_pct?: number
  mem_bytes?: number
}

export interface Sample {
  v: number
  machine: string
  os: string
  hostname: string
  time: string
  interval_sec: number
  uptime_sec?: number
  platform?: string
  agent_version?: string
  /** Motherboard/ACPI sensor, reported where no CPU sensor exists (Windows). */
  system_temp_c?: number
  cpu?: CPU
  memory?: Memory
  gpus?: GPU[]
  power?: Power
  battery?: Battery
  disks?: Disk[]
  net?: NetIf[]
  docker?: { containers: Container[] }
  unavailable?: Record<string, string>
}

export interface MachineInfo {
  name: string
  os: string
  online: boolean
  last_seen: number
  interval_sec: number
  latest: Sample
}

export type Resolution = 'raw' | '1m' | '10m'

/** Raw rows are [ts, v]; 1m and 10m rows are [ts, avg, max]. */
export interface SeriesResponse {
  resolution: Resolution
  step: number
  series: Record<string, number[][]>
}

export interface SampleFrame {
  machine: string
  online: boolean
  ts: number
  sample: Sample
}

export type StatusFrame = { name: string; online: boolean }[]

export interface SessionInfo {
  password_required: boolean
  logged_in: boolean
  version?: string
}
