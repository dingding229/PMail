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

/**
 * Picks the first non-empty timestamp string from candidates.
 * Always pass RFC3339 `*_iso` fields before legacy timezone-less strings.
 */
export function pickDate(...candidates: Array<string | null | undefined>): string {
  for (const c of candidates) {
    if (typeof c === 'string' && c.trim().length > 0) {
      return c.trim()
    }
  }
  return ''
}

export function formatMailListTime(
  dateStr: string | null | undefined,
  lang: string,
  fallback?: string | null,
): string {
  const raw = pickDate(dateStr, fallback)
  if (!raw) return ''
  const d = dayjs(raw)
  if (!d.isValid()) return raw
  const isZh = lang.toLowerCase().startsWith('zh')
  const now = dayjs()
  const diffMs = now.valueOf() - d.valueOf()

  // Clamp future timestamps (e.g. slight clock skew or legacy timezone offset) to "Just now"
  if (diffMs < 60_000) {
    return isZh ? '刚刚' : 'Just now'
  }
  const diffMins = Math.floor(diffMs / 60_000)
  if (diffMins < 60) {
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

export function formatMailFullTime(
  dateStr: string | null | undefined,
  lang: string,
  fallback?: string | null,
): string {
  const raw = pickDate(dateStr, fallback)
  if (!raw) return ''
  const d = dayjs(raw)
  if (!d.isValid()) return raw
  const isZh = lang.toLowerCase().startsWith('zh')
  return isZh ? d.format('YYYY年M月D日 HH:mm') : d.format('MMM D, YYYY, HH:mm')
}

export function formatRelativeTime(
  dateStr: string | null | undefined,
  lang?: string,
  fallback?: string | null,
): string {
  const raw = pickDate(dateStr, fallback)
  if (!raw) return ''
  const d = dayjs(raw)
  if (!d.isValid()) return ''
  const now = dayjs()
  const isZh = lang ? lang.toLowerCase().startsWith('zh') : dayjs.locale().startsWith('zh')
  // Never display future relative time (e.g. "8 小时内" / "in 8 hours")
  if (now.valueOf() - d.valueOf() < 60_000) {
    return isZh ? '刚刚' : 'Just now'
  }
  return d.fromNow()
}
