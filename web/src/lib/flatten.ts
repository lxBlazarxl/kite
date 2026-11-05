import type { Sample } from './types'

/** Mirror of Go protocol.Flatten: same keys, absent values omitted. */
export function flattenSample(s: Sample): Record<string, number> {
  const out: Record<string, number> = {}
  const put = (k: string, v: number | undefined) => {
    if (v !== undefined && v !== null) out[k] = v
  }
  if (s.cpu) {
    put('cpu.total', s.cpu.total_pct)
    ;(s.cpu.per_core_pct ?? []).forEach((v, i) => put(`cpu.core.${i}`, v))
    put('cpu.freq_mhz', s.cpu.freq_mhz)
    put('cpu.temp_c', s.cpu.temp_c)
  }
  if (s.memory) {
    put('mem.total', s.memory.total_bytes)
    put('mem.used', s.memory.used_bytes)
    put('swap.total', s.memory.swap_total_bytes)
    put('swap.used', s.memory.swap_used_bytes)
  }
  for (const g of s.gpus ?? []) {
    const p = `gpu.${g.index}.`
    put(p + 'util', g.util_pct)
    put(p + 'vram_used', g.vram_used_bytes)
    put(p + 'vram_total', g.vram_total_bytes)
    put(p + 'temp_c', g.temp_c)
    put(p + 'power_w', g.power_w)
  }
  if (s.power) {
    put('power.cpu_w', s.power.cpu_w)
    put('power.platform_w', s.power.platform_w)
  }
  if (s.battery) {
    put('battery.pct', s.battery.percent)
    put('battery.w', s.battery.watts)
  }
  for (const d of s.disks ?? []) {
    const p = `disk.${d.mount}.`
    put(p + 'used', d.used_bytes)
    put(p + 'total', d.total_bytes)
    put(p + 'read_bps', d.read_bps)
    put(p + 'write_bps', d.write_bps)
  }
  for (const n of s.net ?? []) {
    put(`net.${n.name}.rx_bps`, n.rx_bps)
    put(`net.${n.name}.tx_bps`, n.tx_bps)
  }
  for (const c of s.docker?.containers ?? []) {
    const p = `docker.${c.name}.`
    put(p + 'running', c.state === 'running' ? 1 : 0)
    put(p + 'cpu_pct', c.cpu_pct)
    put(p + 'mem', c.mem_bytes)
  }
  return out
}
