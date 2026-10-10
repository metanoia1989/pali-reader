<script setup>
// The application bar. It is the same on every screen so the search field and
// the account control never move.
import { computed, onMounted, onBeforeUnmount, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { BookMarked, BookOpen, LogOut, Menu, Search, Settings2, User } from 'lucide-vue-next'
import { useAuth } from '../store/auth'
import { api, getToken } from '../api'
import { useSettings } from '../store/settings'
import SettingsPanel from './SettingsPanel.vue'
import BrandMark from './BrandMark.vue'
import { useReader } from '../store/reader'

defineProps({
  crumbs: { type: Array, default: () => [] },
  // The reading page does not carry these: its side panel already is the
  // catalogue, and the vocabulary list is reachable from the account menu.
  // Everywhere else they are the way back, written as words rather than as
  // icons that could mean anything.
  showNav: { type: Boolean, default: true },
})

const router = useRouter()
const auth = useAuth()
const S = useSettings()
const q = ref('')
const menu = ref(false)
const exporting = ref(false)
const exportError = ref('')
const searchEl = ref(null)

function submit() {
  const v = q.value.trim()
  if (v.length < 2) return
  router.push({ name: 'search', query: { q: v } })
}

function onKey(e) {
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
    e.preventDefault()
    searchEl.value?.focus()
  }
  if (e.key === 'Escape') S.panelOpen = false
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))

// The export used to be an ordinary <a href>. That cannot work: this app
// authenticates with an Authorization: Bearer header, and a browser navigation
// sends no such header — so the request arrived anonymous and the server
// answered 401, which the reader saw as 「请先登录」 while signed in.
// Fetching it lets the token travel, and the file is saved from the response.
async function exportData() {
  if (exporting.value) return
  exporting.value = true
  exportError.value = ''
  try {
    const res = await fetch(api.exportUrl, { headers: { Authorization: 'Bearer ' + getToken() } })
    if (!res.ok) throw new Error(res.status === 401 ? '登录已失效，请重新登录' : '导出失败（' + res.status + '）')
    const blob = await res.blob()
    // The server names the file; fall back to a date if it does not.
    const named = /filename="?([^";]+)"?/.exec(res.headers.get('content-disposition') || '')
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = named ? named[1] : 'pali-reader-' + new Date().toISOString().slice(0, 10) + '.json'
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
  } catch (e) {
    exportError.value = e.message || '导出失败'
  } finally {
    exporting.value = false
  }
}

async function signOut() {
  menu.value = false
  await auth.signOut()
  router.push('/')
}

const label = computed(() => auth.user?.displayName || auth.user?.email || '')
const R = useReader()
const route = useRoute()
const inReader = computed(() => route.path.startsWith('/read/'))
const isNarrow = ref(window.innerWidth < 900)
const onResize = () => { isNarrow.value = window.innerWidth < 900 }
onMounted(() => window.addEventListener('resize', onResize))
onBeforeUnmount(() => window.removeEventListener('resize', onResize))
</script>

<template>
  <header class="topbar" :class="{ 'is-hidden': R.barHidden }">
    <!-- On a phone, in the reader, this corner is the contents drawer.
         The drawer carries its own link back to the catalogue, so nothing is
         lost by spending the corner on the thing a reader reaches for first. -->
    <button
      v-if="inReader && isNarrow"
      class="brand brand-btn"
      title="典籍目录"
      aria-label="典籍目录"
      :aria-pressed="R.railDrawer"
      @click="R.railDrawer = !R.railDrawer"
    >
      <span class="mark"><Menu :size="19" /></span>
      <span class="nm">目录</span>
    </button>
    <router-link v-else to="/" class="brand" style="color: inherit">
      <span class="mark"><BrandMark :size="17" /></span>
      <span class="nm">巴利三藏阅读器</span>
    </router-link>

    <ul v-if="crumbs.length" class="crumbs">
      <li v-for="(c, i) in crumbs" :key="i">
        <span v-if="i" style="color: var(--border-strong)">›</span>
        <router-link v-if="c.to" :to="c.to" style="color: inherit">{{ c.label }}</router-link>
        <span v-else :class="{ cur: i === crumbs.length - 1 }">{{ c.label }}</span>
      </li>
    </ul>

    <div class="searchwrap">
      <form class="search" @submit.prevent="submit">
        <Search :size="15" />
        <input
          ref="searchEl"
          v-model="q"
          type="search"
          placeholder="搜索经文…"
          aria-label="搜索经文"
        />
        <span class="kbd">⌘K</span>
      </form>
    </div>

    <nav class="bar-actions">
      <template v-if="showNav">
        <router-link to="/catalog" class="btn btn-quiet btn-sm">全部典籍</router-link>
        <router-link to="/vocab" class="btn btn-quiet btn-sm">生词本</router-link>
      </template>
      <slot name="actions" />
      <button
        class="iconbtn"
        title="阅读设置"
        aria-label="阅读设置"
        :aria-pressed="S.panelOpen"
        @click="S.panelOpen = !S.panelOpen"
      >
        <Settings2 :size="18" />
      </button>

      <template v-if="auth.signedIn">
        <button class="avatar" :title="label" @click="menu = !menu">{{ auth.initial }}</button>
        <div v-if="menu" class="menu card" @click="menu = false">
          <div class="menu-head">
            <div style="font-weight: 500">{{ label }}</div>
            <div style="font-size: 11.5px; color: var(--meta)">{{ auth.user?.email }}</div>
          </div>
          <router-link to="/vocab" class="menu-item"><BookMarked :size="15" />生词本</router-link>
          <button class="menu-item" :disabled="exporting" @click="exportData">
            <Settings2 :size="15" />{{ exporting ? '正在导出…' : '导出我的数据' }}
          </button>
          <p v-if="exportError" class="menu-note">{{ exportError }}</p>
          <button class="menu-item" @click="signOut"><LogOut :size="15" />退出登录</button>
        </div>
      </template>
      <router-link v-else to="/login" class="btn btn-ghost btn-sm" style="margin-left: 6px">
        <User :size="15" />登录
      </router-link>
    </nav>

    <div v-if="S.panelOpen" class="set-anchor">
      <div class="set-scrim" @click="S.panelOpen = false" />
      <SettingsPanel class="set-panel" />
    </div>
  </header>
</template>

<style scoped>
.set-anchor {
  position: absolute;
  top: 50px;
  right: 10px;
  z-index: 55;
}
/* A full-screen catcher so a click anywhere closes the popover, without
   dimming the page: settings are a glance, not a modal. */
.set-scrim {
  position: fixed;
  inset: 0;
  z-index: -1;
}
.set-panel {
  position: relative;
}
.menu {
  position: absolute;
  top: 50px;
  right: 10px;
  z-index: 50;
  min-width: 210px;
  padding: 6px;
  box-shadow: var(--whisper);
}
.menu-head {
  padding: 8px 10px 10px;
  border-bottom: 1px solid var(--border-soft);
  margin-bottom: 4px;
}
/* A one-line report inside the menu. This is the only place the reader is
   looking when they press 导出, so it is where the answer belongs. */
.menu-note {
  margin: 4px 0 0;
  padding: 6px 12px 0;
  border-top: 1px solid var(--border-soft);
  font-size: 12px;
  color: var(--accent);
}
.menu-item {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 8px 10px;
  border-radius: var(--radius-sm);
  color: var(--fg-2);
  font-size: 13px;
  text-align: left;
}
.menu-item:hover {
  background: var(--surface-warm);
  text-decoration: none;
  color: var(--fg);
}
@media (max-width: 860px) {
  .brand .nm {
    display: none;
  }
  .crumbs {
    display: none;
  }
  .kbd {
    display: none;
  }
}
</style>
