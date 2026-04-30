// 简单的"叮"完成提示音生成器。
//
// 设计取向：
//   - 不依赖外部音频文件，纯 Web Audio API 合成。
//   - 极其轻快、清脆 —— 单声短促"叮"(类似音乐盒 / Slack 提示音的质感)。
//   - 自带快速的 ADSR 包络（极短 attack + 指数 release），避免"咔嗒"爆音。
//   - 安静失败：浏览器禁用 / 用户未交互 / Tauri 环境异常时直接静默。
//
// 用法：
//   import { playDing } from '@/sound'
//   playDing()  // 任务完成时调用

let _ctx: AudioContext | null = null

function getCtx(): AudioContext | null {
  try {
    // Webkit 旧浏览器（Safari）兼容
    type WC = typeof window & { webkitAudioContext?: typeof AudioContext }
    const w = window as WC
    const Ctor = w.AudioContext || w.webkitAudioContext
    if (!Ctor) return null
    if (!_ctx) _ctx = new Ctor()
    // 部分浏览器需要在用户手势后 resume
    if (_ctx.state === 'suspended') {
      _ctx.resume().catch(() => { /* ignore */ })
    }
    return _ctx
  } catch {
    return null
  }
}

// 触发一段单音 (频率 / 起始时间 / 时长ms / 响度 / 波形)。
// 短促版本: 极快 attack (1-2ms),指数 release,音色干净不"嗡"。
function blip(
  ctx: AudioContext,
  freq: number,
  startAt: number,
  durMs: number,
  volume = 0.16,
  type: OscillatorType = 'triangle',
) {
  const osc = ctx.createOscillator()
  const gain = ctx.createGain()

  osc.type = type
  osc.frequency.setValueAtTime(freq, startAt)

  // 极快 attack (1.5ms) → 立即指数衰减,营造"啪嗒"般的清脆点感
  const dur = Math.max(0.02, durMs / 1000)
  gain.gain.setValueAtTime(0.0001, startAt)
  gain.gain.exponentialRampToValueAtTime(volume, startAt + 0.0015)
  gain.gain.exponentialRampToValueAtTime(0.0001, startAt + dur)

  osc.connect(gain)
  gain.connect(ctx.destination)
  osc.start(startAt)
  osc.stop(startAt + dur + 0.02)
}

/**
 * 播放一声极轻极脆的"叮"完成提示音。
 *
 * 音色取向: 类似音乐盒高音 / 系统提示音, 单声 ~70ms, 不拖沓不沉重。
 * 主基频 ~2349 Hz (D7), 配一个略高一点的瞬时高泛音让音头更亮。
 *
 * @param opts.muted 静音开关（来自用户偏好）；为 true 时直接 no-op。
 */
export function playDing(opts: { muted?: boolean } = {}) {
  if (opts.muted) return
  const ctx = getCtx()
  if (!ctx) return
  const t0 = ctx.currentTime + 0.005
  // 主体: 短促清亮的单声"叮"
  blip(ctx, 2349.32, t0, 80, 0.14, 'triangle')   // D7
  // 极短的高音瞬态,只存在 ~25ms,让音头更"亮"但不刺耳
  blip(ctx, 3520.00, t0, 25, 0.07, 'sine')       // A7
}

/**
 * 番茄结束的提示音 —— 比 playDing 略长、略响,但仍保持清脆调性。
 * 两个轻快短"叮",间隔很短,辨识度更高。
 */
export function playPomodoroEnd(opts: { muted?: boolean } = {}) {
  if (opts.muted) return
  const ctx = getCtx()
  if (!ctx) return
  const t0 = ctx.currentTime + 0.005
  // 第一声: 高一点
  blip(ctx, 2637.02, t0, 90, 0.16, 'triangle')         // E7
  blip(ctx, 3951.07, t0, 25, 0.08, 'sine')             // B7 高泛音
  // 第二声: 略低,延后 140ms
  blip(ctx, 2093.00, t0 + 0.14, 110, 0.16, 'triangle') // C7
  blip(ctx, 3135.96, t0 + 0.14, 25, 0.08, 'sine')      // G7 高泛音
}
