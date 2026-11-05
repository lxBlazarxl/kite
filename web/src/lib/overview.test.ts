import { describe, expect, it } from 'vitest'
import { level, livePointFromFrame, overviewRow, sortMachines, streamErrorAction } from './overview'
import type { MachineInfo, Sample } from './types'

const GB = 1024 ** 3
const base = { v: 1, os: 'linux', hostname: 'h', time: '2001-01-01T00:00:00Z', interval_sec: 5 }

function machine(name: string, online: boolean, extra: Partial<Sample> = {}): MachineInfo {
  return { name, os: 'linux', online, last_seen: 1000, interval_sec: 5, latest: { ...base, machine: name, ...extra } }
}

describe('overview', () => {
  it('level thresholds', () => {
    expect(level(79.9)).toBe('ok')
    expect(level(80)).toBe('warn')
    expect(level(90)).toBe('warn')
    expect(level(90.1)).toBe('crit')
    expect(level(undefined)).toBe('ok')
  })

  it('overviewRow for a laptop', () => {
    const r = overviewRow(machine('laptop-01', true, {
      platform: 'arch',
      cpu: { total_pct: 12, per_core_pct: [] },
      memory: { total_bytes: 31 * GB, used_bytes: 11.8 * GB, swap_total_bytes: 0, swap_used_bytes: 0 },
      gpus: [{ index: 0, vendor: 'amd', name: 'AMD GPU', util_pct: 4 }],
      battery: { percent: 57, state: 'discharging', watts: -8.2 },
      disks: [
        { mount: '/', device: 'a', fs_type: 'ext4', total_bytes: 100, used_bytes: 38 },
        { mount: '/boot', device: 'b', fs_type: 'vfat', total_bytes: 100, used_bytes: 10 },
      ],
    }), 1010)
    expect(r.cpu).toBe(12)
    expect(r.ram).toBeCloseTo(38.06, 1)
    expect(r.gpu).toBe(4)
    expect(r.power).toBe('8.2 W batt')
    expect(r.disk).toEqual({ pct: 38, mount: '/' })
    expect(r.platform).toBe('Arch')
    expect(r.lastSeenAgo).toBe('10 s ago')
  })

  it('overviewRow handles a machine with no gpu, battery or disks', () => {
    const r = overviewRow(machine('box', true, {
      cpu: { total_pct: 5, per_core_pct: null },
      memory: { total_bytes: 100, used_bytes: 50, swap_total_bytes: 0, swap_used_bytes: 0 },
    }), 1000)
    expect(r.gpu).toBeUndefined()
    expect(r.gpuNote).toBeUndefined()
    expect(r.power).toBeUndefined()
    expect(r.disk).toBeUndefined()
    expect(r.ram).toBe(50)
  })

  it('power falls back to gpu watts, then cpu watts', () => {
    const gpu = overviewRow(machine('rig', true, { gpus: [{ index: 0, vendor: 'nvidia', name: 'x', power_w: 186 }] }), 1000)
    expect(gpu.power).toBe('186 W GPU')
    const cpu = overviewRow(machine('srv', true, { power: { cpu_w: 9 } }), 1000)
    expect(cpu.power).toBe('9.0 W CPU')
    const charging = overviewRow(machine('lap', true, { battery: { percent: 50, state: 'charging', watts: 12 } }), 1000)
    expect(charging.power).toBe('charging')
  })

  it('gpu note when nvidia is asleep and no gpu reports', () => {
    const r = overviewRow(machine('lap', true, { unavailable: { 'gpu.nvidia': 'asleep' } }), 1000)
    expect(r.gpu).toBeUndefined()
    expect(r.gpuNote).toBe('asleep')
  })

  it('sortMachines puts online first then name', () => {
    const out = sortMachines([machine('b', true), machine('A', false), machine('a', true)])
    expect(out.map((m) => m.name)).toEqual(['a', 'b', 'A'])
  })

  it('livePointFromFrame uses frame ts', () => {
    const p = livePointFromFrame({
      machine: 'pc', online: true, ts: 1000,
      sample: { ...base, machine: 'pc', cpu: { total_pct: 7, per_core_pct: [] } },
    })
    expect(p.ts).toBe(1000)
    expect(p.values['cpu.total']).toBe(7)
  })

  it('streamErrorAction', () => {
    expect(streamErrorAction({ password_required: true, logged_in: false })).toBe('login')
    expect(streamErrorAction({ password_required: true, logged_in: true })).toBe('retry')
    expect(streamErrorAction({ password_required: false, logged_in: true })).toBe('retry')
  })
})
