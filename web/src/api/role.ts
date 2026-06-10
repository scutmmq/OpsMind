import request from '../utils/request'

export interface RoleItem {
  id: number
  name: string
  description: string
  permissions: string[]
  created_at?: string
  updated_at?: string
}

export function getRoleList() {
  return request.get<{ data: RoleItem[] }>('/api/v1/admin/roles')
}
