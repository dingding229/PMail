import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import isToday from 'dayjs/plugin/isToday'
import isYesterday from 'dayjs/plugin/isYesterday'
import 'dayjs/locale/zh-cn'
import 'dayjs/locale/en'

dayjs.extend(relativeTime)
dayjs.extend(isToday)
dayjs.extend(isYesterday)

export function setDayjsLocale(lang: string): void {
  dayjs.locale(lang.toLowerCase().startsWith('zh') ? 'zh-cn' : 'en')
}

export function formatMailListTime(dateStr: string, lang: string): string {
  if (!dateStr) return ''
  const d = dayjs(dateStr)
  if (!d.isValid()) return dateStr
  const isZh = lang.toLowerCase().startsWith('zh')
  const now = dayjs()
  const diffMins = now.diff(d, 'minute')

  if (diffMins >= 0 && diffMins < 1) {
    return isZh ? '刚刚' : 'Just now'
  }
  if (diffMins >= 1 && diffMins < 60) {
    return d.fromNow()
  }
  if (d.isToday()) {
    return d.format('HH:mm')
  }
  if (d.isYesterday()) {
    return isZh ? `昨天 ${d.format('HH:mm')}` : `Yesterday`
  }
  if (d.year() === now.year()) {
    return isZh ? d.format('M月D日') : d.format('MMM D')
  }
  return isZh ? d.format('YYYY/M/D') : d.format('YYYY-MM-DD')
}

export function formatMailFullTime(dateStr: string, lang: string): string {
  if (!dateStr) return ''
  const d = dayjs(dateStr)
  if (!d.isValid()) return dateStr
  const isZh = lang.toLowerCase().startsWith('zh')
  return isZh ? d.format('YYYY年M月D日 HH:mm') : d.format('MMM D, YYYY, HH:mm')
}

export function formatRelativeTime(dateStr: string): string {
  if (!dateStr) return ''
  const d = dayjs(dateStr)
  if (!d.isValid()) return ''
  return d.fromNow()
}
