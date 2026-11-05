import { expect, it } from 'vitest'
import { flattenSample } from './flatten'
import type { Sample } from './types'

const base = { v: 1, machine: 'pc', os: 'linux', hostname: 'pc', time: '', interval_sec: 5 }

it('flattenSample matches Go protocol.Flatten (TestFlattenKeys)', () => {
  const s: Sample = {
    ...base,
    cpu: { total_pct: 12.5, per_core_pct: [10, 15], temp_c: 55 },
    memory: { total_bytes: 8e9, used_bytes: 4e9, swap_total_bytes: 0, swap_used_bytes: 0 },
    gpus: [{ index: 0, vendor: '', name: '', util_pct: 30, power_w: 12.5 }],
    power: { cpu_w: 9 },
    battery: { percent: 80, state: 'discharging', watts: -7.5 },
    disks: [{ mount: '/data', device: '', fs_type: '', used_bytes: 100, total_bytes: 200, read_bps: 1024 }],
    net: [{ name: 'wlan0', rx_bps: 500, tx_bps: 250 }],
    docker: { containers: [{ name: 'proxy', image: '', state: 'running', cpu_pct: 3, mem_bytes: 1000 }] },
  }
  expect(flattenSample(s)).toEqual({
    'cpu.total': 12.5, 'cpu.core.0': 10, 'cpu.core.1': 15, 'cpu.temp_c': 55,
    'mem.total': 8e9, 'mem.used': 4e9, 'swap.total': 0, 'swap.used': 0,
    'gpu.0.util': 30, 'gpu.0.power_w': 12.5, 'power.cpu_w': 9,
    'battery.pct': 80, 'battery.w': -7.5,
    'disk./data.used': 100, 'disk./data.total': 200, 'disk./data.read_bps': 1024,
    'net.wlan0.rx_bps': 500, 'net.wlan0.tx_bps': 250,
    'docker.proxy.running': 1, 'docker.proxy.cpu_pct': 3, 'docker.proxy.mem': 1000,
  })
})

it('flattenSample of an empty sample is empty', () => {
  expect(flattenSample(base as Sample)).toEqual({})
})
