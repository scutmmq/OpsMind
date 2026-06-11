/**
 * 问答状态管理 (Pinia)
 *
 * 管理当前问答会话状态、消息列表和加载状态。
 * 支持普通模式和 SSE 流式输出两种问答方式。
 *
 * 流式输出设计：
 * 流式模式下，先添加一条空的 assistant 消息占位，
 * 然后通过 onToken 回调逐步追加内容，实现打字机效果。
 * 流式完成后通过 onDone 回调更新完整的元数据（sources、session_id 等）。
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  streamChatSession,
  submitFeedback as submitFeedbackApi,
  type ChatSessionResponse,
} from '@/api/chat'

export const useChatStore = defineStore('chat', () => {
  // State
  const currentSession = ref<ChatSessionResponse | null>(null)
  const messages = ref<Array<{ role: string; content: string; sources?: any[]; isStreaming?: boolean }>>([])
  const loading = ref(false)
  const streaming = ref(false)  // 是否正在流式输出中
  const selectedKBID = ref<number | null>(null)

  // Actions

  /** 发送问题（SSE 流式模式，默认） */
  async function sendQuestion(question: string, kbID: number) {
    loading.value = true
    streaming.value = true
    selectedKBID.value = kbID

    // 添加用户消息
    messages.value.push({ role: 'user', content: question })

    // 添加 AI 消息占位（流式填充）
    const aiMsgIndex = messages.value.length
    messages.value.push({
      role: 'assistant',
      content: '',
      sources: [],
      isStreaming: true,
    })

    await streamChatSession(
      { question, kb_id: kbID },
      {
        onToken(content: string) {
          // 逐步追加 token 到 AI 消息
          const msg = messages.value[aiMsgIndex]
          if (msg) {
            msg.content += content
          }
        },
        onDone(session: ChatSessionResponse) {
          // 流式完成，更新会话元数据
          currentSession.value = session
          const msg = messages.value[aiMsgIndex]
          if (msg) {
            msg.content = session.answer
            msg.sources = session.sources
            msg.isStreaming = false
          }
          loading.value = false
          streaming.value = false
        },
        onError(error: string) {
          // 流式失败时移除占位消息，显示错误
          messages.value.splice(aiMsgIndex, 1)
          messages.value.push({
            role: 'assistant',
            content: `抱歉，${error || 'AI 服务暂时不可用，请稍后重试或提交申告。'}`,
          })
          loading.value = false
          streaming.value = false
        },
      }
    )
  }

  async function submitFeedback(feedback: number) {
    if (!currentSession.value) return
    try {
      await submitFeedbackApi(currentSession.value.session_id, feedback)
    } catch {
      // 静默失败
    }
  }

  function clearSession() {
    currentSession.value = null
    messages.value = []
  }

  return {
    currentSession,
    messages,
    loading,
    streaming,
    selectedKBID,
    sendQuestion,
    submitFeedback,
    clearSession,
  }
})
