import { describe, expect, it } from 'vitest'
import { formatAgo, formatBytes, formatPct, formatRate, formatUptime, formatWatts, platformLabel } from './format'
import type { Sample } from './types'

describe('format', () => {
  it('formatPct', () => {
    expect(formatPct(12.4)).toBe('12%')
    expect(formatPct(undefined)).toBe('none')
  })
  it('formatBytes', () => {
    expect(formatBytes(4831838208)).toBe('4.5 GB')
    expect(formatBytes(512)).toBe('512 B')
    expect(formatBytes(undefined)).toBe('none')
  })
  it('formatRate', () => {
    expect(formatRate(2202009.6)).toBe('2.1 MB/s')
  })
  it('formatWatts', () => {
    expect(formatWatts(8.199)).toBe('8.2 W')
  })
  it('formatAgo', () => {
    expect(formatAgo(45)).toBe('45 s ago')
    expect(formatAgo(7200)).toBe('2 h ago')
    expect(formatAgo(172800)).toBe('2 d ago')
  })
  it('formatUptime', () => {
    expect(formatUptime(1036800)).toBe('12 d')
    expect(formatUptime(5400)).toBe('1 h 30 m')
    expect(formatUptime(undefined)).toBe('')
  })
  it('platformLabel', () => {
    const base = { v: 1, machine: 'x', hostname: 'x', time: '', interval_sec: 5 }
    expect(platformLabel({ ...base, os: 'linux', platform: 'arch' } as Sample)).toBe('Arch')
    expect(platformLabel({ ...base, os: 'windows' } as Sample)).toBe('windows')
  })
})
