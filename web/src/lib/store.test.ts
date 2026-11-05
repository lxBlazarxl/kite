import { beforeEach, describe, expect, it } from 'vitest'
import { applySample, applyStatus, store } from './store'
import { parseHash } from './router'

const sample = { v: 1, machine: 'new', os: 'linux', hostname: 'h', time: '', interval_sec: 5 }

describe('store', () => {
  beforeEach(() => {
    for (const k of Object.keys(store.machines)) delete store.machines[k]
  })

  it('applySample adds an unknown machine', () => {
    applySample({ machine: 'new', online: true, ts: 1234, sample })
    expect(store.machines.new.online).toBe(true)
    expect(store.machines.new.last_seen).toBe(1234)
    expect(store.machines.new.latest.machine).toBe('new')
    expect(store.lastUpdate).toBe(1234)
  })

  it('applyStatus marks machines offline', () => {
    applySample({ machine: 'new', online: true, ts: 1, sample })
    applyStatus([{ name: 'new', online: false }, { name: 'ghost', online: true }])
    expect(store.machines.new.online).toBe(false)
    expect(store.machines.ghost).toBeUndefined()
  })
})

describe('router', () => {
  it('parseHash', () => {
    expect(parseHash('')).toEqual({ name: 'overview' })
    expect(parseHash('#/')).toEqual({ name: 'overview' })
    expect(parseHash('#/m/my%20pc')).toEqual({ name: 'machine', machine: 'my pc', range: 'live' })
    expect(parseHash('#/m/pc?r=7d')).toEqual({ name: 'machine', machine: 'pc', range: '7d' })
    expect(parseHash('#/m/pc?r=bogus')).toEqual({ name: 'machine', machine: 'pc', range: 'live' })
    expect(parseHash('#/login?next=%23%2Fm%2Fx')).toEqual({ name: 'login', next: '#/m/x' })
  })
})
