import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { AlertCircle, ImageOff, Paperclip } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { formatBytes, parseAddressList } from '@/lib/format'
import { SandboxedHtmlFrame } from '@/features/mail/SandboxedHtmlFrame'
import { PlainTextMailView } from '@/features/mail/PlainTextMailView'

/**
 * Telegram Mini App mail view. It never uses the login session: the server
 * checks Telegram's signed launch data, then returns the mail and a short-lived
 * ticket that only loads this mail's images and attachments.
 */

interface WebAppAttachment {
  index: number
  filename: string
  content_type: string
  size: number
  url: string
}

interface WebAppMail {
  id: number
  subject: string
  from_name: string
  from_address: string
  to: string
  date: string
  html: string
  text: string
  attachments: WebAppAttachment[]
  ticket: string
}

type State =
  | { kind: 'loading' }
  | { kind: 'error'; message: string }
  | { kind: 'ready'; mail: WebAppMail }

const INIT_DATA_KEY = 'pmail_tg_init_data'

/** Telegram passes the signed launch data in the URL fragment. */
function readInitData(): string {
  const fromSdk = (window as { Telegram?: { WebApp?: { initData?: string } } }).Telegram?.WebApp
    ?.initData
  if (fromSdk) return fromSdk
  const fromHash = new URLSearchParams(window.location.hash.slice(1)).get('tgWebAppData') ?? ''
  try {
    if (fromHash) sessionStorage.setItem(INIT_DATA_KEY, fromHash)
    return fromHash || sessionStorage.getItem(INIT_DATA_KEY) || ''
  } catch {
    return fromHash
  }
}

export function TelegramMailPage() {
  const { t, i18n } = useTranslation()
  const [state, setState] = useState<State>({ kind: 'loading' })
  const [showImages, setShowImages] = useState(false)
  const [hasExternal, setHasExternal] = useState(false)

  const mailId = Number(new URLSearchParams(window.location.search).get('mail'))

  useEffect(() => {
    const initData = readInitData()
    if (!initData || !Number.isInteger(mailId) || mailId <= 0) {
      setState({ kind: 'error', message: t('telegramApp.openFromTelegram') })
      return
    }
    const controller = new AbortController()
    void (async () => {
      try {
        const res = await fetch('/api/telegram/webapp/mail', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-PMail-Lang': i18n.language },
          body: JSON.stringify({ init_data: initData, id: mailId }),
          credentials: 'omit',
          signal: controller.signal,
        })
        const body = (await res.json().catch(() => null)) as {
          code?: number
          errorMsg?: string
          data?: WebAppMail
        } | null
        if (!res.ok || !body?.data) {
          setState({
            kind: 'error',
            message: body?.errorMsg || t('telegramApp.loadFailed'),
          })
          return
        }
        setState({ kind: 'ready', mail: body.data })
      } catch (err) {
        if (!controller.signal.aborted) {
          setState({ kind: 'error', message: t('telegramApp.loadFailed') })
          console.warn(err)
        }
      }
    })()
    return () => controller.abort()
  }, [mailId, t, i18n.language])

  const ticket = state.kind === 'ready' ? state.mail.ticket : ''
  const imageUrl = useCallback(
    (url: string) => {
      const absolute = url.trim().startsWith('//') ? `https:${url.trim()}` : url.trim()
      return `/api/telegram/webapp/image/${mailId}?k=${encodeURIComponent(ticket)}&u=${encodeURIComponent(absolute)}`
    },
    [mailId, ticket],
  )

  const recipients = useMemo(
    () =>
      state.kind === 'ready'
        ? parseAddressList(state.mail.to)
            .map((a) => a.name || a.email)
            .join(', ')
        : '',
    [state],
  )

  if (state.kind === 'loading') {
    return (
      <main className="min-h-dvh bg-[var(--surface-canvas)] px-4 py-5 space-y-3" aria-busy="true">
        <Skeleton className="h-6 w-3/4 rounded" />
        <Skeleton className="h-4 w-1/2 rounded" />
        <Skeleton className="h-40 w-full rounded" />
      </main>
    )
  }

  if (state.kind === 'error') {
    return (
      <main className="flex min-h-dvh items-start justify-center bg-[var(--surface-canvas)] px-4 py-10">
        <div
          role="alert"
          className="flex max-w-md items-start gap-2 rounded-[var(--radius-md)] border border-[var(--danger)]/30 bg-[var(--danger-soft)] px-4 py-3 text-[13.5px] text-[var(--danger)]"
        >
          <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
          <span>{state.message}</span>
        </div>
      </main>
    )
  }

  const { mail } = state
  const from = mail.from_name ? `${mail.from_name} <${mail.from_address}>` : mail.from_address
  const date = new Date(mail.date)

  return (
    <main className="min-h-dvh bg-[var(--surface-canvas)] px-4 pb-8 pt-4">
      <h1 className="text-[18px] font-semibold leading-snug text-[var(--ink-primary)] break-words">
        {mail.subject || t('mail.noSubject')}
      </h1>
      <div className="mt-2 space-y-0.5 text-[12.5px] text-[var(--ink-secondary)] [overflow-wrap:anywhere]">
        <p>
          <span className="text-[var(--ink-muted)]">{t('telegramApp.from')}</span> {from}
        </p>
        {recipients && (
          <p>
            <span className="text-[var(--ink-muted)]">{t('telegramApp.to')}</span> {recipients}
          </p>
        )}
        {!Number.isNaN(date.getTime()) && (
          <p className="text-[var(--ink-muted)]">{date.toLocaleString(i18n.language)}</p>
        )}
      </div>

      {mail.attachments.length > 0 && (
        <ul className="mt-3 flex flex-wrap gap-2" aria-label={t('telegramApp.attachments')}>
          {mail.attachments.map((a) => (
            <li key={a.index}>
              <a
                href={a.url}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex max-w-full items-center gap-1.5 rounded-[var(--radius-sm)] border border-[var(--border-hairline)] px-2.5 py-1.5 text-[12.5px] text-[var(--ink-primary)] hover:bg-[var(--surface-hover)]"
              >
                <Paperclip className="h-3.5 w-3.5 shrink-0 text-[var(--ink-muted)]" />
                <span className="truncate">{a.filename || `#${a.index + 1}`}</span>
                {a.size > 0 && (
                  <span className="shrink-0 text-[var(--ink-muted)]">{formatBytes(a.size)}</span>
                )}
              </a>
            </li>
          ))}
        </ul>
      )}

      {mail.html && hasExternal && !showImages && (
        <div className="mt-3 flex flex-wrap items-center justify-between gap-2 rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)] px-3 py-2 text-[12.5px] text-[var(--ink-secondary)]">
          <span className="inline-flex items-center gap-1.5">
            <ImageOff className="h-3.5 w-3.5 shrink-0" />
            {t('mail.security.externalImagesBlocked')}
          </span>
          <Button type="button" size="xs" variant="secondary" onClick={() => setShowImages(true)}>
            {t('mail.security.loadExternalImages')}
          </Button>
        </div>
      )}

      <div className="mt-4 border-t border-[var(--border-hairline)] pt-4">
        {mail.html ? (
          <SandboxedHtmlFrame
            html={mail.html}
            emailId={mail.id}
            allowExternalImages={showImages}
            onDetectExternalImages={setHasExternal}
            imageUrl={imageUrl}
          />
        ) : (
          <PlainTextMailView text={mail.text || t('mail.noSnippet')} />
        )}
      </div>
    </main>
  )
}

export default TelegramMailPage
