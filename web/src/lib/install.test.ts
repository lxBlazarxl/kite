import { describe, expect, it } from 'vitest'
import {
  defaultHub,
  dockerCommand,
  hubAddressWarning,
  hubUsable,
  linuxCommand,
  machineNameError,
  noDownloadsMessage,
  reportStatus,
  windowsCommand,
} from './install'

const HUB = 'http://hub.local:8420'

describe('install commands', () => {
  it('defaultHub strips the page path', () => {
    expect(defaultHub({ origin: 'http://h:8420', pathname: '/' })).toBe('http://h:8420')
    expect(defaultHub({ origin: 'https://x', pathname: '/kite/' })).toBe('https://x/kite')
    expect(defaultHub({ origin: 'https://x', pathname: '/kite/index.html' })).toBe('https://x/kite')
  })

  it('hubAddressWarning flags local addresses', () => {
    for (const h of ['http://localhost:8420', 'http://127.0.0.1:8420', 'http://[::1]:8420', 'http://0.0.0.0:8420', 'not a url', 'hub.local:8420', 'ftp://hub.local']) {
      expect(hubAddressWarning(h)).not.toBeNull()
    }
    expect(hubAddressWarning(HUB)).toBeNull()
  })

  it('commandsUseEditedHub', () => {
    expect(linuxCommand(HUB, 'tok', 'pc-01')).toBe(
      "curl -fsSL 'http://hub.local:8420/install.sh' | sudo sh -s -- --hub 'http://hub.local:8420' --token 'tok' --name 'pc-01'",
    )
  })

  it('windows command', () => {
    expect(windowsCommand(HUB, 'tok', 'pc-01')).toBe(
      "$env:KITE_HUB='http://hub.local:8420'; $env:KITE_TOKEN='tok'; $env:KITE_NAME='pc-01'; irm 'http://hub.local:8420/install.ps1' | iex",
    )
  })

  it('docker command builds before replacing the running agent', () => {
    const c = dockerCommand(HUB, 'tok', 'pc-01')
    expect(c.startsWith(
      "docker build -t kite-agent 'http://hub.local:8420/docker/agent.tar' && { docker rm -f kite-agent >/dev/null 2>&1; docker run -d --name kite-agent --restart unless-stopped --pid host --network host -v /:/host:ro,rslave -v /var/run/docker.sock:/var/run/docker.sock:ro -e KITE_HOST_ROOT=/host",
    )).toBe(true)
    expect(c).toContain("-e HUB_URL='http://hub.local:8420' -e AGENT_TOKEN='tok'")
    expect(c).toContain("-e AGENT_NAME='pc-01'")
    expect(c.endsWith(' kite-agent; }')).toBe(true)
  })

  it('quotes awkward values', () => {
    expect(linuxCommand(HUB, "a'b")).toContain("--token 'a'\\''b'")
    expect(windowsCommand(HUB, "a'b")).toContain("$env:KITE_TOKEN='a''b'")
  })

  it('omits an empty name', () => {
    expect(linuxCommand(HUB, 'tok', '  ')).not.toContain('--name')
    expect(windowsCommand(HUB, 'tok', '')).not.toContain('KITE_NAME')
    expect(dockerCommand(HUB, 'tok')).not.toContain('AGENT_NAME')
  })

  it('hubUsable needs http(s) and a host', () => {
    expect(hubUsable(HUB)).toBe(true)
    expect(hubUsable('https://x.example/kite')).toBe(true)
    expect(hubUsable('http://localhost:8420')).toBe(true) // only a warning, still usable here
    expect(hubUsable('hub.local:8420')).toBe(false)
    expect(hubUsable('')).toBe(false)
  })

  it('machineNameError uses the agent rule', () => {
    expect(machineNameError('')).toBeNull()
    expect(machineNameError('  ')).toBeNull()
    expect(machineNameError('pc-01')).toBeNull()
    expect(machineNameError('server_2.lan')).toBeNull()
    expect(machineNameError('Living Room')).not.toBeNull()
    expect(machineNameError('.hidden')).not.toBeNull()
    expect(machineNameError('a'.repeat(65))).not.toBeNull()
    expect(machineNameError('pc\u2019s')).not.toBeNull()
  })

  it('reportStatus only goes green after a report since the pop-up opened', () => {
    const opened = 1000
    const m = (name: string, online: boolean, last_seen: number) => ({ name, online, last_seen })
    // no name: an existing machine that keeps reporting is not "new"
    expect(reportStatus([m('a', true, 1010)], ['a'], opened)).toEqual({ state: 'waiting' })
    expect(reportStatus([m('a', true, 1010), m('b', true, 1005)], ['a'], opened)).toEqual({ state: 'online', name: 'b' })
    // a machine loaded from history but offline is not "online"
    expect(reportStatus([m('b', false, 900)], [], opened)).toEqual({ state: 'waiting' })
    // named, offline at open, reports later: green
    expect(reportStatus([m('pc-01', false, 900)], [], opened, 'pc-01')).toEqual({ state: 'waiting' })
    expect(reportStatus([m('pc-01', true, 1003)], [], opened, 'pc-01')).toEqual({ state: 'online', name: 'pc-01' })
    // named and already online at open: say so instead of claiming success
    expect(reportStatus([m('pc-01', true, 1003)], ['pc-01'], opened, 'pc-01')).toEqual({ state: 'already', name: 'pc-01' })
  })

  it('noDownloadsMessage', () => {
    expect(noDownloadsMessage({ version: 'dev', token: 't', downloads: false })).not.toBeNull()
    expect(noDownloadsMessage({ version: 'v1', token: 't', downloads: true })).toBeNull()
  })
})
