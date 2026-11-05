<script setup lang="ts">
import { ref } from 'vue'

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
</script>

<template>
  <header class="top">
    <a href="#/" class="brand">Kite</a>
    <button class="theme" :title="`Switch to ${theme === 'dark' ? 'light' : 'dark'} theme`" @click="toggleTheme">
      {{ theme === 'dark' ? 'Light' : 'Dark' }}
    </button>
  </header>
  <main class="main"></main>
</template>

<style scoped>
.top { display: flex; align-items: center; gap: 12px; padding: 12px 16px; border-bottom: 1px solid var(--line); }
.brand { font-weight: 700; font-size: 16px; text-decoration: none; letter-spacing: 0.02em; }
.theme { margin-left: auto; font-size: 12px; }
.main { padding: 16px; max-width: 1400px; margin: 0 auto; }
</style>
