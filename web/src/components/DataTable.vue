<script setup lang="ts">
import { computed, ref } from 'vue'

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
        <tr v-for="(row, i) in sorted" :key="i" :class="{ dim: dim?.(row) }">
          <td v-for="c in columns" :key="c.key" :class="{ num: c.numeric }">
            <span v-if="c.bar" class="bar"><i :style="{ width: `${Math.min(100, row[c.bar] ?? 0)}%` }"></i></span>
            {{ cell(c, row) }}
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
</style>
