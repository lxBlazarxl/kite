<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import LoginView from './components/LoginView.vue'
import MachineDetail from './components/MachineDetail.vue'
import OverviewTable from './components/OverviewTable.vue'
import { api } from './lib/api'
import { nowSec } from './lib/clock'
import { formatAgo } from './lib/format'
import { useRoute } from './lib/router'
import { loadMachines, startStream, store } from './lib/store'

type Theme = 'dark' | 'light'

function initialTheme(): Theme {
  try {
    const saved = localStorage.getItem('kite.theme')
    if (saved === 'dark' || saved === 'light') return saved
  } catch {
    // storage blocked: fall back to the system setting
  }
  return window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark'
}

const theme = ref<Theme>(initialTheme())
document.documentElement.dataset.theme = theme.value

function toggleTheme() {
  theme.value = theme.value === 'dark' ? 'light' : 'dark'
  document.documentElement.dataset.theme = theme.value
  try {
    localStorage.setItem('kite.theme', theme.value)
  } catch {
    // ignore
  }
}

const route = useRoute()
let stopStream: (() => void) | null = null

async function connect() {
  stopStream?.()
  try {
    store.session = await api.session()
    await loadMachines()
  } catch {
    return // a 401 already redirected to login
  }
  stopStream = startStream()
}

onMounted(connect)
onUnmounted(() => stopStream?.())

async function logout() {
  await api.logout()
  stopStream?.()
  stopStream = null
  store.machines = {}
  window.location.hash = '/login'
}

const counts = computed(() => {
  const ms = Object.values(store.machines)
  return { total: ms.length, online: ms.filter((m) => m.online).length }
})
</script>

<template>
  <header class="top">
    <a href="#/" class="brand">Kite</a>
    <span v-if="route.name !== 'login'" class="conn" :class="{ live: store.connected }">
      {{ store.connected ? 'live' : 'reconnecting' }}
    </span>
    <button class="theme" :title="`Switch to ${theme === 'dark' ? 'light' : 'dark'} theme`" @click="toggleTheme">
      {{ theme === 'dark' ? 'Light' : 'Dark' }}
    </button>
  </header>

  <main class="main">
    <LoginView v-if="route.name === 'login'" :next="route.next" @logged-in="connect" />
    <OverviewTable v-else-if="route.name === 'overview'" />
    <MachineDetail v-else :key="route.machine" :name="route.machine" :range="route.range" />
  </main>

  <footer v-if="route.name !== 'login'" class="foot muted">
    <span>{{ counts.total }} machines · {{ counts.online }} online</span>
    <span v-if="store.lastUpdate"> · updated {{ formatAgo(nowSec - store.lastUpdate) }}</span>
    <a v-if="store.session?.password_required" href="#" class="logout" @click.prevent="logout">Log out</a>
  </footer>
</template>

<style scoped>
.top { display: flex; align-items: center; gap: 12px; padding: 12px 16px; border-bottom: 1px solid var(--line); }
.brand { font-weight: 700; font-size: 16px; text-decoration: none; letter-spacing: 0.02em; }
.conn { font-size: 11px; color: var(--muted); border: 1px solid var(--line-strong); border-radius: 8px; padding: 1px 7px; }
.conn.live { color: var(--ok); border-color: color-mix(in srgb, var(--ok) 50%, transparent); }
.theme { margin-left: auto; font-size: 12px; }
.main { padding: 16px; max-width: 1400px; margin: 0 auto; }
.foot { max-width: 1400px; margin: 0 auto; padding: 0 16px 24px; font-size: 12px; display: flex; gap: 4px; flex-wrap: wrap; }
.logout { margin-left: auto; }
</style>
