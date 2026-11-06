<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import Chart from './Chart.vue'
import DataTable, { type Column } from './DataTable.vue'
import { api, HttpError } from '../lib/api'
import { formatBytes, formatPct, formatRate, formatUnit, formatUptime, platformLabel } from '../lib/format'
import { livePointFromFrame } from '../lib/overview'
import { go, machinePath, RANGES, type Range } from '../lib/router'
import { chartSpecs, isIdle, LiveBuffer, type ChartSpec } from '../lib/series'
import { onSample, removeMachine, store } from '../lib/store'

const props = defineProps<{ name: string; range: Range }>()

const LIVE_WINDOW = 900
const RANGE_SEC: Record<Exclude<Range, 'live'>, number> = { '1h': 3600, '24h': 86400, '7d': 7 * 86400 }

const specs = shallowRef<ChartSpec[]>([])
const data = shallowRef<Record<string, number[][]>>({})
const step = ref(5)
const band = ref(false)
const win = ref<[number, number]>([0, 0])
const notFound = ref(false)
const error = ref('')
const loading = ref(true)
const showCores = ref(false)

const machine = computed(() => store.machines[props.name])
const latest = computed(() => machine.value?.latest)

let unsubscribe: (() => void) | null = null
let generation = 0

async function load() {
  const gen = ++generation
  unsubscribe?.()
  unsubscribe = null
  loading.value = true
  error.value = ''
  notFound.value = false
  try {
    const keys = await api.getKeys(props.name)
    const sp = chartSpecs(keys)
    const wanted = sp.flatMap((s) => s.series.map((x) => x.key))
    const now = Date.now() / 1000
    if (props.range === 'live') {
      const resp = await api.getSeries(props.name, wanted, now - LIVE_WINDOW, now, 'raw')
      if (gen !== generation) return
      const buf = new LiveBuffer(LIVE_WINDOW)
      buf.seed(resp)
      const publish = () => {
        data.value = Object.fromEntries(wanted.map((k) => [k, buf.series(k)]))
        const end = Math.max(buf.latest, Date.now() / 1000)
        win.value = [end - LIVE_WINDOW, end]
      }
      step.value = resp.step || machine.value?.interval_sec || 5
      band.value = false
      specs.value = sp
      publish()
      unsubscribe = onSample((f) => {
        if (f.machine !== props.name) return
        const p = livePointFromFrame(f)
        buf.push(p.ts, p.values)
        publish()
      })
    } else {
      const span = RANGE_SEC[props.range]
      const resp = await api.getSeries(props.name, wanted, now - span, now)
      if (gen !== generation) return
      step.value = resp.step
      band.value = resp.resolution !== 'raw'
      specs.value = sp
      data.value = resp.series
      win.value = [now - span, now]
    }
  } catch (e) {
    if (gen !== generation) return
    if (e instanceof HttpError && e.status === 404) notFound.value = true
    else if (!(e instanceof HttpError && e.status === 401)) error.value = 'Could not load data from the hub.'
  } finally {
    if (gen === generation) loading.value = false
  }
}

watch(() => [props.name, props.range], load, { immediate: true })
// After the stream reconnects (sleep, network change) refetch the live window so
// the missed minutes are filled in instead of showing as a false gap.
watch(
  () => store.reconnects,
  () => {
    if (props.range === 'live') load()
  },
)
onBeforeUnmount(() => {
  generation++
  unsubscribe?.()
})

/** Charts whose keys have no points in this range (old disks, interfaces) are hidden. */
const withData = computed(() => specs.value.filter((s) => s.series.some((x) => (data.value[x.key]?.length ?? 0) > 0)))
/** Network interfaces with no traffic at all in this range are hidden too. */
const idleNets = computed(() => withData.value.filter((s) => s.id.startsWith('net:') && isIdle(s, data.value)))
const visible = computed(() => withData.value.filter((s) => !idleNets.value.includes(s)))
const mainCharts = computed(() => visible.value.filter((s) => !s.collapsed))
const coreChart = computed(() => visible.value.find((s) => s.collapsed))

/** Newest value of a chart's first series, for its title. Memory shows used / total. */
function current(s: ChartSpec): string {
  const last = (key: string) => data.value[key]?.at(-1)?.[1]
  if (s.id === 'memory' && last('mem.total') !== undefined) {
    return `${formatBytes(last('mem.used'))} / ${formatBytes(last('mem.total'))}`
  }
  return formatUnit(s.unit, last(s.series[0].key))
}

const containerCols: Column[] = [
  { key: 'name', label: 'Container' },
  { key: 'state', label: 'State' },
  { key: 'cpu_pct', label: 'CPU (% of one core)', numeric: true, format: (v) => (v === undefined ? '' : `${v.toFixed(1)}%`) },
  { key: 'mem_bytes', label: 'Memory', numeric: true, format: (v) => (v === undefined ? '' : formatBytes(v)) },
  { key: 'image', label: 'Image' },
]
const containers = computed(() => latest.value?.docker?.containers ?? [])

const diskCols: Column[] = [
  { key: 'mount', label: 'Mount' },
  { key: 'device', label: 'Device' },
  { key: 'pct', label: 'Used', numeric: true, bar: 'pct', format: (_v, r) => `${formatBytes(r.used_bytes)} / ${formatBytes(r.total_bytes)} (${formatPct(r.pct)})` },
]
const disks = computed(() =>
  (latest.value?.disks ?? []).map((d) => ({ ...d, pct: d.total_bytes ? (d.used_bytes / d.total_bytes) * 100 : 0 })),
)

const netCols: Column[] = [
  { key: 'name', label: 'Interface' },
  { key: 'rx_bps', label: 'In', numeric: true, format: (v) => formatRate(v) },
  { key: 'tx_bps', label: 'Out', numeric: true, format: (v) => formatRate(v) },
]
const nets = computed(() => latest.value?.net ?? [])

async function remove() {
  if (!confirm(`Remove ${props.name} and all of its history? It will come back if its agent keeps sending.`)) return
  try {
    await api.deleteMachine(props.name)
    removeMachine(props.name)
    go('#/')
  } catch {
    error.value = 'Could not remove the machine.'
  }
}
</script>

<template>
  <div v-if="notFound" class="center">
    <p>Machine "{{ name }}" not found.</p>
    <a href="#/">Back to all machines</a>
  </div>

  <div v-else class="detail">
    <header class="head">
      <a href="#/" class="back muted">← All machines</a>
      <span class="dot" :class="machine?.online ? 'on' : 'down'"></span>
      <h1>{{ name }}</h1>
      <span v-if="latest" class="muted info">
        {{ platformLabel(latest) }}<template v-if="latest.cpu?.model"> · {{ latest.cpu.model }}</template
        ><template v-if="latest.uptime_sec"> · up {{ formatUptime(latest.uptime_sec) }}</template
        ><template v-if="latest.agent_version"> · agent {{ latest.agent_version }}</template>
      </span>
      <span v-if="range === 'live'" class="live">● LIVE</span>
      <nav class="seg">
        <a v-for="r in RANGES" :key="r" :href="machinePath(name, r)" :class="{ act: r === range }">{{ r === 'live' ? 'Live' : r }}</a>
      </nav>
    </header>

    <p v-if="error" class="err">{{ error }}</p>
    <p v-else-if="loading && !specs.length" class="muted">Loading…</p>
    <p v-else-if="!mainCharts.length" class="muted">No data in this time range.</p>

    <section class="grid">
      <div v-for="s in mainCharts" :key="s.id + range" class="panel">
        <h2>{{ s.title }} <span class="cur num">{{ current(s) }}</span></h2>
        <Chart :spec="s" :data="data" :step="step" :band="band" :range="win" />
        <template v-if="s.id === 'cpu' && coreChart">
          <button class="toggle" @click="showCores = !showCores">{{ showCores ? 'Hide' : 'Show' }} cores</button>
          <Chart v-if="showCores" :spec="coreChart" :data="data" :step="step" :band="band" :range="win" />
        </template>
      </div>
    </section>

    <p v-if="idleNets.length" class="muted idle">
      Hidden, idle in this range (under 1 KB/s): {{ idleNets.map((s) => s.id.slice(4)).join(', ') }}
    </p>

    <section v-if="latest?.docker" class="panel">
      <h2>Containers <span class="muted">{{ containers.filter((c) => c.state === 'running').length }} / {{ containers.length }} running</span></h2>
      <DataTable :columns="containerCols" :rows="containers" :dim="(r) => r.state !== 'running'" initial-sort="name" />
    </section>

    <div class="two">
      <section v-if="disks.length" class="panel">
        <h2>Disks</h2>
        <DataTable :columns="diskCols" :rows="disks" initial-sort="mount" />
      </section>
      <section v-if="nets.length" class="panel">
        <h2>Network</h2>
        <DataTable :columns="netCols" :rows="nets" initial-sort="name" />
      </section>
    </div>

    <footer class="foot">
      <button class="danger" @click="remove">Remove this machine</button>
    </footer>
  </div>
</template>

<style scoped>
.head { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; margin-bottom: 16px; }
.back { text-decoration: none; margin-right: 6px; }
h1 { font-size: 20px; margin: 0; }
.info { font-size: 13px; }
.dot { width: 9px; height: 9px; border-radius: 50%; display: inline-block; }
.dot.on { background: var(--ok); }
.dot.down { background: var(--muted); }
.live { font-size: 11px; color: var(--ok); border: 1px solid color-mix(in srgb, var(--ok) 50%, transparent); border-radius: 8px; padding: 1px 7px; }
.seg { margin-left: auto; display: flex; border: 1px solid var(--line-strong); border-radius: 6px; overflow: hidden; }
.seg a { padding: 4px 12px; text-decoration: none; color: var(--muted); font-size: 13px; }
.seg a.act { background: var(--line); color: var(--text); }
.grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 460px), 1fr)); gap: 12px; margin-bottom: 12px; }
.panel { background: var(--panel); border: 1px solid var(--line); border-radius: 8px; padding: 12px 14px; margin-bottom: 12px; min-width: 0; }
.grid .panel { margin-bottom: 0; }
h2 { font-size: 13px; font-weight: 600; margin: 0 0 8px; }
h2 .muted { font-weight: 400; margin-left: 6px; }
h2 .cur { float: right; font-weight: 600; }
.toggle { font-size: 12px; margin: 6px 0; }
.two { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 420px), 1fr)); gap: 12px; }
.foot { margin: 12px 0 0; text-align: right; }
.idle { font-size: 12px; margin: 0 0 12px; }
.danger { color: var(--crit); }
.err { color: var(--crit); }
.center { text-align: center; padding: 48px 16px; }
</style>
