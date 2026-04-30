// 简单的"叮咚"完成提示音生成器。
//
// 设计取向：
//   - 不依赖外部音频文件，纯 Web Audio API 合成两段下降琶音。
//   - 第一声较高、第二声稍低，营造"叮—咚"的清脆感。
//   - 自带轻微 ADSR 包络，避免"咔嗒"爆音。
//   - 安静失败：浏览器禁用 / 用户未交互 / Tauri 环境异常时直接静默。
//
// 用法：
//   import { playDing } from '@/sound'
//   playDing()  // 任务完成 / 番茄结束时调用

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

// 触发一段单音（频率/起始时间/时长/响度）。
function blip(
  ctx: AudioContext,
  freq: number,
  startAt: number,
  durMs: number,
  volume = 0.18,
) {
  const osc = ctx.createOscillator()
  const gain = ctx.createGain()

  osc.type = 'sine'
  osc.frequency.setValueAtTime(freq, startAt)
  // 轻微的频率下滑，让"叮"更立体
  osc.frequency.exponentialRampToValueAtTime(
    Math.max(50, freq * 0.85),
    startAt + durMs / 1000,
  )

  // ADSR：4ms attack → 短 sustain → 指数 release
  gain.gain.setValueAtTime(0.0001, startAt)
  gain.gain.exponentialRampToValueAtTime(volume, startAt + 0.005)
  gain.gain.exponentialRampToValueAtTime(0.0001, startAt + durMs / 1000)

  osc.connect(gain)
  gain.connect(ctx.destination)
  osc.start(startAt)
  osc.stop(startAt + durMs / 1000 + 0.05)
}

/**
 * 播放一声清脆的"叮咚"完成提示音。
 *
 * @param opts.muted 静音开关（来自用户偏好）；为 true 时直接 no-op。
 */
export function playDing(opts: { muted?: boolean } = {}) {
  if (opts.muted) return
  const ctx = getCtx()
  if (!ctx) return
  const t0 = ctx.currentTime + 0.01
  // "叮"：高频清脆 (E6 ≈ 1318Hz)
  blip(ctx, 1318.5, t0, 180, 0.22)
  // "咚"：稍低（A5 ≈ 880Hz），延后 110ms
  blip(ctx, 880, t0 + 0.11, 280, 0.18)
}

/**
 * 番茄结束（更长、更突出）的提示音。两声"叮咚"之后再补一个稍长的尾音。
 */
export function playPomodoroEnd(opts: { muted?: boolean } = {}) {
  if (opts.muted) return
  const ctx = getCtx()
  if (!ctx) return
  const t0 = ctx.currentTime + 0.01
  // 一声叮 + 一声咚
  blip(ctx, 1568, t0, 200, 0.24)            // G6
  blip(ctx, 1046, t0 + 0.13, 220, 0.22)     // C6
  // 短暂延后再来一组
  blip(ctx, 1318, t0 + 0.42, 220, 0.22)     // E6
  blip(ctx, 880, t0 + 0.55, 380, 0.20)      // A5
}
