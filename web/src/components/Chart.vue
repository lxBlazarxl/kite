<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import uPlot from 'uplot'
import 'uplot/dist/uPlot.min.css'
import { formatUnit, yMax } from '../lib/format'
import { alignSeries, type ChartSpec } from '../lib/series'

const props = defineProps<{
  spec: ChartSpec
  data: Record<string, number[][]>
  step: number
  /** Rows are [ts, avg, max]: draw max as a faint band above avg. */
  band: boolean
  /** Visible time range in unix seconds. */
  range: [number, number]
}>()

const el = ref<HTMLDivElement>()
let plot: uPlot | null = null
let zoomed = false
let pending = false
let resizeObs: ResizeObserver | null = null
let themeObs: MutationObserver | null = null

const PALETTE = ['--cpu', '--ram', '--gpu', '--c4', '--c5', '--c6']
// Axis steps in round binary amounts (512 MB, 1 GB) for byte charts.
const BINARY_INCRS = [0, 1, 2, 3, 4, 5].flatMap((e) => [1, 2, 4, 8, 16, 32, 64, 128, 256, 512].map((m) => m * 1024 ** e))

function css(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim()
}

function aligned(): uPlot.AlignedData {
  return alignSeries(props.data, props.spec.series.map((s) => s.key), props.step, props.band) as uPlot.AlignedData
}

function build() {
  plot?.destroy()
  if (!el.value) return
  const muted = css('--muted')
  const grid = css('--line')
  const series: uPlot.Series[] = [{ value: (_u, ts) => (ts ? new Date(ts * 1000).toLocaleString() : '') }]
  const bands: uPlot.Band[] = []
  props.spec.series.forEach((s, i) => {
    const color = css(PALETTE[i % PALETTE.length])
    series.push({ label: s.label, stroke: color, width: 1.5, spanGaps: false, value: (_u, v) => formatUnit(props.spec.unit, v) })
    if (props.band) {
      const avgIdx = series.length - 1
      series.push({ label: `${s.label} peak`, stroke: 'transparent', width: 0, spanGaps: false, value: (_u, v) => formatUnit(props.spec.unit, v) })
      bands.push({ series: [series.length - 1, avgIdx], fill: color + '33' })
    }
  })
  const binary = props.spec.unit === 'bytes' || props.spec.unit === 'bps'
  const opts: uPlot.Options = {
    width: el.value.clientWidth,
    height: 170,
    series,
    bands,
    cursor: { drag: { x: true, y: false } },
    scales: {
      x: { time: true },
      y: { range: (_u, _min, max) => [0, yMax(props.spec.unit, max ?? 0)] },
    },
    axes: [
      { stroke: muted, grid: { stroke: grid, width: 1 }, ticks: { stroke: grid } },
      { stroke: muted, grid: { stroke: grid, width: 1 }, ticks: { stroke: grid }, size: 84, incrs: binary ? BINARY_INCRS : undefined, values: (_u, vals) => vals.map((v) => formatUnit(props.spec.unit, v)) },
    ],
    hooks: {
      setSelect: [() => (zoomed = true)],
    },
  }
  plot = new uPlot(opts, aligned(), el.value)
  plot.over.addEventListener('dblclick', () => {
    zoomed = false
    applyRange()
  })
  applyRange()
}

function applyRange() {
  if (!plot || zoomed) return
  plot.setScale('x', { min: props.range[0], max: props.range[1] })
}

function update() {
  if (document.hidden) {
    pending = true // redraw once the tab is visible again
    return
  }
  pending = false
  if (!plot) return
  plot.setData(aligned(), false)
  applyRange()
}

function onVisible() {
  if (!document.hidden && pending) update()
}

onMounted(() => {
  build()
  resizeObs = new ResizeObserver(() => {
    if (plot && el.value) plot.setSize({ width: el.value.clientWidth, height: 170 })
  })
  if (el.value) resizeObs.observe(el.value)
  // Colors come from CSS variables, so rebuild when the theme changes.
  themeObs = new MutationObserver(build)
  themeObs.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })
  document.addEventListener('visibilitychange', onVisible)
})

onBeforeUnmount(() => {
  resizeObs?.disconnect()
  themeObs?.disconnect()
  document.removeEventListener('visibilitychange', onVisible)
  plot?.destroy()
  plot = null
})

watch(() => [props.data, props.range], update, { deep: false })
watch(() => [props.band, props.spec], () => {
  zoomed = false
  build()
})
</script>

<template>
  <div ref="el" class="chart"></div>
</template>

<style scoped>
.chart { width: 100%; min-height: 170px; }
.chart :deep(.u-legend) { font-size: 12px; color: var(--muted); text-align: left; }
.chart :deep(.u-legend .u-value) { color: var(--text); font-variant-numeric: tabular-nums; }
.chart :deep(.u-select) { background: color-mix(in srgb, var(--cpu) 15%, transparent); }
</style>
