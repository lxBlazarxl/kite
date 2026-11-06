<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api } from '../lib/api'
import {
  defaultHub,
  dockerCommand,
  hubAddressWarning,
  hubUsable,
  linuxCommand,
  machineNameError,
  noDownloadsMessage,
  reportStatus,
  windowsCommand,
  type InstallInfo,
} from '../lib/install'
import { machinePath } from '../lib/router'
import { store } from '../lib/store'

const emit = defineEmits<{ close: [] }>()

type Tab = 'linux' | 'windows' | 'docker'
const tab = ref<Tab>('linux')
const name = ref('')
const hub = ref(defaultHub(window.location))
const info = ref<InstallInfo | null>(null)
const error = ref('')
const copyLabel = ref('Copy')
// Snapshot taken once machines are loaded (after install info arrives), in
// hub time, so a machine counts as added only when it reports after this.
let openedAt = 0
let onlineAtOpen: string[] = []

onMounted(async () => {
  window.addEventListener('keydown', onKey)
  try {
    info.value = await api.installInfo()
  } catch {
    error.value = 'Could not load install details from the hub.'
  }
  openedAt = store.lastUpdate
  onlineAtOpen = Object.values(store.machines)
    .filter((m) => m.online)
    .map((m) => m.name)
})
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}

const warning = computed(() => hubAddressWarning(hub.value))
const nameError = computed(() => machineNameError(name.value))
const command = computed(() => {
  if (!info.value || !hubUsable(hub.value) || nameError.value) return ''
  const build = { linux: linuxCommand, windows: windowsCommand, docker: dockerCommand }[tab.value]
  return build(hub.value, info.value.token, name.value)
})
const hint = computed(
  () =>
    ({
      linux: 'Run on the new machine. Installs to /usr/local/bin with a systemd service; root lets it read CPU power.',
      windows: 'Run in an Administrator PowerShell on the new machine. Installs a Windows service that starts after boot.',
      docker: 'Run on a Linux machine with Docker 23 or newer (BuildKit). The agent reads the host through read-only mounts.',
    })[tab.value],
)
const status = computed(() => reportStatus(Object.values(store.machines), onlineAtOpen, openedAt, name.value))
const waitingFor = computed(() => name.value.trim() || 'the new machine')

function flash(label: string) {
  copyLabel.value = label
  setTimeout(() => (copyLabel.value = 'Copy'), 2000)
}

async function copy() {
  try {
    await navigator.clipboard.writeText(command.value)
    flash('Copied')
    return
  } catch {
    // navigator.clipboard only exists on https and localhost; a hub on plain
    // http needs the older copy command on a selection.
  }
  const el = document.getElementById('kite-install-cmd')
  if (!el) return
  window.getSelection()?.selectAllChildren(el)
  flash(document.execCommand('copy') ? 'Copied' : 'Press Ctrl+C')
}

// Close on a click outside the dialog, but not when a text selection that
// started inside it ends outside.
let downOnBackdrop = false
const onDown = (e: MouseEvent) => (downOnBackdrop = e.target === e.currentTarget)
const onUp = (e: MouseEvent) => {
  if (downOnBackdrop && e.target === e.currentTarget) emit('close')
  downOnBackdrop = false
}
</script>

<template>
  <div class="backdrop" @mousedown="onDown" @mouseup="onUp">
    <div class="modal" role="dialog" aria-label="Add a machine">
      <header>
        <h2>Add a machine</h2>
        <button class="close" title="Close" @click="emit('close')">✕</button>
      </header>

      <p v-if="error" class="err">{{ error }}</p>
      <p v-else-if="info && noDownloadsMessage(info)" class="warn">{{ noDownloadsMessage(info) }}</p>

      <template v-if="info && !noDownloadsMessage(info)">
        <nav class="tabs">
          <button v-for="t in ['linux', 'windows', 'docker'] as Tab[]" :key="t" :class="{ act: tab === t }" @click="tab = t">
            {{ t === 'linux' ? 'Linux' : t === 'windows' ? 'Windows' : 'Docker' }}
          </button>
        </nav>

        <label class="row">
          <span class="lbl muted">Name</span>
          <input v-model="name" placeholder="defaults to the hostname" spellcheck="false" />
        </label>
        <label class="row">
          <span class="lbl muted">Hub address</span>
          <input v-model="hub" spellcheck="false" />
        </label>
        <p v-if="nameError" class="warn">{{ nameError }}</p>
        <p v-if="warning" class="warn">{{ warning }}</p>

        <template v-if="command">
          <pre id="kite-install-cmd" class="cmd">{{ command }}</pre>
          <div class="row">
            <button class="primary" @click="copy">{{ copyLabel }}</button>
            <span class="muted small">{{ hint }}</span>
          </div>
        </template>
        <p v-if="!store.session?.password_required" class="muted small">
          Anyone who can open this dashboard can see the agent token. Set WEB_PASSWORD on a shared network.
        </p>

        <p v-if="status.state === 'online'" class="ok">
          ✓ {{ status.name }} is online · <a :href="machinePath(status.name)" @click="emit('close')">Open it</a>
        </p>
        <p v-else-if="status.state === 'already'" class="muted small">
          {{ status.name }} is already reporting; running the command again upgrades it.
        </p>
        <p v-else-if="command" class="waiting">● Waiting for {{ waitingFor }} to report…</p>
      </template>
    </div>
  </div>
</template>

<style scoped>
.backdrop { position: fixed; inset: 0; background: color-mix(in srgb, #000 55%, transparent); display: flex; align-items: flex-start; justify-content: center; padding: 8vh 16px; z-index: 10; }
.modal { width: min(720px, 100%); background: var(--panel); border: 1px solid var(--line-strong); border-radius: 10px; padding: 16px 18px; box-shadow: 0 12px 40px #0006; }
header { display: flex; align-items: center; margin-bottom: 8px; }
h2 { font-size: 16px; margin: 0; }
.close { margin-left: auto; border: 0; background: none; font-size: 16px; }
.tabs { display: flex; gap: 4px; margin: 6px 0 12px; }
.tabs button.act { background: var(--line); border-color: var(--muted); }
.row { display: flex; align-items: center; gap: 10px; margin: 6px 0; }
.row .lbl { width: 92px; flex: none; font-size: 13px; }
input { flex: 1; font: inherit; font-size: 13px; color: var(--text); background: var(--bg); border: 1px solid var(--line-strong); border-radius: 6px; padding: 6px 8px; }
.cmd { background: var(--bg); border: 1px solid var(--line-strong); border-radius: 6px; padding: 10px 12px; font-size: 12px; white-space: pre-wrap; overflow-wrap: anywhere; margin: 12px 0 8px; }
.primary { background: color-mix(in srgb, var(--ok) 30%, var(--panel)); }
.small { font-size: 12px; }
.warn { color: var(--warn); font-size: 13px; }
.err { color: var(--crit); }
.ok { color: var(--ok); font-size: 13px; margin-top: 12px; }
.waiting { color: var(--warn); font-size: 13px; margin-top: 12px; }
</style>
