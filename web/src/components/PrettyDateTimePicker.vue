<script setup lang="ts">
// 漂亮的、丝滑的日期 + 时间选择器（替代原生 <input type="datetime-local">）。
//
// 设计要点:
//   - v-model 绑定的是 "YYYY-MM-DDTHH:mm" 形式的本地字符串（与原生控件兼容），
//     这样在父组件里完全 drop-in 替换 datetime-local 即可。
//   - 上半区是日历月视图：左右切月、点击切换日期；今天/已选有强烈视觉反馈；
//     非当月天淡出但仍可点。
//   - 下半区是时间：两个滑动滚轮（小时/分钟，每5分钟一档），加 "+/-1h" 等快捷键。
//   - 一排"快捷预设"：今天/明天/后天 9:00、本周末 18:00、清空 等。
//   - 整个面板用 popover 形式从触发按钮下方弹出；移动端居中显示。
//   - 触发按钮自身就显示当前选中的"日期 时间"（或"未设置"），点击展开 popover。
//
// 与父组件的契约：
//   <PrettyDateTimePicker v-model="addDueLocal" :allow-clear="true" />
//   addDueLocal: string，'' 表示未设置；非空时形如 '2026-04-30T17:30'。

import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = defineProps<{
  modelValue: string
  /** 是否允许"清空" —— 在"日程"视图为必填时设为 false */
  allowClear?: boolean
  /** placeholder 文案（按钮显示）—— 默认 "选择日期与时间" */
  placeholder?: string
  /** 是否禁用整个组件 */
  disabled?: boolean
}>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: string): void
}>()

// ---------- 内部状态 ----------
const open = ref(false)
const triggerRef = ref<HTMLElement | null>(null)
const popRef = ref<HTMLElement | null>(null)

// ---------- 弹层定位 ----------
// 早期实现里, 我们曾用 "锚定到触发按钮 + JS 计算 fixed 坐标" 的方式让
// 弹层在按钮下方展开. 但当触发按钮位于一个居中的 modal 表单底部时,
// 锚定在按钮左下角的弹层会明显地"偏左 / 偏下", 与表单本身不对称, 观感很怪.
//
// 现在改成: 在 <body> 下铺一层全屏 overlay (.pdt-overlay), 用 flex 把
// 弹层 (.pdt-pop) 在视口中居中. 这样:
//   1. 弹层始终在视口正中, 与背后的居中表单天然对齐, 不再有"偏一边"的违和感.
//   2. overlay 自带 backdrop-filter: blur, 把背后的表单虚化 / 压暗,
//      把用户注意力聚焦到时间盘上.
//   3. overlay 自己捕获 mousedown -- 因为它通过 Teleport 挂到 <body> 下,
//      与表单的 modal-backdrop 是 DOM 兄弟节点, 事件不会冒泡到 modal-backdrop
//      的 @click.self 上. 所以"点击表盘外区域"只会关闭表盘, 不会关闭表单.

// 当前正在浏览的"月"，与 selected 解耦：用户可能在浏览 5 月但 selected 是 3 月某天
const browseDate = ref(new Date())
// 日历"已选"日期；和时间分开存
const selectedDate = ref<Date | null>(null)
// 时间分开记，避免日历切换时把时间也清掉
const selectedHour = ref<number>(9)
const selectedMinute = ref<number>(0)

// ---------- 与 modelValue 同步 ----------
function parseModel(v: string): Date | null {
  if (!v) return null
  // 期望格式：YYYY-MM-DDTHH:mm
  const m = v.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/)
  if (!m) {
    // 兼容带秒的情况
    const d = new Date(v)
    return isNaN(d.getTime()) ? null : d
  }
  return new Date(
    Number(m[1]), Number(m[2]) - 1, Number(m[3]),
    Number(m[4]), Number(m[5]), 0, 0,
  )
}
function formatModel(d: Date): string {
  const y = d.getFullYear()
  const mo = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  const h = String(d.getHours()).padStart(2, '0')
  const mi = String(d.getMinutes()).padStart(2, '0')
  return `${y}-${mo}-${dd}T${h}:${mi}`
}

watch(
  () => props.modelValue,
  (v) => {
    const d = parseModel(v)
    if (d) {
      selectedDate.value = new Date(d.getFullYear(), d.getMonth(), d.getDate())
      selectedHour.value = d.getHours()
      selectedMinute.value = d.getMinutes()
      browseDate.value = new Date(d.getFullYear(), d.getMonth(), 1)
    } else {
      selectedDate.value = null
      // 不清空时间，让用户重新选日期时维持上次时间偏好
    }
  },
  { immediate: true },
)

function commit() {
  if (!selectedDate.value) {
    emit('update:modelValue', '')
    return
  }
  const d = new Date(selectedDate.value)
  d.setHours(selectedHour.value, selectedMinute.value, 0, 0)
  emit('update:modelValue', formatModel(d))
}

// ---------- 显示文案 ----------
const dows = ['一', '二', '三', '四', '五', '六', '日']
const PRIO_THEME = '' // 为以后扩展留位

const triggerLabel = computed(() => {
  if (!props.modelValue) {
    return props.placeholder || '选择日期与时间'
  }
  const d = parseModel(props.modelValue)
  if (!d) return props.placeholder || '选择日期与时间'
  const today = new Date()
  const tomorrow = new Date(today); tomorrow.setDate(today.getDate() + 1)
  const dayKey = (x: Date) =>
    `${x.getFullYear()}-${x.getMonth()}-${x.getDate()}`
  const prefix =
    dayKey(d) === dayKey(today)    ? '今天' :
    dayKey(d) === dayKey(tomorrow) ? '明天' :
    `${d.getMonth() + 1} 月 ${d.getDate()} 日 · 周${dows[(d.getDay() + 6) % 7]}`
  const hh = String(d.getHours()).padStart(2, '0')
  const mm = String(d.getMinutes()).padStart(2, '0')
  return `${prefix}  ${hh}:${mm}`
})

const isEmpty = computed(() => !props.modelValue)

// ---------- 月视图 ----------
const monthLabel = computed(() => {
  const d = browseDate.value
  return `${d.getFullYear()} 年 ${d.getMonth() + 1} 月`
})

const cells = computed<Date[]>(() => {
  const first = new Date(browseDate.value.getFullYear(), browseDate.value.getMonth(), 1)
  const last = new Date(browseDate.value.getFullYear(), browseDate.value.getMonth() + 1, 0)
  const dowFirst = (first.getDay() + 6) % 7   // 周一为 0
  const dowLast = (last.getDay() + 6) % 7
  const arr: Date[] = []
  const start = new Date(first); start.setDate(1 - dowFirst)
  const end = new Date(last); end.setDate(last.getDate() + (6 - dowLast))
  const cur = new Date(start)
  while (cur.getTime() <= end.getTime()) {
    arr.push(new Date(cur))
    cur.setDate(cur.getDate() + 1)
  }
  return arr
})

const todayKey = computed(() => {
  const t = new Date()
  return `${t.getFullYear()}-${t.getMonth()}-${t.getDate()}`
})
function dKey(d: Date) {
  return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`
}
function inBrowseMonth(d: Date) {
  return d.getMonth() === browseDate.value.getMonth()
}
function isToday(d: Date) { return dKey(d) === todayKey.value }
function isSelected(d: Date) {
  if (!selectedDate.value) return false
  return dKey(d) === dKey(selectedDate.value)
}
function isWeekend(d: Date) {
  const w = d.getDay()
  return w === 0 || w === 6
}

function pickDay(d: Date) {
  selectedDate.value = new Date(d.getFullYear(), d.getMonth(), d.getDate())
  // 跨月点击时同步 browse
  if (!inBrowseMonth(d)) {
    browseDate.value = new Date(d.getFullYear(), d.getMonth(), 1)
  }
  commit()
}

function prevMonth() {
  const d = new Date(browseDate.value)
  d.setDate(1)
  d.setMonth(d.getMonth() - 1)
  browseDate.value = d
}
function nextMonth() {
  const d = new Date(browseDate.value)
  d.setDate(1)
  d.setMonth(d.getMonth() + 1)
  browseDate.value = d
}
function jumpToday() {
  const t = new Date()
  browseDate.value = new Date(t.getFullYear(), t.getMonth(), 1)
}

// ---------- 时间滚轮 ----------
const hours = Array.from({ length: 24 }, (_, i) => i)
const minutes = Array.from({ length: 12 }, (_, i) => i * 5)  // 5min 步进
const hourListRef = ref<HTMLElement | null>(null)
const minuteListRef = ref<HTMLElement | null>(null)

const ITEM_H = 36

function pickHour(h: number) {
  selectedHour.value = h
  if (!selectedDate.value) {
    // 没选日期就不能 commit；但允许预设时间
    return
  }
  commit()
}
function pickMinute(m: number) {
  selectedMinute.value = m
  if (!selectedDate.value) return
  commit()
}

// 切换 +/- 小时（用于 "+1h" / "-1h" 快捷）
function shiftMinutes(deltaMin: number) {
  if (!selectedDate.value) {
    // 无日期时仅做时间预设
    let total = selectedHour.value * 60 + selectedMinute.value + deltaMin
    total = ((total % 1440) + 1440) % 1440
    selectedHour.value = Math.floor(total / 60)
    selectedMinute.value = total - selectedHour.value * 60
    // 对齐到 5 分钟
    selectedMinute.value = Math.round(selectedMinute.value / 5) * 5
    if (selectedMinute.value === 60) {
      selectedMinute.value = 0
      selectedHour.value = (selectedHour.value + 1) % 24
    }
    scrollWheelsToSelected()
    return
  }
  const d = new Date(selectedDate.value)
  d.setHours(selectedHour.value, selectedMinute.value, 0, 0)
  d.setMinutes(d.getMinutes() + deltaMin)
  selectedDate.value = new Date(d.getFullYear(), d.getMonth(), d.getDate())
  selectedHour.value = d.getHours()
  // 时间对齐到 5 分钟（只在大跨度时）
  selectedMinute.value = Math.round(d.getMinutes() / 5) * 5
  if (selectedMinute.value === 60) {
    selectedMinute.value = 0
    selectedHour.value = (selectedHour.value + 1) % 24
  }
  if (browseDate.value.getMonth() !== selectedDate.value.getMonth()) {
    browseDate.value = new Date(
      selectedDate.value.getFullYear(),
      selectedDate.value.getMonth(),
      1,
    )
  }
  commit()
  scrollWheelsToSelected()
}

function scrollWheelsToSelected() {
  nextTick(() => {
    if (hourListRef.value) {
      hourListRef.value.scrollTo({
        top: selectedHour.value * ITEM_H,
        behavior: 'smooth',
      })
    }
    if (minuteListRef.value) {
      const idx = Math.round(selectedMinute.value / 5)
      minuteListRef.value.scrollTo({
        top: idx * ITEM_H,
        behavior: 'smooth',
      })
    }
  })
}

// ---------- 滚轮"软滚"防过冲 ----------
//
// 用户痛点: 在分钟盘上稍微一滑，惯性会带过去好几个 5 分钟档，导致很难
// 精确停在想要的值上。修复思路:
//   1. 监听 scroll 事件,在停下时(scrollend / 一段防抖延迟)读取容器
//      的 scrollTop, 反推用户实际"停"在了哪个 item, 然后写回 selected
//      并平滑对齐到正中。
//   2. 不主动节流滚动事件 —— 让 CSS 的 scroll-snap 先做粗对齐,
//      JS 只负责把状态同步到 selected。
//   3. 对于鼠标滚轮 / 触控板这类"惯性极强"的输入, 浏览器一次 wheel 事件
//      会带来 100+ 像素的 scroll, 一滑就跳过好几档. 因此我们额外拦截
//      wheel 事件: 阻止默认滚动, 改为"每个滚轮刻度精确步进 1 档".
//      触摸滑动 / 拖拽滚动 不会触发 wheel, 仍走原生路径 + scroll-snap.
const HOUR_STEP_PX = 36
let hourScrollTimer: number | null = null
let minuteScrollTimer: number | null = null

// 鼠标滚轮专用: 把"惯性滚动"折算成"逐档跳", 并节流防抖避免一次滚轮事件
// 累积成多次 step. 我们累计 deltaY, 达到阈值才动一档.
const WHEEL_THRESHOLD = 24
let hourWheelAcc = 0
let minuteWheelAcc = 0
let wheelCooldownTimer: number | null = null
function onHourWheel(e: WheelEvent) {
  e.preventDefault()
  hourWheelAcc += e.deltaY
  // 累积到阈值就步进一档. 每次只动一档, 哪怕一次惯性滑动 deltaY 很大,
  // 也会因为 cooldown 而被截断 —— 这正是用户想要的"精确感".
  while (Math.abs(hourWheelAcc) >= WHEEL_THRESHOLD) {
    const dir = hourWheelAcc > 0 ? 1 : -1
    stepHour(dir as 1 | -1)
    hourWheelAcc -= dir * WHEEL_THRESHOLD
  }
  // 余量在 cooldown 后清零, 避免"滚到一半被冻住"的体感.
  if (wheelCooldownTimer) window.clearTimeout(wheelCooldownTimer)
  wheelCooldownTimer = window.setTimeout(() => {
    hourWheelAcc = 0
    minuteWheelAcc = 0
  }, 180)
}
function onMinuteWheel(e: WheelEvent) {
  e.preventDefault()
  minuteWheelAcc += e.deltaY
  while (Math.abs(minuteWheelAcc) >= WHEEL_THRESHOLD) {
    const dir = minuteWheelAcc > 0 ? 1 : -1
    stepMinute(dir as 1 | -1)
    minuteWheelAcc -= dir * WHEEL_THRESHOLD
  }
  if (wheelCooldownTimer) window.clearTimeout(wheelCooldownTimer)
  wheelCooldownTimer = window.setTimeout(() => {
    hourWheelAcc = 0
    minuteWheelAcc = 0
  }, 180)
}
function onHourScroll() {
  if (!hourListRef.value) return
  if (hourScrollTimer) window.clearTimeout(hourScrollTimer)
  hourScrollTimer = window.setTimeout(() => {
    if (!hourListRef.value) return
    const idx = Math.round(hourListRef.value.scrollTop / HOUR_STEP_PX)
    const h = Math.max(0, Math.min(23, idx))
    if (h !== selectedHour.value) {
      selectedHour.value = h
      if (selectedDate.value) commit()
    }
    // 对齐到正中, 避免 scroll-snap 在一些浏览器上"差一个像素"
    const target = h * HOUR_STEP_PX
    if (Math.abs(hourListRef.value.scrollTop - target) > 1) {
      hourListRef.value.scrollTo({ top: target, behavior: 'smooth' })
    }
  }, 120)
}
function onMinuteScroll() {
  if (!minuteListRef.value) return
  if (minuteScrollTimer) window.clearTimeout(minuteScrollTimer)
  minuteScrollTimer = window.setTimeout(() => {
    if (!minuteListRef.value) return
    const idx = Math.round(minuteListRef.value.scrollTop / HOUR_STEP_PX)
    const clampedIdx = Math.max(0, Math.min(11, idx))
    const m = clampedIdx * 5
    if (m !== selectedMinute.value) {
      selectedMinute.value = m
      if (selectedDate.value) commit()
    }
    const target = clampedIdx * HOUR_STEP_PX
    if (Math.abs(minuteListRef.value.scrollTop - target) > 1) {
      minuteListRef.value.scrollTo({ top: target, behavior: 'smooth' })
    }
  }, 120)
}

// 直接 +/- 一个 5 分钟档；专门为"我只想动一下"的场景设计
function stepMinute(delta: 1 | -1) {
  let idx = Math.round(selectedMinute.value / 5) + delta
  let h = selectedHour.value
  if (idx >= 12) { idx = 0; h = (h + 1) % 24 }
  if (idx < 0)  { idx = 11; h = (h + 23) % 24 }
  selectedMinute.value = idx * 5
  selectedHour.value = h
  if (selectedDate.value) commit()
  scrollWheelsToSelected()
}
function stepHour(delta: 1 | -1) {
  selectedHour.value = (selectedHour.value + delta + 24) % 24
  if (selectedDate.value) commit()
  scrollWheelsToSelected()
}

// ---------- 快捷预设 ----------
function presetToday(hh: number, mm: number) {
  const t = new Date()
  selectedDate.value = new Date(t.getFullYear(), t.getMonth(), t.getDate())
  browseDate.value = new Date(t.getFullYear(), t.getMonth(), 1)
  selectedHour.value = hh
  selectedMinute.value = mm
  commit()
  scrollWheelsToSelected()
}
function presetTomorrow(hh: number, mm: number) {
  const t = new Date()
  t.setDate(t.getDate() + 1)
  selectedDate.value = new Date(t.getFullYear(), t.getMonth(), t.getDate())
  browseDate.value = new Date(t.getFullYear(), t.getMonth(), 1)
  selectedHour.value = hh
  selectedMinute.value = mm
  commit()
  scrollWheelsToSelected()
}
function presetWeekend() {
  // 找到本周最近的周六（如果今天就是周六或周日，取这周日）
  const t = new Date()
  const day = (t.getDay() + 6) % 7  // 周一=0...周日=6
  let delta: number
  if (day < 5) delta = 5 - day  // 推到周六
  else if (day === 5) delta = 1 // 周六 → 周日
  else delta = 0                // 周日就是今天
  const x = new Date(t)
  x.setDate(t.getDate() + delta)
  selectedDate.value = new Date(x.getFullYear(), x.getMonth(), x.getDate())
  browseDate.value = new Date(x.getFullYear(), x.getMonth(), 1)
  selectedHour.value = 18
  selectedMinute.value = 0
  commit()
  scrollWheelsToSelected()
}
function clearAll() {
  selectedDate.value = null
  emit('update:modelValue', '')
}

// ---------- 弹层管理 ----------
function toggleOpen() {
  if (props.disabled) return
  open.value = !open.value
  if (open.value) {
    // 打开时：把滚轮滚到选中位置 & 把日历切到选中所在月
    if (selectedDate.value) {
      browseDate.value = new Date(
        selectedDate.value.getFullYear(),
        selectedDate.value.getMonth(),
        1,
      )
    }
    nextTick(() => {
      scrollWheelsToSelected()
    })
  }
}
function close() { open.value = false }

// 点击外部关闭
function onDocClick(e: MouseEvent) {
  if (!open.value) return
  const t = e.target as Node
  if (
    triggerRef.value && !triggerRef.value.contains(t) &&
    popRef.value && !popRef.value.contains(t)
  ) {
    close()
  }
}
function onKey(e: KeyboardEvent) {
  if (!open.value) return
  if (e.key === 'Escape') {
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
    <!-- ============ 触发按钮（输入框样式） ============ -->
    <button
      ref="triggerRef"
      type="button"
      class="pdt-trigger"
      :class="{ 'is-empty': isEmpty, 'is-open': open }"
      :disabled="disabled"
      @click="toggleOpen"
    >
      <span class="pdt-trigger-icon" aria-hidden="true">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor"
             stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
          <rect x="3" y="4" width="18" height="18" rx="3" ry="3"/>
          <line x1="16" y1="2" x2="16" y2="6"/>
          <line x1="8" y1="2" x2="8" y2="6"/>
          <line x1="3" y1="10" x2="21" y2="10"/>
        </svg>
      </span>
      <span class="pdt-trigger-label">{{ triggerLabel }}</span>
      <span class="pdt-trigger-caret" aria-hidden="true">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor"
             stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="6 9 12 15 18 9"/>
        </svg>
      </span>
    </button>

    <!-- ============ 弹层 ============
         挂到 <body> 下铺一层全屏 overlay, 用 flex 把弹层在视口中居中.
         好处:
           - 弹层与背后居中的表单天然对齐, 不再"偏一边".
           - overlay 自带 backdrop-filter: blur, 把表单虚化 / 压暗,
             把注意力集中到表盘上.
           - overlay 自己捕获 mousedown -- 因为通过 Teleport 挂到 <body> 下,
             与表单的 modal-backdrop 是 DOM 兄弟, 事件不冒泡到表单的
             @click.self 上, 所以"点击表盘外区域"只关闭表盘, 不关闭表单. -->
    <Teleport to="body">
      <Transition name="pdt-pop">
        <div
          v-if="open"
          class="pdt-overlay"
          @mousedown.self="close"
        >
        <div
          ref="popRef"
          class="pdt-pop"
        >
        <!-- 顶部：快捷预设 -->
        <div class="pdt-presets">
          <button class="pdt-preset" @click="presetToday(23, 59)">
            <span class="pdt-preset-icon">⚡</span>
            今天
          </button>
          <button class="pdt-preset" @click="presetTomorrow(9, 0)">
            <span class="pdt-preset-icon">🌅</span>
            明早 9:00
          </button>
          <button class="pdt-preset" @click="presetTomorrow(18, 0)">
            <span class="pdt-preset-icon">🌆</span>
            明晚 18:00
          </button>
          <button class="pdt-preset" @click="presetWeekend">
            <span class="pdt-preset-icon">🏖️</span>
            周末
          </button>
        </div>

        <!-- 主体: 日历(左) + 时间(右) 两列布局 -->
        <div class="pdt-main">
          <!-- 左侧: 日历 -->
          <div class="pdt-cal">
            <div class="pdt-cal-head">
              <button class="pdt-cal-nav" type="button" @click="prevMonth" aria-label="上个月">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor"
                     stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
                  <polyline points="15 18 9 12 15 6"/>
                </svg>
              </button>
              <button class="pdt-cal-title" type="button" @click="jumpToday" title="回到本月">
                {{ monthLabel }}
              </button>
              <button class="pdt-cal-nav" type="button" @click="nextMonth" aria-label="下个月">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor"
                     stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
                  <polyline points="9 18 15 12 9 6"/>
                </svg>
              </button>
            </div>
            <div class="pdt-cal-dows">
              <span v-for="(w, i) in dows" :key="i" :class="{ 'is-weekend': i >= 5 }">
                {{ w }}
              </span>
            </div>
            <div class="pdt-cal-grid">
              <button
                v-for="(d, i) in cells"
                :key="i"
                type="button"
                class="pdt-cal-cell"
                :class="{
                  'is-out': !inBrowseMonth(d),
                  'is-today': isToday(d),
                  'is-selected': isSelected(d),
                  'is-weekend': isWeekend(d),
                }"
                @click="pickDay(d)"
              >{{ d.getDate() }}</button>
            </div>
          </div>

          <!-- 右侧: 时间 -->
          <div class="pdt-time">
            <div class="pdt-time-head">
              <span class="pdt-time-title">
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                     stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
                  <circle cx="12" cy="12" r="10"/>
                  <polyline points="12 6 12 12 16 14"/>
                </svg>
                时间
              </span>
            </div>

            <!-- 大字号显示当前时间 -->
            <div class="pdt-time-display">
              <span class="pdt-time-value">{{ String(selectedHour).padStart(2, '0') }}</span>
              <span class="pdt-time-colon">:</span>
              <span class="pdt-time-value">{{ String(selectedMinute).padStart(2, '0') }}</span>
            </div>

            <!-- 步进按钮 (单击精确 ±5 分钟 / ±1 小时) -->
            <div class="pdt-step-row">
              <div class="pdt-step-group">
                <button type="button" class="pdt-step-btn" title="-1 小时" @click="stepHour(-1)">
                  <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round"><line x1="5" y1="12" x2="19" y2="12"/></svg>
                </button>
                <span class="pdt-step-label">小时</span>
                <button type="button" class="pdt-step-btn" title="+1 小时" @click="stepHour(1)">
                  <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
                </button>
              </div>
              <div class="pdt-step-group">
                <button type="button" class="pdt-step-btn" title="-5 分钟" @click="stepMinute(-1)">
                  <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round"><line x1="5" y1="12" x2="19" y2="12"/></svg>
                </button>
                <span class="pdt-step-label">5 分</span>
                <button type="button" class="pdt-step-btn" title="+5 分钟" @click="stepMinute(1)">
                  <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
                </button>
              </div>
            </div>

            <!-- 滚轮 (拖滑选择) -->
            <div class="pdt-wheels">
              <!-- 小时 -->
              <div class="pdt-wheel" @wheel.prevent="onHourWheel">
                <div class="pdt-wheel-band" aria-hidden="true" />
                <div ref="hourListRef" class="pdt-wheel-list" @scroll.passive="onHourScroll">
                  <div class="pdt-wheel-pad" />
                  <div
                    v-for="h in hours"
                    :key="`h-${h}`"
                    class="pdt-wheel-item"
                    :class="{ 'is-selected': h === selectedHour }"
                    @click="pickHour(h)"
                  >
                    {{ String(h).padStart(2, '0') }}
                  </div>
                  <div class="pdt-wheel-pad" />
                </div>
              </div>
              <span class="pdt-wheel-sep">:</span>
              <!-- 分钟 -->
              <div class="pdt-wheel" @wheel.prevent="onMinuteWheel">
                <div class="pdt-wheel-band" aria-hidden="true" />
                <div ref="minuteListRef" class="pdt-wheel-list" @scroll.passive="onMinuteScroll">
                  <div class="pdt-wheel-pad" />
                  <div
                    v-for="m in minutes"
                    :key="`m-${m}`"
                    class="pdt-wheel-item"
                    :class="{ 'is-selected': m === selectedMinute }"
                    @click="pickMinute(m)"
                  >
                    {{ String(m).padStart(2, '0') }}
                  </div>
                  <div class="pdt-wheel-pad" />
                </div>
              </div>
            </div>

            <div class="pdt-wheel-hint">滑动滚轮 · 单击数字 · 或点上方步进按钮</div>
          </div>
        </div>

        <!-- 底部：清空 / 完成 -->
        <div class="pdt-foot">
          <button
            v-if="allowClear !== false"
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
          <button type="button" class="pdt-foot-btn pdt-foot-done" @click="close">
            完成
          </button>
        </div>
        </div><!-- /.pdt-pop -->
        </div><!-- /.pdt-overlay -->
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
/* =====================================================
 *  PrettyDateTimePicker
 *  全部用应用统一的 var(--tg-*) 设计 token
 * ===================================================== */

.pdt {
  position: relative;
  width: 100%;
}
.pdt.is-disabled { opacity: 0.55; pointer-events: none; }

/* ===== 触发按钮 ===== */
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
  box-shadow: 0 4px 14px -6px rgba(99, 102, 241, 0.28);
}
.pdt-trigger.is-open,
.pdt-trigger:focus-visible {
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
  flex-shrink: 0;
  width: 22px; height: 22px;
  color: var(--tg-primary);
  transition: transform var(--tg-trans-fast);
}
.pdt-trigger-icon svg { width: 18px; height: 18px; }
.pdt-trigger:hover .pdt-trigger-icon,
.pdt-trigger.is-open .pdt-trigger-icon { transform: scale(1.1); }

.pdt-trigger-label {
  flex: 1; min-width: 0;
  letter-spacing: 0.005em;
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

/* ===== 弹层 =====
   .pdt-overlay   是全屏遮罩, flex 把 .pdt-pop 居中, 同时虚化 / 压暗背后的表单.
   .pdt-pop       是真正的时间选择面板, 由 overlay 用 flex 居中, 不再用 JS 定位.
   注意 Teleport 后 scoped 选择器依然能命中, 因为 scoped 样式靠 data-v-* 属性
   匹配, Teleport 不会删 data-v-*. */
.pdt-overlay {
  position: fixed;
  inset: 0;
  z-index: 9100;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
  /* 半透明 + 模糊: 把背后的表单虚化, 让用户聚焦在时间盘上 */
  background: rgba(15, 23, 42, 0.32);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
}
.pdt-pop {
  position: relative;
  width: 580px;
  max-width: calc(100vw - 32px);
  max-height: calc(100vh - 32px);
  overflow: auto;
  background: var(--tg-bg-elev);
  border: 1px solid var(--tg-divider);
  border-radius: var(--tg-radius-lg);
  box-shadow:
    0 28px 80px -20px rgba(15, 23, 42, 0.30),
    0 12px 36px -8px rgba(15, 23, 42, 0.14),
    0 0 0 1px rgba(99, 102, 241, 0.06) inset;
}

/* ===== 主体: 日历(左) + 时间(右) =====
   收紧两列宽度以适配 580px 弹层. 之前 minmax(280, 1fr) + minmax(220, auto)
   会让弹层最小要 ~520-540px, 在被 modal 裁过一次以后右侧"时间"就被切掉一半 —
   连分钟都看不到. 现在固定时间列宽度 220px, 余下都给日历, 两列严格对齐. */
.pdt-main {
  display: grid;
  grid-template-columns: 1fr 220px;
  gap: 0;
}
.pdt-main > .pdt-time {
  border-top: none;
  border-left: 1px solid var(--tg-divider);
}

/* ===== 快捷预设 ===== */
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
  font-size: 11.5px; font-weight: 600;
  color: var(--tg-text-secondary);
  cursor: pointer;
  transition: all var(--tg-trans-fast);
  line-height: 1.2;
}
.pdt-preset:hover {
  background: color-mix(in srgb, var(--tg-primary) 10%, transparent);
  border-color: color-mix(in srgb, var(--tg-primary) 35%, transparent);
  color: var(--tg-primary);
  transform: translateY(-1px);
}
.pdt-preset:active { transform: translateY(0) scale(0.97); }
.pdt-preset-icon { font-size: 16px; line-height: 1; }

/* ===== 日历区 ===== */
.pdt-cal {
  padding: 10px 12px 6px;
}
.pdt-cal-head {
  display: flex; align-items: center; justify-content: space-between;
  margin-bottom: 4px;
}
.pdt-cal-nav {
  width: 28px; height: 28px;
  display: inline-flex; align-items: center; justify-content: center;
  background: transparent;
  border: none;
  border-radius: var(--tg-radius-pill);
  color: var(--tg-text-secondary);
  cursor: pointer;
  transition: all var(--tg-trans-fast);
}
.pdt-cal-nav:hover {
  background: var(--tg-hover);
  color: var(--tg-primary);
  transform: scale(1.05);
}
.pdt-cal-nav svg { width: 16px; height: 16px; }
.pdt-cal-title {
  flex: 1;
  background: transparent;
  border: none;
  font-family: 'Sora', sans-serif;
  font-size: 14.5px; font-weight: 700;
  letter-spacing: -0.018em;
  color: var(--tg-text);
  cursor: pointer;
  padding: 4px 8px;
  border-radius: var(--tg-radius-sm);
  transition: background var(--tg-trans-fast), color var(--tg-trans-fast);
}
.pdt-cal-title:hover {
  background: var(--tg-hover);
  color: var(--tg-primary);
}

.pdt-cal-dows {
  display: grid;
  grid-template-columns: repeat(7, 1fr);
  margin-bottom: 2px;
}
.pdt-cal-dows span {
  text-align: center;
  font-family: 'Sora', sans-serif;
  font-size: 10.5px; font-weight: 700;
  color: var(--tg-text-tertiary);
  letter-spacing: 0.06em;
  padding: 5px 0;
}
.pdt-cal-dows span.is-weekend { color: color-mix(in srgb, var(--tg-primary) 65%, var(--tg-text-tertiary)); }

.pdt-cal-grid {
  display: grid;
  grid-template-columns: repeat(7, 1fr);
  gap: 2px;
}
.pdt-cal-cell {
  appearance: none;
  height: 32px;
  display: inline-flex; align-items: center; justify-content: center;
  background: transparent;
  border: none;
  border-radius: 8px;
  font-family: 'Sora', sans-serif;
  font-size: 12.5px; font-weight: 600;
  color: var(--tg-text);
  cursor: pointer;
  transition: all var(--tg-trans-fast);
  font-variant-numeric: tabular-nums;
  position: relative;
}
.pdt-cal-cell:hover {
  background: var(--tg-hover);
  color: var(--tg-primary);
}
.pdt-cal-cell.is-out { color: var(--tg-text-tertiary); opacity: 0.45; }
.pdt-cal-cell.is-weekend:not(.is-out):not(.is-selected) {
  color: color-mix(in srgb, var(--tg-primary) 70%, var(--tg-text));
}
.pdt-cal-cell.is-today {
  background: color-mix(in srgb, var(--tg-primary) 10%, transparent);
  color: var(--tg-primary);
  font-weight: 800;
}
.pdt-cal-cell.is-today::after {
  content: '';
  position: absolute;
  bottom: 4px;
  width: 4px; height: 4px;
  border-radius: 50%;
  background: var(--tg-primary);
}
.pdt-cal-cell.is-selected {
  background: var(--tg-grad-brand);
  color: var(--tg-on-primary);
  font-weight: 800;
  box-shadow: 0 4px 14px -3px rgba(99, 102, 241, 0.45);
  transform: scale(1.02);
}
.pdt-cal-cell.is-selected.is-today::after {
  background: rgba(255, 255, 255, 0.95);
}

/* ===== 时间区 =====
   收紧 padding / 间距以适配 220px 列宽. 关键: pdt-step-row 改成允许换行的
   flex-wrap, 这样在窄列里两组 ±按钮会自动分两行而不是被挤出列外. */
.pdt-time {
  border-top: 1px solid var(--tg-divider);
  padding: 12px 12px 10px;
  display: flex;
  flex-direction: column;
  min-width: 0;        /* 防止 flex 子项撑爆父级宽度 */
}
.pdt-time-head {
  display: flex; align-items: center; justify-content: space-between;
  margin-bottom: 6px;
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

/* 大字号当前值 */
.pdt-time-display {
  display: flex; align-items: baseline; justify-content: center;
  gap: 4px;
  padding: 6px 0 4px;
  font-family: 'Sora', sans-serif;
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.02em;
}
.pdt-time-value {
  font-size: 26px; font-weight: 800;
  color: var(--tg-primary);
  background: color-mix(in srgb, var(--tg-primary) 8%, transparent);
  padding: 2px 10px;
  border-radius: 8px;
  min-width: 56px; text-align: center;
}
.pdt-time-colon {
  font-size: 24px; font-weight: 800;
  color: var(--tg-text-secondary);
}

/* 步进按钮组 */
.pdt-step-row {
  display: flex; flex-wrap: wrap; gap: 6px; justify-content: center;
  margin: 6px 0 8px;
}
.pdt-step-group {
  display: inline-flex; align-items: center; gap: 4px;
  padding: 3px;
  background: var(--tg-hover);
  border: 1px solid var(--tg-divider);
  border-radius: 999px;
}
.pdt-step-btn {
  appearance: none;
  width: 26px; height: 26px;
  display: inline-flex; align-items: center; justify-content: center;
  background: var(--tg-bg-elev);
  border: 1px solid var(--tg-divider);
  border-radius: 50%;
  color: var(--tg-primary);
  cursor: pointer;
  transition: all var(--tg-trans-fast);
}
.pdt-step-btn:hover {
  background: var(--tg-primary);
  color: var(--tg-on-primary);
  border-color: var(--tg-primary);
  transform: scale(1.06);
}
.pdt-step-btn:active { transform: scale(0.94); }
.pdt-step-label {
  font-family: 'Sora', sans-serif;
  font-size: 11px; font-weight: 700;
  color: var(--tg-text-secondary);
  padding: 0 4px;
  letter-spacing: 0.04em;
}

/* ===== 滚轮 ===== */
.pdt-wheels {
  display: flex; align-items: center; justify-content: center;
  gap: 10px;
  padding: 4px 0 0;
}
.pdt-wheel {
  position: relative;
  width: 70px; height: 144px;  /* 4 * 36 */
  border-radius: var(--tg-radius-md);
  background: var(--tg-hover);
  overflow: hidden;
}
.pdt-wheel-hint {
  font-size: 10.5px;
  color: var(--tg-text-tertiary);
  text-align: center;
  margin-top: 8px;
  font-weight: 500;
  letter-spacing: 0.02em;
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
  -ms-overflow-style: none;
  /* 顶部/底部羽化 */
  mask-image: linear-gradient(180deg,
    transparent 0%, #000 22%, #000 78%, transparent 100%);
  -webkit-mask-image: linear-gradient(180deg,
    transparent 0%, #000 22%, #000 78%, transparent 100%);
}
.pdt-wheel-list::-webkit-scrollbar { display: none; }
.pdt-wheel-pad {
  height: 54px;  /* (144 - 36) / 2 */
}
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
.pdt-wheel-item:hover {
  color: var(--tg-primary);
}
.pdt-wheel-item.is-selected {
  color: var(--tg-primary);
  font-size: 18px;
  font-weight: 800;
  letter-spacing: -0.02em;
  transform: scale(1.05);
}
.pdt-wheel-sep {
  font-family: 'Sora', sans-serif;
  font-size: 28px; font-weight: 800;
  color: var(--tg-primary);
  letter-spacing: -0.02em;
  margin-bottom: 4px;
}

/* ===== Foot ===== */
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
.pdt-foot-clear {
  background: transparent;
  color: var(--tg-text-tertiary);
}
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

/* ===== Transition =====
   外层 overlay 做淡入淡出, 内层 .pdt-pop 自己再叠一个轻微的"弹"动效, 让面板
   出现时有质感, 但不会干扰 overlay 的 backdrop-filter 渐变. */
.pdt-pop-enter-active {
  transition: opacity 200ms ease, backdrop-filter 200ms ease;
}
.pdt-pop-leave-active {
  transition: opacity 160ms ease, backdrop-filter 160ms ease;
}
.pdt-pop-enter-from, .pdt-pop-leave-to {
  opacity: 0;
}
.pdt-pop-enter-active .pdt-pop {
  animation: pdt-pop-in 240ms cubic-bezier(0.34, 1.56, 0.64, 1);
}
@keyframes pdt-pop-in {
  from { transform: translateY(8px) scale(0.96); opacity: 0; }
  to   { transform: translateY(0)    scale(1);    opacity: 1; }
}

/* ===== 移动端优化 =====
   设计选择: 用户明确要求时间永远在日期右边(两列布局), 因此不再把它折成
   单列堆叠. 即使在 ~480px 宽以下也保留两列, 仅切换为"底部抽屉"形式以贴近
   原生移动端选择器的体感. 这样选时间的入口永远不会被砍掉.
   现在弹层位置由 overlay 的 flex 控制 (居中 / 贴底), 不再依赖 JS 定位. */
@media (max-width: 480px) {
  .pdt-overlay {
    align-items: flex-end;
    justify-content: stretch;
    padding: 0;
  }
  .pdt-pop {
    width: 100vw;
    max-width: 100vw;
    max-height: 88vh;
    border-radius: var(--tg-radius-xl) var(--tg-radius-xl) 0 0;
    border-bottom: none;
    box-shadow:
      0 -16px 60px -8px rgba(15, 23, 42, 0.30),
      0 -8px 20px -4px rgba(15, 23, 42, 0.14);
  }
  .pdt-pop::before {
    /* 顶部把手条 */
    content: '';
    position: absolute;
    top: 8px; left: 50%; transform: translateX(-50%);
    width: 38px; height: 4px;
    border-radius: 999px;
    background: var(--tg-divider-strong);
  }
  .pdt-presets { padding-top: 24px; }
  /* 在底部抽屉下日历列收紧, 但仍与时间并排 */
  .pdt-main { grid-template-columns: 1fr 200px; }
  .pdt-time { padding: 10px 8px 8px; }
  .pdt-wheel { width: 60px; }
  /* 移动端从底部弹起的动效 */
  .pdt-pop-enter-active .pdt-pop {
    animation: pdt-pop-in-mobile 260ms cubic-bezier(0.34, 1.56, 0.64, 1);
  }
  @keyframes pdt-pop-in-mobile {
    from { transform: translateY(100%); opacity: 0; }
    to   { transform: translateY(0);    opacity: 1; }
  }
}
</style>
