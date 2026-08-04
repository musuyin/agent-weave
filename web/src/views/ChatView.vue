<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { Message, ContentBlock, ThreadStatusPayload } from '@/types'
import * as api from '@/api'
import { useSSE } from '@/composables/useSSE'
import MessageBubble from '@/components/MessageBubble.vue'
import { useConversationStore } from '@/stores/conversations'

const route  = useRoute()
const router = useRouter()
const store  = useConversationStore()

const convId = computed(() => route.params.id as string)

const messages      = ref<Message[]>([])
const streamBlocks  = ref<Record<string, string>>({})  // block_id → accumulated text
const streamingMsg  = ref<Message | null>(null)         // synthesized streaming message
const agentRunning  = ref(false)
const threadStatus  = ref<ThreadStatusPayload | null>(null)
const inputText     = ref('')
const sending       = ref(false)
const listEl        = ref<HTMLElement | null>(null)
const inputEl       = ref<HTMLTextAreaElement | null>(null)

// Build a fake streaming message from accumulated blocks
const allMessages = computed<Message[]>(() => {
  if (!streamingMsg.value) return messages.value
  return [...messages.value, streamingMsg.value]
})

function scrollBottom() {
  nextTick(() => {
    if (listEl.value) listEl.value.scrollTop = listEl.value.scrollHeight
  })
}

async function loadMessages() {
  try {
    messages.value = await api.listMessages(convId.value)
    scrollBottom()
  } catch { /* ignore */ }
}

// SSE handlers
const { connect, disconnect } = useSSE(convId.value, {
  onAgentStart() {
    agentRunning.value = true
    streamBlocks.value  = {}
    streamingMsg.value  = {
      ID: '__streaming__', ConversationID: convId.value,
      Role: 'assistant', AgentID: null, Compacted: false,
      Content: [], CreatedAt: new Date().toISOString()
    }
    scrollBottom()
  },
  onBlockStart(p) {
    if (p.block_type === 'text') streamBlocks.value[p.block_id] = ''
  },
  onBlockDelta(p) {
    if (streamBlocks.value[p.block_id] !== undefined) {
      streamBlocks.value[p.block_id] += p.text
      rebuildStreamingContent()
      scrollBottom()
    }
  },
  onBlockStop() { rebuildStreamingContent() },
  async onMessageAppended() { await loadMessages() },
  onThreadStatus(p) { threadStatus.value = p },
  onRoundDone() {
    agentRunning.value = false
    threadStatus.value = null
  },
  async onQueueDrained() {
    agentRunning.value  = false
    streamingMsg.value  = null
    streamBlocks.value  = {}
    threadStatus.value  = null
    await loadMessages()
  }
})

function rebuildStreamingContent() {
  if (!streamingMsg.value) return
  const textContent = Object.entries(streamBlocks.value).map(([, text]) => ({
    type: 'text' as const, text
  }))
  streamingMsg.value = { ...streamingMsg.value, Content: textContent }
}

async function send() {
  const content = inputText.value.trim()
  if (!content || sending.value || agentRunning.value) return
  sending.value = true
  inputText.value = ''
  try {
    connect()  // ensure SSE open before or at send
    const userMsg = await api.sendMessage(convId.value, content)
    messages.value.push(userMsg)
    scrollBottom()
  } catch (e: unknown) {
    console.error(e)
  } finally {
    sending.value = false
  }
}

async function cancel() {
  try { await api.cancelThreads(convId.value) } catch { /* */ }
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    send()
  }
}

// Watch route change to load new conv
watch(convId, async (id) => {
  if (!id) return
  disconnect()
  messages.value     = []
  streamingMsg.value = null
  streamBlocks.value = {}
  agentRunning.value = false
  await loadMessages()
  // find conv in store and set active
  const conv = store.list.find(c => c.ID === id)
  if (conv) store.setActive(conv)
}, { immediate: true })

onUnmounted(() => disconnect())
</script>

<template>
  <div class="chat">
    <header class="chat-header">
      <div class="conv-info">
        <span class="conv-name">{{ store.active?.Title ?? 'Conversation' }}</span>
      </div>
      <div class="header-actions">
        <div v-if="threadStatus" class="thread-status">
          <span class="pulse" />
          <span class="thread-label">{{ threadStatus.agent_name }} · {{ threadStatus.status }}</span>
        </div>
        <button v-if="agentRunning" class="btn-stop" @click="cancel">■ Stop</button>
      </div>
    </header>

    <div class="msg-list" ref="listEl">
      <div v-if="messages.length === 0 && !agentRunning" class="empty">
        <span>Send a message to begin.</span>
      </div>

      <MessageBubble
        v-for="msg in allMessages"
        :key="msg.ID"
        :message="msg"
        :streaming="msg.ID === '__streaming__' && agentRunning"
      />
    </div>

    <div class="input-area">
      <div class="input-row">
        <textarea
          ref="inputEl"
          v-model="inputText"
          class="input"
          placeholder="Message…"
          rows="1"
          :disabled="sending || agentRunning"
          @keydown="onKeydown"
        />
        <button
          class="btn-send"
          :disabled="!inputText.trim() || sending || agentRunning"
          @click="send"
        >
          ↑
        </button>
      </div>
      <div class="input-hint">Enter to send · Shift+Enter for newline</div>
    </div>
  </div>
</template>

<style scoped>
.chat {
  display: flex; flex-direction: column; height: 100vh;
}

.chat-header {
  padding: 14px 20px;
  border-bottom: 1px solid var(--border);
  display: flex; align-items: center; justify-content: space-between;
  flex-shrink: 0;
}
.conv-name { font-size: 13px; font-weight: 500; color: var(--text); }

.header-actions { display: flex; align-items: center; gap: 12px; }
.thread-status  { display: flex; align-items: center; gap: 6px; }
.thread-label   { font-size: 11px; font-family: var(--font-mono); color: var(--text-dim); }
.pulse {
  width: 6px; height: 6px; border-radius: 50%;
  background: var(--green); animation: blink .8s ease infinite;
}
.btn-stop {
  padding: 4px 10px; border-radius: var(--radius);
  background: rgba(224,108,117,.15); color: var(--red);
  font-size: 11px; font-family: var(--font-mono);
  border: 1px solid rgba(224,108,117,.3);
  transition: background .12s;
}
.btn-stop:hover { background: rgba(224,108,117,.25); }

.msg-list {
  flex: 1; overflow-y: auto;
  padding: 24px 20px;
  display: flex; flex-direction: column; gap: 12px;
}
.empty {
  margin: auto; color: var(--text-faint);
  font-size: 13px; font-family: var(--font-mono);
}

.input-area {
  flex-shrink: 0; padding: 12px 20px 16px;
  border-top: 1px solid var(--border);
}
.input-row { display: flex; gap: 8px; align-items: flex-end; }
.input {
  flex: 1; resize: none; min-height: 40px; max-height: 180px;
  padding: 9px 12px; font-size: 13px; line-height: 1.6;
  border-radius: var(--radius-lg);
  overflow-y: auto;
}
.input:disabled { opacity: .5; }
.btn-send {
  width: 36px; height: 36px; border-radius: 50%;
  background: var(--accent); color: var(--bg);
  font-size: 16px; font-weight: 700;
  display: flex; align-items: center; justify-content: center;
  flex-shrink: 0; transition: background .12s, transform .1s;
}
.btn-send:hover:not(:disabled) { background: var(--accent-hi); transform: scale(1.05); }
.btn-send:disabled { opacity: .4; cursor: default; }
.input-hint { font-size: 10px; color: var(--text-faint); margin-top: 6px; font-family: var(--font-mono); }
</style>
