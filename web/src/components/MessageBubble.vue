<script setup lang="ts">
import type { Message, ContentBlock } from '@/types'

const props = defineProps<{ message: Message; streaming?: boolean }>()

function textBlocks(msg: Message): ContentBlock[] {
  return msg.Content.filter(b => b.type === 'text')
}

function toolBlocks(msg: Message): ContentBlock[] {
  return msg.Content.filter(b => b.type === 'tool_use')
}

function isUser(msg: Message) { return msg.Role === 'user' }
</script>

<template>
  <div class="msg-wrap" :class="isUser(message) ? 'user' : 'assistant'">
    <div v-if="!isUser(message)" class="msg-label">
      <span v-if="message.AgentID" class="agent-badge">sub-agent</span>
      <span v-else class="agent-badge orchestrator">orchestrator</span>
    </div>

    <div class="bubble" :class="{ streaming }">
      <template v-if="textBlocks(message).length">
        <p v-for="(b, i) in textBlocks(message)" :key="i" class="text-block">{{ b.text }}</p>
      </template>
      <template v-else-if="!isUser(message) && streaming">
        <span class="thinking-dots"><span /><span /><span /></span>
      </template>

      <div v-if="toolBlocks(message).length" class="tools">
        <div v-for="(b, i) in toolBlocks(message)" :key="i" class="tool-chip">
          <span class="tool-icon">⚙</span> {{ b.name }}
        </div>
      </div>

      <span v-if="streaming && textBlocks(message).length" class="cursor" />
    </div>
  </div>
</template>

<style scoped>
.msg-wrap { display: flex; flex-direction: column; gap: 4px; animation: fade-in .2s ease; }
.msg-wrap.user { align-items: flex-end; }
.msg-wrap.assistant { align-items: flex-start; }

.msg-label { display: flex; gap: 6px; padding: 0 4px; }
.agent-badge {
  font-size: 10px; font-family: var(--font-mono);
  padding: 1px 6px; border-radius: 3px;
  background: var(--border); color: var(--text-dim);
  letter-spacing: .04em;
}
.agent-badge.orchestrator { background: var(--accent-lo); color: var(--accent); }

.bubble {
  max-width: 72%;
  padding: 10px 14px;
  border-radius: var(--radius-lg);
  position: relative;
  line-height: 1.7;
  font-size: 13.5px;
}
.user .bubble {
  background: var(--accent-lo);
  border: 1px solid rgba(200,169,110,.25);
  color: var(--text);
  border-bottom-right-radius: 3px;
}
.assistant .bubble {
  background: var(--surface);
  border: 1px solid var(--border);
  color: var(--text);
  border-bottom-left-radius: 3px;
}

.text-block { white-space: pre-wrap; word-break: break-word; }
.text-block + .text-block { margin-top: 8px; }

.cursor {
  display: inline-block; width: 2px; height: 14px;
  background: var(--accent); margin-left: 2px; vertical-align: middle;
  animation: blink .9s step-start infinite;
}

.thinking-dots { display: flex; gap: 4px; align-items: center; padding: 2px 0; }
.thinking-dots span {
  width: 5px; height: 5px; border-radius: 50%;
  background: var(--text-dim); animation: blink 1.2s ease infinite;
}
.thinking-dots span:nth-child(2) { animation-delay: .2s; }
.thinking-dots span:nth-child(3) { animation-delay: .4s; }

.tools { margin-top: 8px; display: flex; flex-wrap: wrap; gap: 4px; }
.tool-chip {
  display: flex; align-items: center; gap: 4px;
  font-size: 11px; font-family: var(--font-mono);
  padding: 2px 8px; border-radius: 3px;
  background: var(--border); color: var(--text-dim);
}
.tool-icon { font-size: 9px; opacity: .6; }
</style>
