<script setup lang="ts">
// 仅时间选择器（不带日期）。用于"在某一天里新建任务"时只挑时间。
//
// v-model 绑定 'HH:mm' 字符串；'' 表示未选择。
// 风格与 PrettyDateTimePicker 完全一致，但只渲染时间区。

import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = defineProps<{
  modelValue: string
  placeholder?: string
  disabled?: boolean
  /** 默认时间（HH:mm），用户清空后再次打开时回填 */
  defaultTime?: string
}>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: string): void
}>()

const open = ref(false)
const triggerRef = ref<HTMLElement | null>(null)
const popRef = ref<HTMLElement | null>(null)

const selectedHour = ref(9)
const selectedMinute = ref(0)

const hours = Array.from({ length: 24 }, (_, i) => i)
const minutes = Array.from({ length: 12 }, (_, i) => i * 5)
const ITEM_H = 36

const hourListRef = ref<HTMLElement | null>(null)
const minuteListRef = ref<HTMLElement | null>(null)

watch(
  () => props.modelValue,
  (v) => {
    const m = v && v.match(/^(\d{1,2}):(\d{2})$/)
    if (m) {
      selectedHour.value = Number(m[1])
      // 对齐到 5 分钟
      const mm = Number(m[2])
      selectedMinute.value = Math.round(mm / 5) * 5
      if (selectedMinute.value === 60) selectedMinute.value = 0
    }
  },
  { immediate: true },
)

function commit() {
  const hh = String(selectedHour.value).padStart(2, '0')
  const mm = String(selectedMinute.value).padStart(2, '0')
  emit('update:modelValue', `${hh}:${mm}`)
}

const triggerLabel = computed(() => {
  if (!props.modelValue) return props.placeholder || '选择时间'
  return props.modelValue
})
const isEmpty = computed(() => !props.modelValue)

function pickHour(h: number) {
  selectedHour.value = h
  commit()
}
function pickMinute(m: number) {
  selectedMinute.value = m
  commit()
}

function shiftMinutes(delta: number) {
  let total = selectedHour.value * 60 + selectedMinute.value + delta
  total = ((total % 1440) + 1440) % 1440
  selectedHour.value = Math.floor(total / 60)
  selectedMinute.value = Math.round((total - selectedHour.value * 60) / 5) * 5
  if (selectedMinute.value === 60) {
    selectedMinute.value = 0
    selectedHour.value = (selectedHour.value + 1) % 24
  }
  commit()
  scrollWheelsToSelected()
}

function scrollWheelsToSelected() {
  nextTick(() => {
    if (hourListRef.value) {
      hourListRef.value.scrollTo({ top: selectedHour.value * ITEM_H, behavior: 'smooth' })
    }
    if (minuteListRef.value) {
      const idx = Math.round(selectedMinute.value / 5)
      minuteListRef.value.scrollTo({ top: idx * ITEM_H, behavior: 'smooth' })
    }
  })
}

function quickPick(h: number, m: number) {
  selectedHour.value = h
  selectedMinute.value = m
  commit()
  scrollWheelsToSelected()
}
function clearAll() {
  emit('update:modelValue', '')
}

function toggleOpen() {
  if (props.disabled) return
  open.value = !open.value
  if (open.value) {
    // 打开时若空则回填默认
    if (!props.modelValue && props.defaultTime) {
      emit('update:modelValue', props.defaultTime)
    }
    nextTick(() => scrollWheelsToSelected())
  }
}
function close() { open.value = false }

function onDocClick(e: MouseEvent) {
  if (!open.value) return
  const t = e.target as Node
  if (
    triggerRef.value && !triggerRef.value.contains(t) &&
    popRef.value && !popRef.value.contains(t)
  ) close()
}
function onKey(e: KeyboardEvent) {
  if (open.value && e.key === 'Escape') {
    e.preventDefault()
    close()
  }
}
onMounted(() => {
  document.addEventListener('mousedown', onDocClick)
  document.addEventListener('keydown', onKey)
})
onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onDocClick)
  document.removeEventListener('keydown', onKey)
})
</script>

<template>
  <div class="pdt" :class="{ 'is-disabled': disabled }">
    <button
      ref="triggerRef"
      type="button"
      class="pdt-trigger"
      :class="{ 'is-empty': isEmpty, 'is-open': open }"
      :disabled="disabled"
      @click="toggleOpen"
    >
      <span class="pdt-trigger-icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor"
             stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="12" cy="12" r="10"/>
          <polyline points="12 6 12 12 16 14"/>
        </svg>
      </span>
      <span class="pdt-trigger-label">{{ triggerLabel }}</span>
      <span class="pdt-trigger-caret">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor"
             stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="6 9 12 15 18 9"/>
        </svg>
      </span>
    </button>

    <Transition name="pdt-pop">
      <div v-if="open" ref="popRef" class="pdt-pop">
        <!-- 快捷预设 -->
        <div class="pdt-presets">
          <button class="pdt-preset" @click="quickPick(9, 0)">
            <span class="pdt-preset-icon">🌅</span>09:00
          </button>
          <button class="pdt-preset" @click="quickPick(12, 0)">
            <span class="pdt-preset-icon">🍱</span>12:00
          </button>
          <button class="pdt-preset" @click="quickPick(18, 0)">
            <span class="pdt-preset-icon">🌆</span>18:00
          </button>
          <button class="pdt-preset" @click="quickPick(23, 59)">
            <span class="pdt-preset-icon">🌙</span>23:59
          </button>
        </div>

        <div class="pdt-time">
          <div class="pdt-time-head">
            <span class="pdt-time-title">
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                   stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
                <circle cx="12" cy="12" r="10"/>
                <polyline points="12 6 12 12 16 14"/>
              </svg>
              选择时间
            </span>
            <div class="pdt-time-quick">
              <button type="button" @click="shiftMinutes(-60)">-1h</button>
              <button type="button" @click="shiftMinutes(-15)">-15m</button>
              <button type="button" @click="shiftMinutes(15)">+15m</button>
              <button type="button" @click="shiftMinutes(60)">+1h</button>
            </div>
          </div>

          <div class="pdt-wheels">
            <div class="pdt-wheel">
              <div class="pdt-wheel-band" />
              <div ref="hourListRef" class="pdt-wheel-list">
                <div class="pdt-wheel-pad" />
                <div
                  v-for="h in hours"
                  :key="`h-${h}`"
                  class="pdt-wheel-item"
                  :class="{ 'is-selected': h === selectedHour }"
                  @click="pickHour(h)"
                >{{ String(h).padStart(2, '0') }}</div>
                <div class="pdt-wheel-pad" />
              </div>
            </div>
            <span class="pdt-wheel-sep">:</span>
            <div class="pdt-wheel">
              <div class="pdt-wheel-band" />
              <div ref="minuteListRef" class="pdt-wheel-list">
                <div class="pdt-wheel-pad" />
                <div
                  v-for="m in minutes"
                  :key="`m-${m}`"
                  class="pdt-wheel-item"
                  :class="{ 'is-selected': m === selectedMinute }"
                  @click="pickMinute(m)"
                >{{ String(m).padStart(2, '0') }}</div>
                <div class="pdt-wheel-pad" />
              </div>
            </div>
          </div>
        </div>

        <div class="pdt-foot">
          <button
            type="button"
            class="pdt-foot-btn pdt-foot-clear"
            :disabled="isEmpty"
            @click="clearAll"
          >
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                 stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
              <line x1="18" y1="6" x2="6" y2="18"/>
              <line x1="6" y1="6" x2="18" y2="18"/>
            </svg>
            清空
          </button>
          <span class="pdt-foot-spacer" />
          <button type="button" class="pdt-foot-btn pdt-foot-done" @click="close">完成</button>
        </div>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.pdt {
  position: relative;
  width: 100%;
}
.pdt.is-disabled { opacity: 0.55; pointer-events: none; }

.pdt-trigger {
  width: 100%;
  display: flex; align-items: center; gap: 12px;
  padding: 13px 14px;
  background:
    linear-gradient(var(--tg-bg-elev), var(--tg-bg-elev)) padding-box,
    linear-gradient(135deg, var(--tg-divider), var(--tg-divider)) border-box;
  border: 1.5px solid transparent;
  border-radius: var(--tg-radius-md);
  color: var(--tg-text);
  font-family: inherit;
  font-size: 14.5px; font-weight: 600;
  text-align: left;
  cursor: pointer;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.04);
  transition: transform var(--tg-trans-fast),
              box-shadow var(--tg-trans-fast),
              background var(--tg-trans-fast);
  font-variant-numeric: tabular-nums;
}
.pdt-trigger:hover {
  background:
    linear-gradient(color-mix(in srgb, var(--tg-primary) 2%, var(--tg-bg-elev)),
                    color-mix(in srgb, var(--tg-primary) 2%, var(--tg-bg-elev))) padding-box,
    linear-gradient(135deg,
      color-mix(in srgb, var(--tg-primary) 30%, var(--tg-divider-strong)),
      color-mix(in srgb, var(--tg-accent) 30%, var(--tg-divider-strong))) border-box;
  transform: translateY(-1px);
}
.pdt-trigger.is-open {
  background:
    linear-gradient(var(--tg-bg-elev), var(--tg-bg-elev)) padding-box,
    var(--tg-grad-brand) border-box;
  outline: none;
  box-shadow:
    0 0 0 4px color-mix(in srgb, var(--tg-primary) 14%, transparent),
    0 6px 18px -6px color-mix(in srgb, var(--tg-primary) 38%, transparent);
}
.pdt-trigger.is-empty .pdt-trigger-label {
  color: var(--tg-text-tertiary);
  font-weight: 500;
}
.pdt-trigger-icon {
  display: inline-flex; align-items: center; justify-content: center;
  flex-shrink: 0; width: 22px; height: 22px;
  color: var(--tg-primary);
  transition: transform var(--tg-trans-fast);
}
.pdt-trigger-icon svg { width: 18px; height: 18px; }
.pdt-trigger:hover .pdt-trigger-icon,
.pdt-trigger.is-open .pdt-trigger-icon { transform: scale(1.1); }
.pdt-trigger-label {
  flex: 1; min-width: 0;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.pdt-trigger-caret {
  display: inline-flex; align-items: center; justify-content: center;
  width: 18px; height: 18px;
  color: var(--tg-text-tertiary);
  transition: transform var(--tg-trans-fast), color var(--tg-trans-fast);
}
.pdt-trigger-caret svg { width: 16px; height: 16px; }
.pdt-trigger.is-open .pdt-trigger-caret { transform: rotate(180deg); color: var(--tg-primary); }

.pdt-pop {
  position: absolute;
  top: calc(100% + 8px);
  left: 0;
  width: 280px;
  max-width: calc(100vw - 32px);
  z-index: 800;
  background: var(--tg-bg-elev);
  border: 1px solid var(--tg-divider);
  border-radius: var(--tg-radius-lg);
  box-shadow:
    0 28px 80px -20px rgba(15, 23, 42, 0.30),
    0 12px 36px -8px rgba(15, 23, 42, 0.14),
    0 0 0 1px rgba(99, 102, 241, 0.06) inset;
  overflow: hidden;
}

.pdt-presets {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 6px;
  padding: 12px 12px 0;
}
.pdt-preset {
  display: flex; flex-direction: column; align-items: center; gap: 3px;
  padding: 8px 4px;
  background: var(--tg-hover);
  border: 1px solid var(--tg-divider);
  border-radius: var(--tg-radius-sm);
  font-family: inherit;
  font-size: 11.5px; font-weight: 700;
  color: var(--tg-text-secondary);
  cursor: pointer;
  transition: all var(--tg-trans-fast);
  font-variant-numeric: tabular-nums;
}
.pdt-preset:hover {
  background: color-mix(in srgb, var(--tg-primary) 10%, transparent);
  border-color: color-mix(in srgb, var(--tg-primary) 35%, transparent);
  color: var(--tg-primary);
  transform: translateY(-1px);
}
.pdt-preset-icon { font-size: 16px; line-height: 1; }

.pdt-time {
  padding: 12px 12px 6px;
  border-top: 1px solid var(--tg-divider);
}
.pdt-time-head {
  display: flex; align-items: center; justify-content: space-between;
  margin-bottom: 8px;
}
.pdt-time-title {
  display: inline-flex; align-items: center; gap: 5px;
  font-family: 'Sora', sans-serif;
  font-size: 11px; font-weight: 700;
  color: var(--tg-text-secondary);
  letter-spacing: 0.06em;
  text-transform: uppercase;
}
.pdt-time-title svg { color: var(--tg-primary); }
.pdt-time-quick { display: flex; gap: 3px; }
.pdt-time-quick button {
  appearance: none;
  padding: 3px 7px;
  background: var(--tg-hover);
  border: 1px solid var(--tg-divider);
  border-radius: var(--tg-radius-pill);
  font-family: 'Manrope', sans-serif;
  font-size: 10.5px; font-weight: 700;
  color: var(--tg-text-secondary);
  cursor: pointer;
  transition: all var(--tg-trans-fast);
  font-variant-numeric: tabular-nums;
}
.pdt-time-quick button:hover {
  background: color-mix(in srgb, var(--tg-primary) 12%, transparent);
  color: var(--tg-primary);
  border-color: color-mix(in srgb, var(--tg-primary) 35%, transparent);
}

.pdt-wheels {
  display: flex; align-items: center; justify-content: center;
  gap: 12px;
  padding: 4px 0 0;
}
.pdt-wheel {
  position: relative;
  width: 64px; height: 144px;
  border-radius: var(--tg-radius-md);
  background: var(--tg-hover);
  overflow: hidden;
}
.pdt-wheel-band {
  position: absolute;
  left: 6px; right: 6px;
  top: 50%; transform: translateY(-50%);
  height: 36px;
  border-radius: 10px;
  background:
    linear-gradient(135deg,
      color-mix(in srgb, var(--tg-primary) 18%, transparent),
      color-mix(in srgb, var(--tg-accent) 18%, transparent));
  box-shadow:
    inset 0 0 0 1.5px color-mix(in srgb, var(--tg-primary) 35%, transparent);
  pointer-events: none;
  z-index: 1;
}
.pdt-wheel-list {
  position: relative; z-index: 2;
  height: 100%;
  overflow-y: auto;
  scroll-snap-type: y mandatory;
  -webkit-overflow-scrolling: touch;
  scrollbar-width: none;
  mask-image: linear-gradient(180deg,
    transparent 0%, #000 22%, #000 78%, transparent 100%);
  -webkit-mask-image: linear-gradient(180deg,
    transparent 0%, #000 22%, #000 78%, transparent 100%);
}
.pdt-wheel-list::-webkit-scrollbar { display: none; }
.pdt-wheel-pad { height: 54px; }
.pdt-wheel-item {
  height: 36px;
  display: flex; align-items: center; justify-content: center;
  font-family: 'Sora', sans-serif;
  font-size: 16px; font-weight: 700;
  font-variant-numeric: tabular-nums;
  color: var(--tg-text-secondary);
  cursor: pointer;
  scroll-snap-align: center;
  user-select: none;
  transition: color var(--tg-trans-fast), transform var(--tg-trans-fast);
}
.pdt-wheel-item:hover { color: var(--tg-primary); }
.pdt-wheel-item.is-selected {
  color: var(--tg-primary);
  font-size: 18px;
  font-weight: 800;
  transform: scale(1.05);
}
.pdt-wheel-sep {
  font-family: 'Sora', sans-serif;
  font-size: 28px; font-weight: 800;
  color: var(--tg-primary);
  margin-bottom: 4px;
}

.pdt-foot {
  display: flex; align-items: center;
  padding: 10px 12px 12px;
  border-top: 1px solid var(--tg-divider);
  margin-top: 6px;
  background: color-mix(in srgb, var(--tg-primary) 2%, transparent);
}
.pdt-foot-spacer { flex: 1; }
.pdt-foot-btn {
  appearance: none;
  display: inline-flex; align-items: center; gap: 5px;
  padding: 7px 14px;
  border: none;
  border-radius: var(--tg-radius-pill);
  font-family: 'Sora', sans-serif;
  font-size: 12.5px; font-weight: 700;
  cursor: pointer;
  transition: all var(--tg-trans-fast);
}
.pdt-foot-clear { background: transparent; color: var(--tg-text-tertiary); }
.pdt-foot-clear:hover:not(:disabled) {
  background: color-mix(in srgb, var(--tg-danger) 10%, transparent);
  color: var(--tg-danger);
}
.pdt-foot-clear:disabled { opacity: 0.4; cursor: not-allowed; }
.pdt-foot-done {
  background: var(--tg-grad-brand);
  color: var(--tg-on-primary);
  box-shadow: 0 4px 12px -3px rgba(99, 102, 241, 0.4);
}
.pdt-foot-done:hover {
  transform: translateY(-1px);
  box-shadow: 0 6px 16px -3px rgba(99, 102, 241, 0.5);
}

.pdt-pop-enter-active {
  transition: opacity var(--tg-trans-fast), transform var(--tg-trans);
  transform-origin: top left;
}
.pdt-pop-leave-active {
  transition: opacity var(--tg-trans-fast), transform var(--tg-trans-fast);
  transform-origin: top left;
}
.pdt-pop-enter-from, .pdt-pop-leave-to {
  opacity: 0;
  transform: translateY(-8px) scale(0.97);
}

@media (max-width: 480px) {
  .pdt-pop {
    position: fixed;
    top: auto;
    bottom: 0; left: 0; right: 0;
    width: auto;
    max-width: 100vw;
    border-radius: var(--tg-radius-xl) var(--tg-radius-xl) 0 0;
    border-bottom: none;
    box-shadow:
      0 -16px 60px -8px rgba(15, 23, 42, 0.30),
      0 -8px 20px -4px rgba(15, 23, 42, 0.14);
  }
  .pdt-pop::before {
    content: '';
    position: absolute;
    top: 8px; left: 50%; transform: translateX(-50%);
    width: 38px; height: 4px;
    border-radius: 999px;
    background: var(--tg-divider-strong);
  }
  .pdt-presets { padding-top: 24px; }
}
</style>
