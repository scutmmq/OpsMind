<template>
  <div class="dashboard-page">
    <div class="page-header">
      <h1 class="page-title">数据看板</h1>
      <span class="page-subtitle">运维运营概览</span>
    </div>

    <!-- 加载中 -->
    <div v-if="loading" class="loading-state">
      <p>加载中...</p>
    </div>

    <!-- 统计数据卡片 -->
    <div v-else class="stats-grid">
      <div class="stat-card">
        <div class="stat-value">{{ stats.today_tickets }}</div>
        <div class="stat-label">今日申告</div>
      </div>
      <div class="stat-card stat-card--warn">
        <div class="stat-value">{{ stats.pending_tickets }}</div>
        <div class="stat-label">待处理</div>
      </div>
      <div class="stat-card stat-card--info">
        <div class="stat-value">{{ stats.processing_tickets }}</div>
        <div class="stat-label">处理中</div>
      </div>
      <div class="stat-card stat-card--success">
        <div class="stat-value">{{ stats.resolved_tickets }}</div>
        <div class="stat-label">已解决</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{{ stats.today_chats }}</div>
        <div class="stat-label">今日问答</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{{ (stats.avg_confidence * 100).toFixed(0) }}%</div>
        <div class="stat-label">平均置信度</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{{ stats.knowledge_count }}</div>
        <div class="stat-label">知识条目数</div>
      </div>
    </div>

    <!-- 加载错误 -->
    <div v-if="error" class="error-state">
      <p>加载数据失败：{{ error }}</p>
      <button class="btn-retry" @click="fetchStats">重试</button>
    </div>

    <!-- 趋势图区域 -->
    <div class="trends-section">
      <div class="trends-header">
        <h2 class="section-title">趋势数据</h2>
        <div class="trends-filters">
          <input
            type="date"
            v-model="trendStart"
            class="date-input"
            @change="fetchTrends"
          />
          <span class="date-separator">至</span>
          <input
            type="date"
            v-model="trendEnd"
            class="date-input"
            @change="fetchTrends"
          />
        </div>
      </div>

      <!-- 趋势加载中 -->
      <div v-if="trendsLoading" class="loading-state">
        <p>加载趋势数据中...</p>
      </div>

      <!-- 趋势为空 -->
      <div v-else-if="trendPoints.length === 0" class="empty-state">
        <p>暂无趋势数据</p>
      </div>

      <!-- 趋势图表 -->
      <div v-else class="trend-chart">
        <div class="chart-bars">
          <div
            v-for="(point, i) in trendPoints"
            :key="i"
            class="chart-column"
          >
            <div class="bar-group">
              <div
                class="bar bar--ticket"
                :style="{ height: barHeight(point.ticket_count, maxTrendValue) + 'px' }"
                :title="`申告: ${point.ticket_count}`"
              ></div>
              <div
                class="bar bar--chat"
                :style="{ height: barHeight(point.chat_count, maxTrendValue) + 'px' }"
                :title="`问答: ${point.chat_count}`"
              ></div>
            </div>
            <div class="chart-label">{{ formatDate(point.date) }}</div>
          </div>
        </div>
        <div class="chart-legend">
          <span class="legend-item"><span class="legend-dot legend-dot--ticket"></span> 申告</span>
          <span class="legend-item"><span class="legend-dot legend-dot--chat"></span> 问答</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { getStats, getTrends, type StatsData, type TrendDataPoint } from '@/api/dashboard'

const loading = ref(true)
const error = ref('')
const stats = ref<StatsData>({
  today_tickets: 0, pending_tickets: 0, processing_tickets: 0,
  resolved_tickets: 0, today_chats: 0, avg_confidence: 0, knowledge_count: 0
})

// 趋势数据
const trendsLoading = ref(false)
const trendPoints = ref<TrendDataPoint[]>([])
const trendStart = ref('')
const trendEnd = ref('')

onMounted(() => {
  const d = new Date()
  trendEnd.value = d.toISOString().slice(0, 10)
  d.setDate(d.getDate() - 6)
  trendStart.value = d.toISOString().slice(0, 10)
  fetchStats()
  fetchTrends()
})

// 趋势最大值（用于计算柱状图高度）
const maxTrendValue = computed(() => {
  let max = 1
  for (const p of trendPoints.value) {
    max = Math.max(max, p.ticket_count, p.chat_count)
  }
  return max
})

async function fetchStats() {
  loading.value = true
  error.value = ''
  try {
    const res = await getStats()
    const data = (res as any).data || res
    if (data) stats.value = data
  } catch (e: any) {
    error.value = e?.response?.data?.message || e?.message || '网络错误'
  } finally {
    loading.value = false
  }
}

async function fetchTrends() {
  trendsLoading.value = true
  try {
    const res = await getTrends({
      start_date: trendStart.value,
      end_date: trendEnd.value,
      granularity: 'day'
    })
    const data = (res as any).data || res
    trendPoints.value = data?.data_points || []
  } catch {
    trendPoints.value = []
  } finally {
    trendsLoading.value = false
  }
}

// 简单柱状图高度计算（最大高度 120px）
function barHeight(count: number, max: number): number {
  if (max <= 0) return 0
  return Math.max(4, Math.round((count / max) * 120))
}

function formatDate(dateStr: string): string {
  const parts = dateStr.split('-')
  if (parts.length === 3) return `${parts[1]}/${parts[2]}`
  return dateStr
}
</script>

<style scoped>
.dashboard-page {
  max-width: 1100px;
}

.page-header {
  margin-bottom: 28px;
}

.page-title {
  font-size: 22px;
  font-weight: 510;
  color: var(--text-primary);
}

.page-subtitle {
  font-size: 13px;
  color: var(--text-secondary);
  margin-left: 10px;
}

/* 统计卡片网格 */
.stats-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 14px;
  margin-bottom: 32px;
}

.stat-card {
  padding: 20px;
  background: var(--bg-overlay);
  border: 1px solid var(--border-default);
  border-radius: 10px;
  text-align: center;
}

.stat-value {
  font-size: 28px;
  font-weight: 510;
  color: var(--text-primary);
  line-height: 1.2;
}

.stat-label {
  font-size: 12px;
  color: var(--text-secondary);
  margin-top: 6px;
}

.stat-card--warn .stat-value { color: #f59e0b; }
.stat-card--info .stat-value { color: #3b82f6; }
.stat-card--success .stat-value { color: #22c55e; }

/* 加载/错误/空状态 */
.loading-state, .error-state, .empty-state {
  text-align: center;
  padding: 48px 24px;
  color: var(--text-secondary);
  font-size: 14px;
}

.error-state { color: #f87171; }

.btn-retry {
  margin-top: 12px;
  padding: 6px 18px;
  background: var(--accent);
  color: #fff;
  border: none;
  border-radius: 6px;
  font-size: 13px;
  cursor: pointer;
  font-family: inherit;
}

/* 趋势区域 */
.trends-section {
  background: var(--bg-overlay);
  border: 1px solid var(--border-default);
  border-radius: 10px;
  padding: 24px;
}

.trends-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.section-title {
  font-size: 16px;
  font-weight: 510;
  color: var(--text-primary);
}

.trends-filters {
  display: flex;
  align-items: center;
  gap: 8px;
}

.date-input {
  padding: 5px 10px;
  background: var(--bg-base);
  border: 1px solid var(--border-default);
  border-radius: 6px;
  color: var(--text-primary);
  font-size: 13px;
  font-family: inherit;
}

.date-separator {
  color: var(--text-secondary);
  font-size: 13px;
}

/* 柱状图 */
.trend-chart {
  padding-top: 8px;
}

.chart-bars {
  display: flex;
  align-items: flex-end;
  gap: 6px;
  height: 140px;
  padding: 4px 0;
  border-bottom: 1px solid var(--border-default);
}

.chart-column {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
}

.bar-group {
  display: flex;
  align-items: flex-end;
  gap: 3px;
  height: 120px;
}

.bar {
  width: 12px;
  border-radius: 3px 3px 0 0;
  transition: height 0.3s ease;
  min-height: 4px;
}

.bar--ticket { background: var(--accent); }
.bar--chat { background: rgba(94, 106, 210, 0.35); }

.chart-label {
  font-size: 10px;
  color: var(--text-secondary);
  margin-top: 6px;
  white-space: nowrap;
}

.chart-legend {
  display: flex;
  gap: 16px;
  padding-top: 12px;
  font-size: 12px;
  color: var(--text-secondary);
}

.legend-item {
  display: flex;
  align-items: center;
  gap: 5px;
}

.legend-dot {
  width: 10px;
  height: 10px;
  border-radius: 2px;
}

.legend-dot--ticket { background: var(--accent); }
.legend-dot--chat { background: rgba(94, 106, 210, 0.35); }
</style>
