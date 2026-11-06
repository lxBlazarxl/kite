// Builds the "Add machine" install commands. Kept free of Vue so it can be
// tested; the pop-up only wires these to inputs.

export interface InstallInfo {
  version: string
  token: string
  downloads: boolean
}

/** The hub address as this page sees it: origin plus base path, no trailing slash. */
export function defaultHub(loc: { origin: string; pathname: string }): string {
  const dir = loc.pathname.slice(0, loc.pathname.lastIndexOf('/') + 1)
  return (loc.origin + dir).replace(/\/+$/, '')
}

const LOCAL_HOSTS = new Set(['localhost', '0.0.0.0', '[::1]', '::1'])

/** A warning when other machines could not reach this address. */
export function hubAddressWarning(hub: string): string | null {
  let host: string
  try {
    host = new URL(hub).hostname
  } catch {
    return 'This is not a valid address, for example http://hub.local:8420.'
  }
  if (LOCAL_HOSTS.has(host) || host.startsWith('127.')) {
    return 'This address only works on this computer; enter one other machines can reach.'
  }
  return null
}

/** POSIX shell single quotes. */
const sq = (s: string) => `'${s.replace(/'/g, `'\\''`)}'`
/** PowerShell single quotes. */
const pq = (s: string) => `'${s.replace(/'/g, "''")}'`
const clean = (name?: string) => (name ?? '').trim()
const trimHub = (hub: string) => hub.trim().replace(/\/+$/, '')

export function linuxCommand(hub: string, token: string, name?: string): string {
  const h = trimHub(hub)
  const n = clean(name)
  return `curl -fsSL ${sq(`${h}/install.sh`)} | sudo sh -s -- --hub ${sq(h)} --token ${sq(token)}${n ? ` --name ${sq(n)}` : ''}`
}

export function windowsCommand(hub: string, token: string, name?: string): string {
  const h = trimHub(hub)
  const n = clean(name)
  return `$env:KITE_HUB=${pq(h)}; $env:KITE_TOKEN=${pq(token)};${n ? ` $env:KITE_NAME=${pq(n)};` : ''} irm ${pq(`${h}/install.ps1`)} | iex`
}

export function dockerCommand(hub: string, token: string, name?: string): string {
  const h = trimHub(hub)
  const n = clean(name)
  return [
    'docker rm -f kite-agent >/dev/null 2>&1;',
    `docker build -t kite-agent ${sq(`${h}/docker/agent.tar`)} &&`,
    'docker run -d --name kite-agent --restart unless-stopped --pid host --network host',
    '-v /:/host:ro,rslave -v /var/run/docker.sock:/var/run/docker.sock:ro -e KITE_HOST_ROOT=/host',
    `-e HUB_URL=${sq(h)} -e AGENT_TOKEN=${sq(token)}${n ? ` -e AGENT_NAME=${sq(n)}` : ''}`,
    'kite-agent',
  ].join(' ')
}

/**
 * Which machine the pop-up is waiting for has reported: the wanted name once
 * it is present, or with no name the first machine that was not there
 * when the pop-up opened.
 */
export function newMachineSeen(before: string[], now: string[], wanted?: string): string | null {
  const w = clean(wanted)
  if (w) return now.includes(w) ? w : null
  const old = new Set(before)
  return now.find((n) => !old.has(n)) ?? null
}

export function noDownloadsMessage(info: InstallInfo): string | null {
  return info.downloads ? null : 'This hub was built without agent downloads; install a release build of kite-hub to add machines from here.'
}
