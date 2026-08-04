import { ref, onUnmounted } from 'vue'
import type {
  AgentStartPayload, BlockStartPayload, BlockDeltaPayload,
  BlockStopPayload, ThreadStatusPayload, SSEEvent
} from '@/types'

export interface StreamHandlers {
  onAgentStart?: (p: AgentStartPayload) => void
  onBlockStart?: (p: BlockStartPayload) => void
  onBlockDelta?: (p: BlockDeltaPayload) => void
  onBlockStop?:  (p: BlockStopPayload)  => void
  onMessageAppended?: () => void
  onThreadStatus?: (p: ThreadStatusPayload) => void
  onRoundDone?: () => void
  onQueueDrained?: () => void
}

export function useSSE(convId: string, handlers: StreamHandlers) {
  const connected = ref(false)
  let es: EventSource | null = null

  function connect() {
    if (es) return
    es = new EventSource(`/api/conversations/${convId}/stream`)
    connected.value = true

    const parse = (e: MessageEvent): SSEEvent => JSON.parse(e.data)

    es.addEventListener('agent_start',      e => handlers.onAgentStart?.(parse(e).data as AgentStartPayload))
    es.addEventListener('block_start',      e => handlers.onBlockStart?.(parse(e).data as BlockStartPayload))
    es.addEventListener('block_delta',      e => handlers.onBlockDelta?.(parse(e).data as BlockDeltaPayload))
    es.addEventListener('block_stop',       e => handlers.onBlockStop?.(parse(e).data as BlockStopPayload))
    es.addEventListener('message_appended', () => handlers.onMessageAppended?.())
    es.addEventListener('thread_status',    e => handlers.onThreadStatus?.(parse(e).data as ThreadStatusPayload))
    es.addEventListener('round_done',       () => handlers.onRoundDone?.())
    es.addEventListener('queue_drained',    () => {
      handlers.onQueueDrained?.()
      disconnect()
    })
    es.onerror = () => { connected.value = false }
  }

  function disconnect() {
    es?.close()
    es = null
    connected.value = false
  }

  onUnmounted(disconnect)

  return { connect, disconnect, connected }
}
