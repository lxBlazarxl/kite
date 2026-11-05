import { ref, type Ref } from 'vue'

export type Range = 'live' | '1h' | '24h' | '7d'
export const RANGES: Range[] = ['live', '1h', '24h', '7d']

export type Route =
  | { name: 'overview' }
  | { name: 'machine'; machine: string; range: Range }
  | { name: 'login'; next: string }

export function parseHash(hash: string): Route {
  const h = hash.replace(/^#/, '')
  const [path, query = ''] = h.split('?')
  const params = new URLSearchParams(query)
  if (path.startsWith('/m/')) {
    const r = params.get('r') as Range | null
    return { name: 'machine', machine: decodeURIComponent(path.slice(3)), range: r && RANGES.includes(r) ? r : 'live' }
  }
  if (path === '/login') return { name: 'login', next: params.get('next') || '#/' }
  return { name: 'overview' }
}

export function machinePath(name: string, range: Range = 'live'): string {
  return `#/m/${encodeURIComponent(name)}${range === 'live' ? '' : `?r=${range}`}`
}

let route: Ref<Route> | null = null

export function useRoute(): Ref<Route> {
  if (!route) {
    route = ref(parseHash(window.location.hash))
    window.addEventListener('hashchange', () => {
      route!.value = parseHash(window.location.hash)
    })
  }
  return route
}

export function go(hash: string) {
  window.location.hash = hash.replace(/^#/, '')
}
