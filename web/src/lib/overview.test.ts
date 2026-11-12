import { describe, expect, it } from 'vitest'
import { level, livePointFromFrame, overviewRow, scopeLabel, serviceOrder, serviceSummary, sortMachines, streamErrorAction } from './overview'
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

  it('serviceOrder puts failed first, then odd states, then running', () => {
    expect(serviceOrder({ active_state: 'failed', sub_state: 'failed' })).toBe(0)
    expect(serviceOrder({ active_state: 'active', sub_state: 'exited' })).toBe(1)
    expect(serviceOrder({ active_state: 'inactive', sub_state: 'dead' })).toBe(1)
    expect(serviceOrder({ active_state: 'active', sub_state: 'running' })).toBe(2)
    expect(serviceOrder({})).toBe(1)
  })

  it('serviceSummary counts failed and running', () => {
    const s = serviceSummary([
      { name: 'a', active_state: 'failed', sub_state: 'failed' },
      { name: 'b', active_state: 'failed', sub_state: 'failed' },
      { name: 'c', active_state: 'active', sub_state: 'running' },
      { name: 'd', active_state: 'inactive', sub_state: 'dead' },
    ])
    expect(s.total).toBe(4)
    expect(s.running).toBe(1)
    expect(s.failed).toBe(2)
    expect(s.rows.map((r) => r.order)).toEqual([0, 0, 2, 1])
    expect(s.rows[0].name).toBe('a') // original order kept; the table sorts
  })

  it('serviceSummary handles no systemd at all', () => {
    expect(serviceSummary(undefined)).toEqual({ rows: [], total: 0, running: 0, failed: 0, scopes: [] })
  })

  it('serviceSummary lists distinct scopes and keeps them per row', () => {
    const s = serviceSummary([
      { name: 'sshd.service', scope: 'system', active_state: 'active', sub_state: 'running' },
      { name: 'dbus.service', scope: 'system', active_state: 'active', sub_state: 'running' },
      { name: 'pipewire.service', scope: 'user', owner: 'alice', active_state: 'active', sub_state: 'running' },
      { name: 'gvfs.service', scope: 'user', owner: 'alice', active_state: 'failed', sub_state: 'failed' },
    ])
    expect(s.scopes).toEqual(['system', 'user'])
    expect(s.total).toBe(4)
    expect(s.failed).toBe(1)
    expect(s.rows.filter((r) => r.scope === 'user')).toHaveLength(2)
  })

  it('treats a missing scope as system', () => {
    const s = serviceSummary([{ name: 'ssh.service', active_state: 'active', sub_state: 'running' }])
    expect(s.scopes).toEqual(['system'])
  })

  it('scopeLabel names the owning user', () => {
    expect(scopeLabel({ scope: 'system' })).toBe('system')
    expect(scopeLabel({ scope: 'user', owner: 'alice' })).toBe('user · alice')
    expect(scopeLabel({})).toBe('system')
  })
})
