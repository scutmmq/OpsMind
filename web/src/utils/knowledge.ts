/**
 * 知识库共享工具函数
 *
 * 从 admin/KnowledgeList, admin/KnowledgeEdit 提取重复定义的状态显示函数。
 */

/** 文章审核状态 CSS 类名 */
export function articleStatusClass(status: number): string {
  const map: Record<number, string> = {
    0: 'disabled', 1: 'draft', 2: 'pending', 3: 'approved', 4: 'published', 5: 'rejected',
  }
  return map[status] || ''
}

/** 文章审核状态显示文本 */
export function articleStatusText(status: number): string {
  const map: Record<number, string> = {
    0: '已停用', 1: '草稿', 2: '待审核', 3: '已通过', 4: '已发布', 5: '已驳回',
  }
  return map[status] || '未知'
}

/** 文档处理状态 CSS 类名 */
export function processClass(status: number | undefined): string {
  if (status === undefined) return ''
  const map: Record<number, string> = { 1: 'pending', 2: 'pending', 3: 'pending', 4: 'completed', 5: 'failed' }
  return map[status] || ''
}

/** 文档处理状态显示文本 */
export function processText(status: number | undefined): string {
  if (status === undefined) return '-'
  const map: Record<number, string> = { 0: '待处理', 1: '解析中', 2: '分块中', 3: '向量化中', 4: '已完成', 5: '失败' }
  return map[status] || '-'
}
