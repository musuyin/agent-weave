<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useConversationStore } from '@/stores/conversations'

const store = useConversationStore()
const router = useRouter()

async function start() {
  const conv = await store.create()
  router.push(`/chat/${conv.ID}`)
}
</script>

<template>
  <div class="home">
    <div class="hero">
      <h1 class="headline">
        <em>Orchestrate</em> your<br />agent network.
      </h1>
      <p class="sub">Start a conversation, dispatch subagents, and watch things unfold.</p>
      <button class="cta" @click="start">New conversation →</button>
    </div>
  </div>
</template>

<style scoped>
.home {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 40px;
}
.hero { max-width: 480px; }
.headline {
  font-family: var(--font-serif);
  font-size: 42px;
  line-height: 1.2;
  color: var(--text);
  margin-bottom: 16px;
}
.headline em { color: var(--accent); font-style: italic; }
.sub { color: var(--text-dim); font-size: 14px; margin-bottom: 32px; line-height: 1.7; }
.cta {
  padding: 10px 22px;
  border-radius: var(--radius);
  background: var(--accent);
  color: var(--bg);
  font-family: var(--font-sans);
  font-size: 13px; font-weight: 600;
  letter-spacing: .03em;
  transition: background .15s, transform .1s;
}
.cta:hover { background: var(--accent-hi); transform: translateY(-1px); }
</style>
