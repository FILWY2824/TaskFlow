// 自定义 confirm / alert 对话框（替代浏览器原生的 window.confirm / window.alert）。
//
// 设计要点:
//   - 用一个全局 reactive 队列驱动 <AppDialogs /> (在 App.vue 挂载) 渲染。
//   - 每个 dialog 都返回一个 Promise<boolean>，true=确认，false=取消/关闭。
//   - 完全不依赖原生弹窗：移动端体验、动画、配色都和应用其它部分一致。
//
// 用法:
//   const ok = await confirmDialog({
//     title: '确认删除任务?',
//     message: `任务 "${name}" 将被永久删除。`,
//     confirmText: '删除',
//     danger: true,
//   })
//   if (!ok) return
//   ...

import { reactive } from 'vue'

export type DialogKind = 'confirm' | 'alert'

export interface DialogOptions {
  title?: string
  message: string
  confirmText?: string
  cancelText?: string
  /** 危险风格：确认按钮变红色，左侧改用警示图标 */
  danger?: boolean
  /** alert 模式只显示一个"知道了"按钮 */
  kind?: DialogKind
}

interface DialogItem extends Required<Omit<DialogOptions, 'title'>> {
  id: number
  title: string
  resolve: (v: boolean) => void
}

let _seq = 0

export const dialogState = reactive<{ items: DialogItem[] }>({
  items: [],
})

function pushDialog(opts: DialogOptions): Promise<boolean> {
  return new Promise<boolean>((resolve) => {
    const id = ++_seq
    dialogState.items.push({
      id,
      title: opts.title ?? '',
      message: opts.message,
      confirmText: opts.confirmText ?? '确定',
      cancelText: opts.cancelText ?? '取消',
      danger: opts.danger ?? false,
      kind: opts.kind ?? 'confirm',
      resolve,
    })
  })
}

/** 自定义 confirm，返回 Promise<boolean>。 */
export function confirmDialog(opts: DialogOptions): Promise<boolean> {
  return pushDialog({ ...opts, kind: 'confirm' })
}

/** 自定义 alert，返回 Promise<true>（用户点了"知道了"）。 */
export function alertDialog(opts: DialogOptions): Promise<boolean> {
  return pushDialog({ ...opts, kind: 'alert' })
}

/** 由 <AppDialogs /> 调用：用户做了选择后清掉这条 dialog。 */
export function _resolveDialog(id: number, value: boolean) {
  const idx = dialogState.items.findIndex((x) => x.id === id)
  if (idx < 0) return
  const item = dialogState.items[idx]
  dialogState.items.splice(idx, 1)
  item.resolve(value)
}
