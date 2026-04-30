// 常见 IANA 时区列表，按地区分组。供设置页 / 注册页的下拉框使用。
// 默认值统一为 Asia/Shanghai（中国上海时间），不在任务/提醒等业务弹窗里再让用户选时区。

export const DEFAULT_TIMEZONE = 'Asia/Shanghai'

export interface TimezoneGroup {
  label: string
  options: { value: string; label: string }[]
}

export const TIMEZONE_GROUPS: TimezoneGroup[] = [
  {
    label: '亚洲',
    options: [
      { value: 'Asia/Shanghai', label: '中国上海 (UTC+8)' },
      { value: 'Asia/Hong_Kong', label: '中国香港 (UTC+8)' },
      { value: 'Asia/Taipei', label: '中国台北 (UTC+8)' },
      { value: 'Asia/Tokyo', label: '日本东京 (UTC+9)' },
      { value: 'Asia/Seoul', label: '韩国首尔 (UTC+9)' },
      { value: 'Asia/Singapore', label: '新加坡 (UTC+8)' },
      { value: 'Asia/Bangkok', label: '泰国曼谷 (UTC+7)' },
      { value: 'Asia/Kolkata', label: '印度加尔各答 (UTC+5:30)' },
      { value: 'Asia/Dubai', label: '阿联酋迪拜 (UTC+4)' },
    ],
  },
  {
    label: '欧洲',
    options: [
      { value: 'Europe/London', label: '英国伦敦 (UTC+0/+1)' },
      { value: 'Europe/Paris', label: '法国巴黎 (UTC+1/+2)' },
      { value: 'Europe/Berlin', label: '德国柏林 (UTC+1/+2)' },
      { value: 'Europe/Moscow', label: '俄罗斯莫斯科 (UTC+3)' },
    ],
  },
  {
    label: '美洲',
    options: [
      { value: 'America/New_York', label: '美国纽约 (UTC-5/-4)' },
      { value: 'America/Chicago', label: '美国芝加哥 (UTC-6/-5)' },
      { value: 'America/Denver', label: '美国丹佛 (UTC-7/-6)' },
      { value: 'America/Los_Angeles', label: '美国洛杉矶 (UTC-8/-7)' },
      { value: 'America/Toronto', label: '加拿大多伦多 (UTC-5/-4)' },
      { value: 'America/Sao_Paulo', label: '巴西圣保罗 (UTC-3)' },
    ],
  },
  {
    label: '大洋洲 / 其他',
    options: [
      { value: 'Australia/Sydney', label: '澳大利亚悉尼 (UTC+10/+11)' },
      { value: 'Pacific/Auckland', label: '新西兰奥克兰 (UTC+12/+13)' },
      { value: 'UTC', label: 'UTC (协调世界时)' },
    ],
  },
]

// 拍平之后的查找
export function timezoneLabel(tz: string): string {
  for (const g of TIMEZONE_GROUPS) {
    for (const o of g.options) {
      if (o.value === tz) return o.label
    }
  }
  return tz
}
