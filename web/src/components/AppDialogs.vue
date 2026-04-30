<script setup lang="ts">
// 全局 confirm/alert 对话框渲染器。挂载在 App.vue 一次。
// 用法见 src/dialogs.ts。
import { onBeforeUnmount, onMounted } from 'vue'
import { dialogState, _resolveDialog } from '@/dialogs'

function onChoose(id: number, ok: boolean) {
  _resolveDialog(id, ok)
}

// ESC 关闭最顶层 dialog（视为"取消"）
function onKey(e: KeyboardEvent) {
  if (e.key !== 'Escape') return
  const top = dialogState.items[dialogState.items.length - 1]
  if (!top) return
  e.preventDefault()
  e.stopPropagation()
  _resolveDialog(top.id, false)
}

onMounted(() => window.addEventListener('keydown', onKey, true))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey, true))
</script>

<template>
  <Transition name="ad-fade">
    <div v-if="dialogState.items.length > 0" class="ad-stack" role="presentation">
      <div
        v-for="(d, i) in dialogState.items"
        :key="d.id"
        class="ad-backdrop"
        :class="{ 'is-top': i === dialogState.items.length - 1 }"
        :style="{ zIndex: 10000 + i }"
        @click.self="onChoose(d.id, false)"
      >
        <div class="ad-card" :class="{ 'is-danger': d.danger }">
          <!-- 头部：图标 + 标题 -->
          <div class="ad-head">
            <span class="ad-icon" :class="{ 'is-danger': d.danger }">
              <svg v-if="d.danger" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                   stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
                <path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/>
                <line x1="12" y1="9" x2="12" y2="13"/>
                <line x1="12" y1="17" x2="12.01" y2="17"/>
              </svg>
              <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor"
                   stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
                <circle cx="12" cy="12" r="10"/>
                <line x1="12" y1="16" x2="12" y2="12"/>
                <line x1="12" y1="8" x2="12.01" y2="8"/>
              </svg>
            </span>
            <div class="ad-text">
              <div v-if="d.title" class="ad-title">{{ d.title }}</div>
              <div class="ad-msg">{{ d.message }}</div>
            </div>
          </div>

          <!-- 操作按钮 -->
          <div class="ad-foot">
            <button
              v-if="d.kind === 'confirm'"
              class="ad-btn ad-btn-secondary"
              @click="onChoose(d.id, false)"
            >{{ d.cancelText }}</button>
            <button
              class="ad-btn ad-btn-primary"
              :class="{ 'is-danger': d.danger }"
              autofocus
              @click="onChoose(d.id, true)"
            >{{ d.confirmText }}</button>
          </div>
        </div>
      </div>
    </div>
  </Transition>
</template>

<style scoped>
/* —— 整个堆栈 —— */
.ad-stack {
  position: fixed; inset: 0;
  pointer-events: none;
  /* isolation: isolate 强制在这里建立独立的层叠上下文,避免父级的 transform /
     filter / opacity 等无意中"困住" backdrop,以致弹窗看起来不在最前。
     z-index 设为非常大的值,确保压过任何业务面板 (drawer 100/101 / modal 200 /
     popover 800 等)。 */
  isolation: isolate;
  z-index: 9999;
}
.ad-backdrop {
  position: fixed; inset: 0;
  pointer-events: auto;
  display: flex; align-items: center; justify-content: center;
  padding: 16px;
  background: rgba(15, 23, 42, 0.55);
  -webkit-backdrop-filter: blur(8px);
  backdrop-filter: blur(8px);
  animation: ad-bd 0.18s ease-out;
}
@keyframes ad-bd {
  from { opacity: 0; }
  to   { opacity: 1; }
}

/* —— 卡片 —— */
.ad-card {
  width: min(420px, 95vw);
  background: var(--tg-bg-elev);
  border: 1px solid var(--tg-divider);
  border-radius: var(--tg-radius-xl);
  box-shadow:
    0 24px 80px -16px rgba(15, 23, 42, 0.28),
    0 8px 24px -8px rgba(15, 23, 42, 0.12),
    0 0 0 1px rgba(99, 102, 241, 0.06) inset;
  overflow: hidden;
  animation: ad-pop 0.36s cubic-bezier(0.34, 1.56, 0.64, 1);
  position: relative;
}
.ad-card.is-danger {
  box-shadow:
    0 24px 80px -16px rgba(239, 68, 68, 0.30),
    0 8px 24px -8px rgba(15, 23, 42, 0.12),
    0 0 0 1px rgba(239, 68, 68, 0.10) inset;
}
@keyframes ad-pop {
  from { transform: translateY(14px) scale(0.94); opacity: 0; }
  to   { transform: translateY(0)    scale(1);    opacity: 1; }
}

/* —— 顶部高光带 —— */
.ad-card::before {
  content: '';
  position: absolute; left: 0; right: 0; top: 0;
  height: 3px;
  background: var(--tg-grad-brand);
}
.ad-card.is-danger::before {
  background: linear-gradient(90deg, #ef4444, #f97316, #ef4444);
}

/* —— Head —— */
.ad-head {
  display: flex; align-items: flex-start; gap: 16px;
  padding: 24px 24px 18px;
}
.ad-icon {
  flex-shrink: 0;
  width: 44px; height: 44px;
  display: inline-flex; align-items: center; justify-content: center;
  border-radius: 14px;
  background: color-mix(in srgb, var(--tg-primary) 14%, transparent);
  color: var(--tg-primary);
}
.ad-icon svg { width: 22px; height: 22px; }
.ad-icon.is-danger {
  background: color-mix(in srgb, var(--tg-danger) 14%, transparent);
  color: var(--tg-danger);
}

.ad-text { flex: 1; min-width: 0; padding-top: 2px; }
.ad-title {
  font-family: 'Sora', sans-serif;
  font-size: 17px; font-weight: 700;
  letter-spacing: -0.018em;
  color: var(--tg-text);
  margin-bottom: 6px;
  line-height: 1.3;
}
.ad-msg {
  font-size: 14px; line-height: 1.6;
  color: var(--tg-text-secondary);
  white-space: pre-wrap;
  word-wrap: break-word;
}

/* —— Foot —— */
.ad-foot {
  display: flex; gap: 10px; justify-content: flex-end;
  padding: 14px 22px 20px;
}
.ad-btn {
  appearance: none;
  padding: 9px 18px;
  font-family: 'Sora', sans-serif;
  font-size: 13.5px; font-weight: 700;
  letter-spacing: 0.01em;
  border-radius: var(--tg-radius-pill);
  cursor: pointer;
  transition: transform var(--tg-trans-fast),
              box-shadow var(--tg-trans-fast),
              background var(--tg-trans-fast),
              color var(--tg-trans-fast);
  min-width: 80px;
}
.ad-btn:active { transform: scale(0.97); }

.ad-btn-secondary {
  background: var(--tg-hover);
  color: var(--tg-text-secondary);
  border: 1.5px solid var(--tg-divider);
}
.ad-btn-secondary:hover {
  background: var(--tg-press);
  color: var(--tg-text);
  border-color: var(--tg-divider-strong);
}

.ad-btn-primary {
  background: var(--tg-grad-brand);
  color: var(--tg-on-primary);
  border: 1.5px solid transparent;
  box-shadow: 0 4px 12px -2px rgba(99, 102, 241, 0.36);
}
.ad-btn-primary:hover {
  transform: translateY(-1px);
  box-shadow: 0 8px 20px -4px rgba(99, 102, 241, 0.48);
}
.ad-btn-primary.is-danger {
  background: linear-gradient(135deg, #ef4444 0%, #dc2626 100%);
  box-shadow: 0 4px 12px -2px rgba(239, 68, 68, 0.36);
}
.ad-btn-primary.is-danger:hover {
  box-shadow: 0 8px 20px -4px rgba(239, 68, 68, 0.48);
}

/* —— 多层堆叠：底层弱化 —— */
.ad-backdrop:not(.is-top) .ad-card {
  filter: brightness(0.92) saturate(0.9);
  transform: scale(0.96) translateY(-6px);
}

/* —— Transition —— */
.ad-fade-enter-active, .ad-fade-leave-active {
  transition: opacity 0.18s ease;
}
.ad-fade-enter-from, .ad-fade-leave-to { opacity: 0; }

@media (max-width: 480px) {
  .ad-head { padding: 20px 18px 14px; gap: 12px; }
  .ad-icon { width: 38px; height: 38px; }
  .ad-icon svg { width: 19px; height: 19px; }
  .ad-title { font-size: 15.5px; }
  .ad-msg { font-size: 13.5px; }
  .ad-foot { padding: 12px 16px 16px; }
  .ad-btn { padding: 8px 14px; font-size: 13px; min-width: 70px; }
}
</style>
