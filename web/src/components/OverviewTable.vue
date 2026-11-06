<script setup lang="ts">
import { computed, ref } from 'vue'
import AddMachine from './AddMachine.vue'
import { nowSec } from '../lib/clock'
import { formatPct } from '../lib/format'
import { level, overviewRow, sortMachines } from '../lib/overview'
import { go, machinePath } from '../lib/router'
import { store } from '../lib/store'

const rows = computed(() => sortMachines(Object.values(store.machines)).map((m) => overviewRow(m, nowSec.value)))
const width = (v?: number) => `${Math.max(0, Math.min(100, v ?? 0))}%`
const adding = ref(false)
</script>

<template>
  <AddMachine v-if="adding" @close="adding = false" />
  <div v-if="rows.length === 0" class="empty">
    <p>No machines yet.</p>
    <button class="add" @click="adding = true">+ Add machine</button>
  </div>
  <template v-else>
  <div class="toolbar">
    <button class="add" @click="adding = true">+ Add machine</button>
  </div>
  <div class="wrap">
    <table>
      <thead>
        <tr>
          <th>Machine</th>
          <th>CPU</th>
          <th>RAM</th>
          <th>GPU</th>
          <th>Power</th>
          <th>Fullest disk</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="r in rows" :key="r.name" :class="{ off: !r.online }" @click="go(machinePath(r.name))">
          <td class="name">
            <span class="dot" :class="r.online ? 'on' : 'down'"></span>{{ r.name }}
            <span class="tag">{{ r.platform }}</span>
          </td>
          <template v-if="r.online">
            <td :class="level(r.cpu)">
              <span class="bar"><i class="cpu" :style="{ width: width(r.cpu) }"></i></span>
              <span class="num">{{ formatPct(r.cpu) }}</span>
            </td>
            <td :class="level(r.ram)">
              <span class="bar"><i class="ram" :style="{ width: width(r.ram) }"></i></span>
              <span class="num">{{ formatPct(r.ram) }}</span>
            </td>
            <td :class="level(r.gpu)">
              <template v-if="r.gpu !== undefined">
                <span class="bar"><i class="gpu" :style="{ width: width(r.gpu) }"></i></span>
                <span class="num">{{ formatPct(r.gpu) }}</span>
              </template>
              <span v-else class="muted">{{ r.gpuNote ?? 'none' }}</span>
            </td>
            <td class="num">
              <span v-if="r.power">{{ r.power }}</span><span v-else class="muted">none</span>
            </td>
            <td :class="level(r.disk?.pct)" class="num">
              <template v-if="r.disk">{{ r.disk.pct }}% {{ r.disk.mount }}</template>
              <span v-else class="muted">none</span>
            </td>
          </template>
          <td v-else colspan="5" class="muted">offline · last seen {{ r.lastSeenAgo }}</td>
        </tr>
      </tbody>
    </table>
  </div>
  </template>
</template>

<style scoped>
.wrap { overflow-x: auto; border: 1px solid var(--line); border-radius: 8px; background: var(--panel); }
table { width: 100%; border-collapse: collapse; min-width: 640px; }
th { text-align: left; font-weight: 500; font-size: 12px; color: var(--muted); padding: 10px 12px; border-bottom: 1px solid var(--line); }
td { padding: 10px 12px; border-bottom: 1px solid var(--line); white-space: nowrap; }
tbody tr { cursor: pointer; }
tbody tr:hover { background: color-mix(in srgb, var(--line) 50%, transparent); }
tbody tr:last-child td { border-bottom: 0; }
th:first-child, td.name { position: sticky; left: 0; background: var(--panel); z-index: 1; }
td.name { font-weight: 600; }
tr.off { opacity: 0.55; }
.dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; margin-right: 8px; vertical-align: 1px; }
.dot.on { background: var(--ok); }
.dot.down { background: var(--muted); }
.tag { font-weight: 400; font-size: 11px; color: var(--muted); background: var(--line); border-radius: 8px; padding: 1px 7px; margin-left: 8px; }
.bar { display: inline-block; width: 70px; height: 6px; background: var(--line); border-radius: 3px; overflow: hidden; vertical-align: middle; margin-right: 8px; }
.bar i { display: block; height: 100%; border-radius: 3px; }
.bar .cpu { background: var(--cpu); }
.bar .ram { background: var(--ram); }
.bar .gpu { background: var(--gpu); }
td.warn .num, td.warn.num { color: var(--warn); }
td.crit .num, td.crit.num { color: var(--crit); }
td.warn .bar i { background: var(--warn); }
td.crit .bar i { background: var(--crit); }
.empty { text-align: center; padding: 48px 16px; }
.toolbar { display: flex; justify-content: flex-end; margin-bottom: 10px; }
.add { background: color-mix(in srgb, var(--ok) 30%, var(--panel)); }
</style>
