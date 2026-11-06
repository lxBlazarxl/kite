import { describe, expect, it } from 'vitest'
import { defaultHub, dockerCommand, hubAddressWarning, linuxCommand, newMachineSeen, noDownloadsMessage, windowsCommand } from './install'

const HUB = 'http://hub.local:8420'

describe('install commands', () => {
  it('defaultHub strips the page path', () => {
    expect(defaultHub({ origin: 'http://h:8420', pathname: '/' })).toBe('http://h:8420')
    expect(defaultHub({ origin: 'https://x', pathname: '/kite/' })).toBe('https://x/kite')
    expect(defaultHub({ origin: 'https://x', pathname: '/kite/index.html' })).toBe('https://x/kite')
  })

  it('hubAddressWarning flags local addresses', () => {
    for (const h of ['http://localhost:8420', 'http://127.0.0.1:8420', 'http://[::1]:8420', 'http://0.0.0.0:8420', 'not a url']) {
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

  it('docker command starts with rm -f and mounts the host', () => {
    const c = dockerCommand(HUB, 'tok', 'pc-01')
    expect(c.startsWith(
      "docker rm -f kite-agent >/dev/null 2>&1; docker build -t kite-agent 'http://hub.local:8420/docker/agent.tar' && docker run -d --name kite-agent --restart unless-stopped --pid host --network host -v /:/host:ro,rslave -v /var/run/docker.sock:/var/run/docker.sock:ro -e KITE_HOST_ROOT=/host",
    )).toBe(true)
    expect(c).toContain("-e HUB_URL='http://hub.local:8420' -e AGENT_TOKEN='tok'")
    expect(c).toContain("-e AGENT_NAME='pc-01'")
    expect(c.endsWith(' kite-agent')).toBe(true)
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

  it('newMachineSeen', () => {
    expect(newMachineSeen(['a'], ['a', 'b'])).toBe('b')
    expect(newMachineSeen(['a'], ['a'], 'pc-01')).toBeNull()
    expect(newMachineSeen(['a'], ['a', 'pc-01'], 'pc-01')).toBe('pc-01')
    expect(newMachineSeen(['pc-01'], ['pc-01'], 'pc-01')).toBe('pc-01') // re-installing an existing machine
  })

  it('noDownloadsMessage', () => {
    expect(noDownloadsMessage({ version: 'dev', token: 't', downloads: false })).not.toBeNull()
    expect(noDownloadsMessage({ version: 'v1', token: 't', downloads: true })).toBeNull()
  })
})
