<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useConversationStore } from '@/stores/conversations'
import * as api from '@/api'

const store = useConversationStore()
const router = useRouter()
const route  = useRoute()
const reportBusy = ref(false)

onMounted(() => store.fetchAll())

async function newChat() {
  const conv = await store.create()
  router.push(`/chat/${conv.ID}`)
}

async function runReport(type: 'daily' | 'weekly') {
  if (reportBusy.value) return
  reportBusy.value = true
  try {
    const { conversation_id } = await api.runReport(type)
    await store.fetchAll()
    router.push(`/chat/${conversation_id}`)
  } finally {
    reportBusy.value = false
  }
}
</script>

<template>
  <aside class="sidebar">
    <div class="logo">
      <span class="logo-mark">⟐</span>
      <span class="logo-text">agent<em>weave</em></span>
    </div>

    <nav class="nav">
      <RouterLink to="/chat" class="nav-link" :class="{ active: route.path === '/chat' }">
        <span class="icon">◈</span> Conversations
      </RouterLink>
      <RouterLink to="/skills" class="nav-link" :class="{ active: route.path.startsWith('/skills') }">
        <span class="icon">◇</span> Skills
      </RouterLink>
      <RouterLink to="/agents" class="nav-link" :class="{ active: route.path.startsWith('/agents') }">
        <span class="icon">◉</span> Agents
      </RouterLink>
    </nav>

    <div class="section-header">
      <span>Conversations</span>
      <button class="btn-new" @click="newChat" title="New chat">＋</button>
    </div>

    <div class="conv-list">
      <div
        v-for="conv in store.list"
        :key="conv.ID"
        class="conv-item"
        :class="{ active: route.params.id === conv.ID }"
        @click="router.push(`/chat/${conv.ID}`)"
      >
        <span class="conv-title">{{ conv.Title }}</span>
      </div>
    </div>

    <div class="reports">
      <button class="report-btn" :disabled="reportBusy" @click="runReport('daily')">
        Daily Report
      </button>
      <button class="report-btn" :disabled="reportBusy" @click="runReport('weekly')">
        Weekly Report
      </button>
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  width: var(--sidebar-w);
  flex-shrink: 0;
  height: 100vh;
  background: var(--surface);
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.logo {
  padding: 20px 16px 16px;
  display: flex;
  align-items: center;
  gap: 8px;
  border-bottom: 1px solid var(--border);
}
.logo-mark { font-size: 20px; color: var(--accent); }
.logo-text  { font-family: var(--font-serif); font-size: 16px; letter-spacing: .02em; }
.logo-text em { font-style: italic; color: var(--accent); }

.nav { padding: 12px 8px 4px; display: flex; flex-direction: column; gap: 2px; }
.nav-link {
  display: flex; align-items: center; gap: 8px;
  padding: 7px 10px; border-radius: var(--radius);
  color: var(--text-dim); font-size: 13px; font-weight: 500;
  transition: background .12s, color .12s;
}
.nav-link:hover, .nav-link.active { background: var(--accent-lo); color: var(--accent); }
.icon { font-size: 10px; opacity: .7; }

.section-header {
  padding: 16px 16px 6px;
  display: flex; justify-content: space-between; align-items: center;
  font-size: 10px; font-weight: 600; letter-spacing: .1em;
  text-transform: uppercase; color: var(--text-faint);
}
.btn-new {
  width: 20px; height: 20px; line-height: 1;
  border-radius: 4px; color: var(--text-dim);
  font-size: 16px; display: flex; align-items: center; justify-content: center;
  transition: background .12s, color .12s;
}
.btn-new:hover { background: var(--border-hi); color: var(--accent); }

.conv-list { flex: 1; overflow-y: auto; padding: 0 8px; }
.conv-item {
  padding: 7px 10px; border-radius: var(--radius);
  cursor: pointer; color: var(--text-dim); font-size: 12px;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  transition: background .1s, color .1s;
}
.conv-item:hover  { background: var(--border); color: var(--text); }
.conv-item.active { background: var(--accent-lo); color: var(--accent); }

.reports {
  padding: 12px 8px 16px;
  border-top: 1px solid var(--border);
  display: flex; flex-direction: column; gap: 4px;
}
.report-btn {
  padding: 6px 10px; border-radius: var(--radius);
  font-size: 12px; color: var(--text-dim); font-family: var(--font-sans);
  text-align: left; transition: background .12s, color .12s;
}
.report-btn:hover:not(:disabled) { background: var(--border-hi); color: var(--text); }
.report-btn:disabled { opacity: .4; cursor: not-allowed; }
</style>
