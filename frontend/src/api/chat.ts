import { apiClient } from './client'
import { getLocale } from '@/i18n'
import type {
  ChatApiKeyOption,
  ChatAttachment,
  ChatConversation,
  ChatMessage,
  ChatModel,
} from '@/types'

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api/v1'

export interface CreateChatConversationRequest {
  api_key_id: number
  model: string
  title?: string
}

export interface UpdateChatConversationRequest {
  api_key_id?: number
  model?: string
  title?: string
}

export interface SendChatMessageRequest {
  text?: string
  attachment_ids?: number[]
}

async function listAPIKeys(): Promise<ChatApiKeyOption[]> {
  const { data } = await apiClient.get<ChatApiKeyOption[]>('/chat/api-keys')
  return data
}

async function listModels(apiKeyId: number): Promise<ChatModel[]> {
  const { data } = await apiClient.get<ChatModel[]>('/chat/models', {
    params: { api_key_id: apiKeyId }
  })
  return data
}

async function listConversations(): Promise<ChatConversation[]> {
  const { data } = await apiClient.get<ChatConversation[]>('/chat/conversations')
  return data
}

async function createConversation(payload: CreateChatConversationRequest): Promise<ChatConversation> {
  const { data } = await apiClient.post<ChatConversation>('/chat/conversations', payload)
  return data
}

async function updateConversation(
  conversationId: number,
  payload: UpdateChatConversationRequest
): Promise<ChatConversation> {
  const { data } = await apiClient.patch<ChatConversation>(`/chat/conversations/${conversationId}`, payload)
  return data
}

async function deleteConversation(conversationId: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/chat/conversations/${conversationId}`)
  return data
}

async function listMessages(conversationId: number): Promise<ChatMessage[]> {
  const { data } = await apiClient.get<ChatMessage[]>(`/chat/conversations/${conversationId}/messages`)
  return data
}

async function uploadAttachment(conversationId: number, file: File): Promise<ChatAttachment> {
  const formData = new FormData()
  formData.append('file', file)
  const { data } = await apiClient.post<ChatAttachment>(
    `/chat/conversations/${conversationId}/attachments`,
    formData,
    {
      headers: {
        'Content-Type': 'multipart/form-data'
      }
    }
  )
  return data
}

async function deleteAttachment(attachmentId: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/chat/attachments/${attachmentId}`)
  return data
}

async function streamConversationMessage(
  conversationId: number,
  payload: SendChatMessageRequest,
  signal?: AbortSignal
): Promise<Response> {
  const token = localStorage.getItem('auth_token')
  const response = await fetch(`${API_BASE_URL}/chat/conversations/${conversationId}/messages/stream`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      'Accept-Language': getLocale()
    },
    body: JSON.stringify(payload),
    signal
  })
  return response
}

export const chatAPI = {
  listAPIKeys,
  listModels,
  listConversations,
  createConversation,
  updateConversation,
  deleteConversation,
  listMessages,
  uploadAttachment,
  deleteAttachment,
  streamConversationMessage
}

export default chatAPI
