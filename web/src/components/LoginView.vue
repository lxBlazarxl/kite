<script setup lang="ts">
import { ref } from 'vue'
import { api, HttpError } from '../lib/api'
import { go } from '../lib/router'
import { loadMachines, store } from '../lib/store'

const props = defineProps<{ next: string }>()
const emit = defineEmits<{ loggedIn: [] }>()
const password = ref('')
const error = ref('')
const busy = ref(false)

async function submit() {
  busy.value = true
  error.value = ''
  try {
    await api.login(password.value)
    store.session = await api.session()
    await loadMachines()
    emit('loggedIn')
    go(props.next || '#/')
  } catch (e) {
    error.value = e instanceof HttpError && e.status === 401 ? 'Wrong password' : 'Could not reach the hub'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <form class="login" @submit.prevent="submit">
    <h1>Kite</h1>
    <label for="pw" class="muted">Password</label>
    <input id="pw" v-model="password" type="password" autocomplete="current-password" autofocus />
    <button type="submit" :disabled="busy || !password">Log in</button>
    <p v-if="error" class="err">{{ error }}</p>
  </form>
</template>

<style scoped>
.login { max-width: 320px; margin: 12vh auto; display: flex; flex-direction: column; gap: 10px; padding: 24px; background: var(--panel); border: 1px solid var(--line); border-radius: 10px; }
h1 { margin: 0 0 8px; font-size: 20px; }
input { font: inherit; color: var(--text); background: var(--bg); border: 1px solid var(--line-strong); border-radius: 6px; padding: 8px 10px; }
.err { color: var(--crit); margin: 0; }
</style>
