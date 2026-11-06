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

function parseHub(hub: string): URL | null {
  try {
    const u = new URL(hub.trim())
    // "hub.local:8420" parses as scheme "hub.local:" with no host.
    return (u.protocol === 'http:' || u.protocol === 'https:') && u.hostname ? u : null
  } catch {
    return null
  }
}

/** Whether commands can be built for this address at all. */
export function hubUsable(hub: string): boolean {
  return parseHub(hub) !== null
}

/** A warning when other machines could not reach this address. */
export function hubAddressWarning(hub: string): string | null {
  const u = parseHub(hub)
  if (!u) return 'Enter a full address starting with http:// or https://, for example http://hub.local:8420.'
  const host = u.hostname
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
  // Build first: if the hub is unreachable the running agent stays in place.
  return [
    `docker build -t kite-agent ${sq(`${h}/docker/agent.tar`)} &&`,
    '{ docker rm -f kite-agent >/dev/null 2>&1;',
    'docker run -d --name kite-agent --restart unless-stopped --pid host --network host',
    '-v /:/host:ro,rslave -v /var/run/docker.sock:/var/run/docker.sock:ro -e KITE_HOST_ROOT=/host',
    `-e HUB_URL=${sq(h)} -e AGENT_TOKEN=${sq(token)}${n ? ` -e AGENT_NAME=${sq(n)}` : ''}`,
    'kite-agent; }',
  ].join(' ')
}

const NAME_RE = /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/

/** Same rule the agent and hub enforce; empty means "use the hostname". */
export function machineNameError(name: string): string | null {
  const n = name.trim()
  if (!n || NAME_RE.test(n)) return null
  return "Use up to 64 letters, digits, '.', '_' or '-', starting with a letter or digit."
}

export interface MachineSeen {
  name: string
  online: boolean
  last_seen: number
}

export type ReportStatus = { state: 'waiting' } | { state: 'online' | 'already'; name: string }

/**
 * Whether the machine being added has reported. A machine only counts once
 * it is online and has reported since the pop-up opened; one that was
 * already online then is "already" reporting (re-running the command on it
 * changes nothing visible), not proof that the command worked.
 */
export function reportStatus(machines: MachineSeen[], onlineAtOpen: string[], openedAt: number, wanted?: string): ReportStatus {
  const was = new Set(onlineAtOpen)
  const fresh = (m: MachineSeen) => m.online && m.last_seen > openedAt
  const w = clean(wanted)
  if (w) {
    if (was.has(w)) return { state: 'already', name: w }
    const m = machines.find((x) => x.name === w)
    return m && fresh(m) ? { state: 'online', name: w } : { state: 'waiting' }
  }
  const m = machines.find((x) => !was.has(x.name) && fresh(x))
  return m ? { state: 'online', name: m.name } : { state: 'waiting' }
}

export function noDownloadsMessage(info: InstallInfo): string | null {
  return info.downloads ? null : 'This hub was built without agent downloads; install a release build of kite-hub to add machines from here.'
}
