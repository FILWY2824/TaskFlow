<script setup lang="ts">
// 番茄钟历史记录页 ——
// 把原本嵌在 Pomodoro 主页底部的"最近记录"独立成单独页面，
// 让主页只关注"当前正在专注 / 即将开始的这一段"，把回顾留给这里。

import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { pomodoro as pomoApi, ApiError } from '@/api'
import type { PomodoroSession } from '@/types'
import { fmtDateTime, fmtDuration } from '@/utils'

const router = useRouter()

const recent = ref<PomodoroSession[]>([])
const loading = ref(false)
const errMsg = ref('')

async function loadRecent() {
  loading.value = true
  errMsg.value = ''
  try {
    recent.value = await pomoApi.list({ limit: 200 })
  } catch (e) {
    errMsg.value = e instanceof ApiError ? e.message : (e as Error).message
  } finally {
    loading.value = false
  }
}

onMounted(loadRecent)

function statusText(s: PomodoroSession): string {
  switch (s.status) {
    case 'completed': return '完成'
    case 'abandoned': return '放弃'
    case 'active': return '进行中'
    default: return s.status
  }
}

function backToTimer() {
  router.push({ name: 'pomodoro' })
}
</script>

<template>
  <div class="pomo-history-wrap">
    <div class="ph-head">
      <button class="btn-secondary ph-back-btn" @click="backToTimer">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor"
             stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="15 18 9 12 15 6"/>
        </svg>
        返回番茄钟
      </button>
      <div class="ph-title">
        <span class="ph-title-icon">🍅</span>
        番茄钟历史
      </div>
      <button class="btn-ghost" :disabled="loading" @click="loadRecent">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor"
             stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="23 4 23 10 17 10"/>
          <path d="M20.49 15a9 9 0 1 1-2.13-9.36L23 10"/>
        </svg>
        刷新
      </button>
    </div>

    <Transition name="fade">
      <div v-if="errMsg" class="auth-error">{{ errMsg }}</div>
    </Transition>

    <div v-if="loading" class="muted" style="text-align:center;padding:32px 0">加载中…</div>

    <div v-else-if="recent.length > 0" class="ph-list">
      <div v-for="s in recent" :key="s.id" class="ph-item">
        <div class="ph-icon">
          <span v-if="s.kind === 'focus'">🎯</span>
          <span v-else-if="s.kind === 'short_break'">☕</span>
          <span v-else>🛌</span>
        </div>
        <div class="ph-info">
          <div class="ph-item-title">
            {{ s.kind === 'focus' ? '专注' : (s.kind === 'short_break' ? '短休' : '长休') }}
            <span v-if="s.note" class="muted"> — {{ s.note }}</span>
          </div>
          <div class="ph-item-time">{{ fmtDateTime(s.started_at) }}</div>
        </div>
        <div class="ph-status-wrap">
          <span class="ph-status-badge" :class="s.status">{{ statusText(s) }}</span>
          <span class="ph-duration">{{ fmtDuration(s.actual_duration_seconds || s.planned_duration_seconds) }}</span>
        </div>
      </div>
    </div>

    <div v-else class="empty">
      <div class="empty-icon">🍅</div>
      <div class="empty-title">还没有番茄记录</div>
      <div class="empty-hint">回到番茄钟主页开始你的第一次专注</div>
    </div>
  </div>
</template>

<style scoped>
.pomo-history-wrap { max-width: 720px; margin: 0 auto; }

.ph-head {
  display: flex; align-items: center; gap: 12px;
  margin-bottom: 18px;
  padding-bottom: 14px;
  border-bottom: 1px solid var(--tg-divider);
}
.ph-back-btn {
  padding: 7px 14px;
  font-size: 13px;
  border-radius: 999px;
}
.ph-title {
  flex: 1;
  display: inline-flex; align-items: center; gap: 8px;
  font-family: 'Sora', sans-serif;
  font-size: 18px; font-weight: 800;
  color: var(--tg-primary);
  letter-spacing: -0.018em;
}
.ph-title-icon { font-size: 22px; }

.ph-list { display: flex; flex-direction: column; gap: 6px; }
.ph-item {
  display: flex; align-items: center; gap: 14px;
  padding: 14px;
  background: var(--tg-bg-elev);
  border: 1px solid var(--tg-divider);
  border-radius: var(--tg-radius-md);
  transition: background 0.15s, border-color 0.15s, transform 0.15s;
}
.ph-item:hover {
  border-color: color-mix(in srgb, var(--tg-primary) 35%, var(--tg-divider));
  transform: translateY(-1px);
}
.ph-icon {
  font-size: 22px;
  background: var(--tg-hover);
  width: 44px; height: 44px;
  border-radius: 50%;
  display: flex; align-items: center; justify-content: center;
  flex-shrink: 0;
}
.ph-info { flex: 1; overflow: hidden; min-width: 0; }
.ph-item-title {
  font-size: 14.5px; font-weight: 700; margin-bottom: 3px;
  color: var(--tg-text);
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.ph-item-time { font-size: 12.5px; color: var(--tg-text-secondary); }
.ph-status-wrap {
  display: flex; flex-direction: column; align-items: flex-end;
  gap: 4px; flex-shrink: 0;
}
.ph-status-badge {
  font-size: 11px; font-weight: 700;
  padding: 2px 10px; border-radius: 999px;
  letter-spacing: 0.02em;
}
.ph-status-badge.completed { background: var(--tg-success-soft); color: var(--tg-success); }
.ph-status-badge.abandoned { background: var(--tg-danger-soft); color: var(--tg-danger); }
.ph-status-badge.active { background: var(--tg-primary-soft); color: var(--tg-primary); }
.ph-duration {
  font-size: 13px; font-weight: 700;
  font-variant-numeric: tabular-nums;
  color: var(--tg-primary);
}
</style>
