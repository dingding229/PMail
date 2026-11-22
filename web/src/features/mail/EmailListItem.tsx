import { memo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  AlertCircle,
  Mail,
  MailOpen,
  Paperclip,
  ShieldAlert,
  Star,
  Trash2,
} from 'lucide-react'
import type { EmailListItem as EmailListItemModel } from '@/api'
import { formatMailListTime, pickDate } from '@/lib/date'
import { cn } from '@/lib/cn'
import { Checkbox } from '@/components/ui/checkbox'
import { SenderAvatar } from '@/components/common/SenderAvatar'
import type { ListDensity } from '@/lib/preferences'

interface EmailListItemProps {
  email: EmailListItemModel
  isActive: boolean
  isChecked: boolean
  isStarred: boolean
  selectionMode: boolean
  isMobile: boolean
  density?: ListDensity
  suppressAuthWarning?: boolean
  onSelect: (email: EmailListItemModel) => void
  onToggleCheck: (id: number) => void
  onToggleStar: (id: number, nextStarred: boolean) => void
  onSwipeToggleRead: (email: EmailListItemModel) => void
  onSwipeDelete: (email: EmailListItemModel) => void
  onPrefetch: (id: number) => void
  onCancelPrefetch: () => void
}

const SWIPE_THRESHOLD = 72

export const EmailListItem = memo(function EmailListItem({
  email,
  isActive,
  isChecked,
  isStarred,
  selectionMode,
  isMobile,
  density = 'comfortable',
  suppressAuthWarning = false,
  onSelect,
  onToggleCheck,
  onToggleStar,
  onSwipeToggleRead,
  onSwipeDelete,
  onPrefetch,
  onCancelPrefetch,
}: EmailListItemProps) {
  const { t, i18n } = useTranslation()
  const [swipeX, setSwipeX] = useState(0)
  const touchStartRef = useRef<{ x: number; y: number; active: boolean }>({
    x: 0,
    y: 0,
    active: false,
  })

  const senderName =
    email.sender?.Name || email.sender?.EmailAddress || t('mail.unknownSender')
  const senderEmail = email.sender?.EmailAddress || ''
  const isUnread = !email.is_read

  const handleTouchStart = (e: React.TouchEvent) => {
    if (!isMobile || e.touches.length !== 1) return
    const touch = e.touches[0]
    if (!touch) return
    touchStartRef.current = { x: touch.clientX, y: touch.clientY, active: true }
  }

  const handleTouchMove = (e: React.TouchEvent) => {
    if (!isMobile || !touchStartRef.current.active || e.touches.length !== 1) return
    const touch = e.touches[0]
    if (!touch) return
    const dx = touch.clientX - touchStartRef.current.x
    const dy = touch.clientY - touchStartRef.current.y
    if (Math.abs(dy) > Math.abs(dx) && Math.abs(swipeX) < 8) {
      touchStartRef.current.active = false
      return
    }
    if (Math.abs(dx) > 10) {
      setSwipeX(Math.max(-108, Math.min(108, dx)))
    }
  }

  const handleTouchEnd = () => {
    if (!isMobile) return
    if (swipeX >= SWIPE_THRESHOLD) {
      onSwipeToggleRead(email)
    } else if (swipeX <= -SWIPE_THRESHOLD) {
      onSwipeDelete(email)
    }
    setSwipeX(0)
    touchStartRef.current.active = false
  }

  const hasAttachments =
    Boolean(email.has_attachments) ||
    Boolean(email.attachments_list && email.attachments_list.some((a) => !a.inline))
  const displayDate = pickDate(
    email.send_date_iso,
    email.datetime_iso,
    email.send_time_iso,
    email.create_time_iso,
    email.update_time_iso,
    email.datetime,
  )
  const isDraftOrSentMessage =
    suppressAuthWarning || email.status === 4 || email.status === 1 || email.type === 1
  const showAuthWarning = isDraftOrSentMessage
    ? false
    : email.auth_status !== undefined
      ? email.auth_status === 'fail'
      : Boolean(email.dangerous)
  const paddingClass =
    density === 'compact' ? 'py-2' : density === 'spacious' ? 'py-4' : 'py-3'

  return (
    <div
      className="relative overflow-hidden border-b border-[var(--border-hairline)]"
      onTouchStart={handleTouchStart}
      onTouchMove={handleTouchMove}
      onTouchEnd={handleTouchEnd}
    >
      {/* Mobile swipe backdrop actions */}
      {isMobile && swipeX !== 0 && (
        <div className="pointer-events-none absolute inset-0 flex items-center justify-between px-4 text-[12px] font-medium text-white">
          <div
            className={cn(
              'flex items-center gap-1.5 rounded-md px-2 py-1 transition-opacity',
              swipeX > 20 ? 'bg-[var(--accent)] opacity-100' : 'opacity-0',
            )}
          >
            {isUnread ? <MailOpen className="h-4 w-4" /> : <Mail className="h-4 w-4" />}
            <span>{isUnread ? t('mail.swipe.read') : t('mail.swipe.unread')}</span>
          </div>
          <div
            className={cn(
              'flex items-center gap-1.5 rounded-md px-2 py-1 transition-opacity',
              swipeX < -20 ? 'bg-[var(--danger)] opacity-100' : 'opacity-0',
            )}
          >
            <Trash2 className="h-4 w-4" />
            <span>{t('mail.swipe.delete')}</span>
          </div>
        </div>
      )}

      {/* Main row */}
      <div
        role="option"
        aria-selected={isActive}
        tabIndex={0}
        style={
          swipeX !== 0
            ? { transform: `translateX(${swipeX}px)` }
            : undefined
        }
        onClick={() => onSelect(email)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            onSelect(email)
          }
        }}
        onMouseEnter={() => onPrefetch(email.id)}
        onMouseLeave={onCancelPrefetch}
        onFocus={() => onPrefetch(email.id)}
        onBlur={onCancelPrefetch}
        className={cn(
          'group relative flex cursor-pointer items-start gap-3 px-3.5 transition-colors duration-[var(--duration-fast)] select-none outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-[var(--accent-ring)]',
          paddingClass,
          isActive
            ? 'bg-[var(--surface-selected)]'
            : isChecked
              ? 'bg-[var(--accent-soft)]/50'
              : 'bg-[var(--surface-list)] hover:bg-[var(--surface-hover)]',
        )}
      >
        {/* Active left accent bar */}
        {isActive && (
          <span
            aria-hidden="true"
            className="absolute inset-y-1.5 left-0 w-[3px] rounded-r-full bg-[var(--accent)]"
          />
        )}

        {/* Unread dot + Avatar / Checkbox swap */}
        <div className="relative mt-0.5 flex shrink-0 items-center">
          <span
            aria-hidden="true"
            className={cn(
              'mr-1.5 h-2 w-2 rounded-full transition-opacity',
              isUnread ? 'bg-[var(--unread-dot)] opacity-100' : 'opacity-0',
            )}
          />
          <div
            className="relative h-8 w-8"
            onClick={(e) => {
              e.stopPropagation()
              onToggleCheck(email.id)
            }}
          >
            <div
              className={cn(
                'transition-opacity duration-[var(--duration-fast)]',
                selectionMode || isChecked
                  ? 'opacity-0'
                  : 'opacity-100 group-hover:opacity-0',
              )}
            >
              <SenderAvatar name={senderName} email={senderEmail} size="sm" />
            </div>
            <div
              className={cn(
                'absolute inset-0 flex items-center justify-center transition-opacity duration-[var(--duration-fast)]',
                selectionMode || isChecked
                  ? 'opacity-100'
                  : 'opacity-0 group-hover:opacity-100',
              )}
            >
              <Checkbox
                checked={isChecked}
                onCheckedChange={() => onToggleCheck(email.id)}
                onClick={(e) => e.stopPropagation()}
                aria-label={senderName}
              />
            </div>
          </div>
        </div>

        {/* Content */}
        <div className="min-w-0 flex-1">
          {/* Row 1: Sender + Date */}
          <div className="flex items-center justify-between gap-2">
            <span
              className={cn(
                'truncate text-[13.5px] leading-snug',
                isUnread
                  ? 'font-semibold text-[var(--ink-primary)]'
                  : 'font-medium text-[var(--ink-secondary)]',
              )}
            >
              {senderName}
            </span>
            <time
              dateTime={displayDate}
              className={cn(
                'shrink-0 text-[11.5px] tabular-nums',
                isUnread
                  ? 'font-medium text-[var(--accent-ink)]'
                  : 'text-[var(--ink-muted)]',
              )}
            >
              {formatMailListTime(displayDate, i18n.language)}
            </time>
          </div>

          {/* Row 2: Subject + Badges */}
          <div className="mt-0.5 flex items-center gap-1.5">
            {showAuthWarning && (
              <ShieldAlert
                className="h-3.5 w-3.5 shrink-0 text-[var(--danger)]"
                aria-label={t('mail.security.dangerousTitle')}
              />
            )}
            {email.error && (
              <AlertCircle
                className="h-3.5 w-3.5 shrink-0 text-[var(--warning)]"
                aria-label={t('mail.security.deliveryErrorTitle')}
              />
            )}
            <p
              className={cn(
                'min-w-0 flex-1 truncate text-[13px] leading-snug',
                isUnread
                  ? 'font-medium text-[var(--ink-primary)]'
                  : 'text-[var(--ink-secondary)]',
              )}
            >
              {email.title || t('mail.noSubject')}
            </p>
          </div>

          {/* Row 3: Snippet + Attachment / Star */}
          <div className="mt-0.5 flex items-center justify-between gap-2">
            <p className="min-w-0 flex-1 truncate text-[12px] text-[var(--ink-muted)]">
              {email.desc?.trim() || t('mail.noSnippet')}
            </p>
            <div className="flex shrink-0 items-center gap-1.5">
              {hasAttachments && (
                <Paperclip className="h-3.5 w-3.5 text-[var(--ink-muted)]" />
              )}
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation()
                  onToggleStar(email.id, !isStarred)
                }}
                aria-label={isStarred ? t('mail.actions.unstar') : t('mail.actions.star')}
                className={cn(
                  'inline-flex h-6 w-6 items-center justify-center rounded transition-opacity',
                  isStarred
                    ? 'opacity-100'
                    : 'opacity-0 group-hover:opacity-100 focus-visible:opacity-100',
                )}
              >
                <Star
                  className={cn(
                    'h-3.5 w-3.5 transition-colors',
                    isStarred
                      ? 'fill-[var(--star-active)] text-[var(--star-active)]'
                      : 'text-[var(--ink-muted)] hover:text-[var(--star-active)]',
                  )}
                />
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
})
