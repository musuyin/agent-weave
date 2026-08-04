import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { Conversation } from '@/types'
import * as api from '@/api'

export const useConversationStore = defineStore('conversations', () => {
  const list = ref<Conversation[]>([])
  const active = ref<Conversation | null>(null)

  async function fetchAll() {
    list.value = await api.listConversations()
  }

  async function create(title?: string) {
    const conv = await api.createConversation(title)
    list.value.unshift(conv)
    active.value = conv
    return conv
  }

  function setActive(conv: Conversation) {
    active.value = conv
  }

  return { list, active, fetchAll, create, setActive }
})
