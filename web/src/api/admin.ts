import type { TicketItem, TicketDetail, TicketRecord } from './ticket'
import request from '../utils/request'

export interface TicketListParams { page?: number; page_size?: number; status?: number }
export interface UpdateStatusParams { action: string; content?: string; operator_id?: number }
export interface AddRecordParams { action: string; content: string }

export function listAllTickets(params?: TicketListParams) {
  return request.get<{ data: TicketItem[]; total: number; page: number; page_size: number }>('/api/v1/admin/tickets', { params })
}

export function getTicketDetail(id: number) {
  return request.get<{ data: TicketDetail }>(`/api/v1/admin/tickets/${id}`)
}

export function updateTicketStatus(id: number, data: UpdateStatusParams) {
  return request.patch(`/api/v1/admin/tickets/${id}/status`, data)
}

export function addTicketRecord(id: number, data: AddRecordParams) {
  return request.post(`/api/v1/admin/tickets/${id}/records`, data)
}

export type { TicketItem, TicketDetail, TicketRecord }
