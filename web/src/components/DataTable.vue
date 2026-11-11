<script setup lang="ts">
import { computed, ref, watch } from 'vue'

export interface Column {
  key: string
  label: string
  format?: (v: any, row: Record<string, any>) => string
  numeric?: boolean
  /** Render a 0..100 bar from this row field next to the text. */
  bar?: string
}

const props = defineProps<{
  columns: Column[]
  rows: Record<string, any>[]
  dim?: (row: Record<string, any>) => boolean
  initialSort?: string
  /** Collapse to this many rows behind a "Show more" button; 0 shows all. */
  maxRows?: number
  /** Changing this collapses the table again (a different machine or filter). */
  resetKey?: string
}>()

const sortKey = ref(props.initialSort ?? props.columns[0]?.key)
const asc = ref(true)

function sortBy(key: string) {
  if (sortKey.value === key) asc.value = !asc.value
  else {
    sortKey.value = key
    asc.value = !props.columns.find((c) => c.key === key)?.numeric // numbers start largest first
  }
}

/** Rows currently revealed; only meaningful with maxRows. */
const shown = ref(props.maxRows ?? 0)
watch(
  () => props.resetKey,
  () => (shown.value = props.maxRows ?? 0),
)

const sorted = computed(() => {
  const k = sortKey.value
  if (!k) return props.rows
  const dir = asc.value ? 1 : -1
  return [...props.rows].sort((a, b) => {
    const x = a[k]
    const y = b[k]
    if (x === y) return 0
    if (x === undefined || x === null) return 1 // missing values last
    if (y === undefined || y === null) return -1
    return (typeof x === 'number' && typeof y === 'number' ? x - y : String(x).localeCompare(String(y))) * dir
  })
})

// Sorting always covers every row; only the rendering is limited, so clicking
// a column header still sorts the whole list.
const visible = computed(() => (shown.value > 0 ? sorted.value.slice(0, shown.value) : sorted.value))
const hidden = computed(() => sorted.value.length - visible.value.length)
const initial = computed(() => props.maxRows ?? 0)
const step = computed(() => Math.max(initial.value, 25))

// "Show less" only applies once the list has grown past its initial size, so
// the control never takes the table below where it started.
const canLess = computed(() => initial.value > 0 && visible.value.length > initial.value)

function showMore() {
  shown.value = Math.min(sorted.value.length, visible.value.length + step.value)
}
function showAll() {
  shown.value = sorted.value.length
}
function showLess() {
  shown.value = Math.max(initial.value, visible.value.length - step.value)
}

const cell = (c: Column, row: Record<string, any>) => (c.format ? c.format(row[c.key], row) : String(row[c.key] ?? ''))
</script>

<template>
  <div class="wrap">
    <table>
      <thead>
        <tr>
          <th v-for="c in columns" :key="c.key" :class="{ num: c.numeric }" @click="sortBy(c.key)">
            {{ c.label }}<span v-if="sortKey === c.key" class="arrow">{{ asc ? '▲' : '▼' }}</span>
          </th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(row, i) in visible" :key="i" :class="{ dim: dim?.(row) }">
          <td v-for="c in columns" :key="c.key" :class="{ num: c.numeric }">
            <span v-if="c.bar" class="bar"><i :style="{ width: `${Math.min(100, row[c.bar] ?? 0)}%` }"></i></span>
            {{ cell(c, row) }}
          </td>
        </tr>
        <tr v-if="hidden > 0 || canLess">
          <td :colspan="columns.length" class="more">
            <button v-if="hidden > 0" @click="showMore">Show more</button>
            <button v-if="hidden > 0" class="link" @click="showAll">Show all</button>
            <button v-if="canLess" class="link" @click="showLess">Show less</button>
            <span class="muted">{{ visible.length }} of {{ sorted.length }}</span>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
.wrap { overflow-x: auto; }
table { width: 100%; border-collapse: collapse; font-size: 13px; }
th { text-align: left; font-weight: 500; font-size: 12px; color: var(--muted); padding: 8px 10px; border-bottom: 1px solid var(--line); cursor: pointer; user-select: none; white-space: nowrap; }
td { padding: 7px 10px; border-bottom: 1px solid var(--line); white-space: nowrap; }
.num { text-align: right; font-variant-numeric: tabular-nums; }
tr.dim { opacity: 0.5; }
.arrow { font-size: 9px; margin-left: 4px; }
.bar { display: inline-block; width: 80px; height: 6px; background: var(--line); border-radius: 3px; overflow: hidden; vertical-align: middle; margin-right: 8px; }
.bar i { display: block; height: 100%; background: var(--cpu); }
.more { text-align: center; }
.more button { font-size: 12px; }
.more .link { border: 0; background: none; color: var(--muted); text-decoration: underline; margin-left: 10px; }
.more .muted { margin-left: 10px; font-size: 12px; }
</style>
