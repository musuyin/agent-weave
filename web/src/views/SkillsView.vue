<script setup lang="ts">
import { ref, onMounted } from 'vue'
import type { Skill } from '@/types'
import * as api from '@/api'

const skills  = ref<Skill[]>([])
const editing = ref<Partial<Skill> | null>(null)
const error   = ref('')

async function load() { skills.value = await api.listSkills() }
onMounted(load)

function openCreate() {
  editing.value = { Name: '', Description: '', Body: '' }
  error.value = ''
}
function openEdit(s: Skill) {
  editing.value = { ...s }
  error.value = ''
}
function cancel() { editing.value = null }

async function save() {
  if (!editing.value) return
  const { ID, Name, Description, Body } = editing.value
  const payload = { name: Name ?? '', description: Description, body: Body ?? '' }
  try {
    if (ID) await api.updateSkill(ID, payload)
    else    await api.createSkill(payload)
    editing.value = null
    await load()
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : 'Error'
  }
}

async function del(s: Skill) {
  if (!confirm(`Delete skill "${s.Name}"?`)) return
  try { await api.deleteSkill(s.ID); await load() }
  catch (e: unknown) { alert(e instanceof Error ? e.message : 'Error') }
}
</script>

<template>
  <div class="page">
    <div class="page-header">
      <h2 class="page-title">Skills</h2>
      <button class="btn-primary" @click="openCreate">+ New skill</button>
    </div>

    <div class="card-grid">
      <div v-for="s in skills" :key="s.ID" class="card">
        <div class="card-top">
          <span class="card-name">{{ s.Name }}</span>
          <span v-if="s.IsSystem" class="badge">system</span>
        </div>
        <p class="card-desc">{{ s.Description || '—' }}</p>
        <div class="card-actions" v-if="!s.IsSystem">
          <button class="btn-text" @click="openEdit(s)">Edit</button>
          <button class="btn-text danger" @click="del(s)">Delete</button>
        </div>
      </div>
    </div>

    <div v-if="editing" class="modal-backdrop" @click.self="cancel">
      <div class="modal">
        <h3 class="modal-title">{{ editing.ID ? 'Edit Skill' : 'New Skill' }}</h3>
        <label class="field">
          <span>Name</span>
          <input v-model="editing.Name" type="text" />
        </label>
        <label class="field">
          <span>Description</span>
          <input v-model="editing.Description" type="text" />
        </label>
        <label class="field">
          <span>Body</span>
          <textarea v-model="editing.Body" rows="10" />
        </label>
        <p v-if="error" class="error">{{ error }}</p>
        <div class="modal-actions">
          <button class="btn-ghost" @click="cancel">Cancel</button>
          <button class="btn-primary" @click="save">Save</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.page { flex: 1; display: flex; flex-direction: column; height: 100vh; overflow: hidden; }
.page-header {
  padding: 20px 24px 16px;
  border-bottom: 1px solid var(--border);
  display: flex; align-items: center; justify-content: space-between;
  flex-shrink: 0;
}
.page-title { font-family: var(--font-serif); font-size: 22px; }

.card-grid {
  flex: 1; overflow-y: auto; padding: 20px 24px;
  display: grid; grid-template-columns: repeat(auto-fill, minmax(260px,1fr)); gap: 12px;
  align-content: start;
}
.card {
  background: var(--surface); border: 1px solid var(--border);
  border-radius: var(--radius-lg); padding: 14px 16px;
  display: flex; flex-direction: column; gap: 6px;
  transition: border-color .15s;
}
.card:hover { border-color: var(--border-hi); }
.card-top { display: flex; align-items: center; gap: 8px; }
.card-name { font-size: 13px; font-weight: 500; }
.badge {
  font-size: 10px; padding: 1px 6px; border-radius: 3px;
  background: var(--border); color: var(--text-faint); font-family: var(--font-mono);
}
.card-desc { font-size: 12px; color: var(--text-dim); flex: 1; }
.card-actions { display: flex; gap: 8px; margin-top: 4px; }

/* Buttons */
.btn-primary {
  padding: 6px 14px; border-radius: var(--radius);
  background: var(--accent); color: var(--bg);
  font-size: 12px; font-weight: 600; font-family: var(--font-sans);
  transition: background .12s;
}
.btn-primary:hover { background: var(--accent-hi); }
.btn-ghost {
  padding: 6px 14px; border-radius: var(--radius);
  border: 1px solid var(--border-hi); color: var(--text-dim);
  font-size: 12px; font-family: var(--font-sans);
  transition: background .12s;
}
.btn-ghost:hover { background: var(--border); }
.btn-text { font-size: 11px; color: var(--text-dim); font-family: var(--font-sans); }
.btn-text:hover { color: var(--text); }
.btn-text.danger:hover { color: var(--red); }

/* Modal */
.modal-backdrop {
  position: fixed; inset: 0; background: rgba(0,0,0,.6);
  display: flex; align-items: center; justify-content: center; z-index: 100;
}
.modal {
  background: var(--surface); border: 1px solid var(--border-hi);
  border-radius: var(--radius-lg); padding: 24px;
  width: 560px; max-width: 95vw; max-height: 80vh; overflow-y: auto;
  display: flex; flex-direction: column; gap: 14px;
  animation: fade-in .15s ease;
}
.modal-title { font-family: var(--font-serif); font-size: 18px; }
.field { display: flex; flex-direction: column; gap: 4px; font-size: 11px; color: var(--text-dim); font-family: var(--font-mono); text-transform: uppercase; letter-spacing: .06em; }
.field input, .field textarea { padding: 8px 10px; width: 100%; }
.field textarea { font-family: var(--font-mono); font-size: 12px; resize: vertical; }
.modal-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 4px; }
.error { font-size: 12px; color: var(--red); font-family: var(--font-mono); }
</style>
