import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  AlertTriangle,
  ArrowLeft,
  Check,
  ChevronDown,
  ChevronUp,
  Copy,
  FolderInput,
  Forward,
  Image as ImageIcon,
  Inbox,
  KeyRound,
  Languages,
  ListChecks,
  Mail,
  MailOpen,
  MoreHorizontal,
  Printer,
  Reply,
  ReplyAll,
  RotateCcw,
  ShieldAlert,
  ShieldBan,
  ShieldCheck,
  Sparkles,
  Star,
  Trash2,
} from 'lucide-react'
import {
  API_ERROR_NO,
  ApiError,
  mailApi,
  SYSTEM_GROUP_IDS,
  type EmailAIExtractResult,
  type EmailAISummaryResult,
  type EmailDetail,
  type EmailTranslationResult,
  type GroupOption,
  type TranslationEngine,
  type TranslationLanguage,
} from '@/api'
import {
  useEmailDetailQuery,
  useUserAIQuery,
  useUserTranslationPrefsQuery,
} from '@/hooks/useMailQueries'
import { formatMailFullTime, formatRelativeTime, pickDate } from '@/lib/date'
import { formatBytes, resolveAddressList, resolveAttachmentsList } from '@/lib/format'
import { cn } from '@/lib/cn'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { SenderAvatar } from '@/components/common/SenderAvatar'
import { isSenderAuthVerified } from '@/lib/brandAvatars'
import { usePreferences } from '@/lib/preferences'
import { SandboxedHtmlFrame } from './SandboxedHtmlFrame'
import { PlainTextMailView } from './PlainTextMailView'
import { AttachmentSection } from './AttachmentSection'
import { MailTranslationCard } from './MailTranslationCard'
import { MailAIPanel } from './MailAIPanel'
import { computePurgeDaysLeft } from './trashHelpers'
import {
  formatAuthReason,
  formatAuthResultLabel,
  formatDmarcPolicyLabel,
  getAuthBadgeVariant,
} from './authResults'
import { formatRiskFindingTitle, formatSpamReason } from './inboundAnalysis'
import type { FolderSelection } from './types'

interface ReadingPaneProps {
  emailId: number | null
  folder: FolderSelection
  unreadInboxCount: number
  isStarred: boolean
  isReadFallback?: boolean
  groupOptions: GroupOption[]
  isMobile: boolean
  onBack?: () => void
  onReply: (email: EmailDetail, allowExternalImages?: boolean) => void
  onReplyAll: (email: EmailDetail, allowExternalImages?: boolean) => void
  onForward: (email: EmailDetail, allowExternalImages?: boolean) => void
  onEditDraft?: (email: EmailDetail) => void
  onToggleStar: (id: number, nextStarred: boolean) => void
  onToggleRead: (id: number, nextRead: boolean) => void
  onDelete: (id: number, forced: boolean) => void
  onRestore?: (id: number) => void
  onMove: (id: number, groupId: number, label: string) => void
  onMarkSpam: (id: number, isSpam: boolean) => void
}

const TRANSLATE_LANGUAGES: TranslationLanguage[] = [
  'zh-CN',
  'zh-TW',
  'en',
  'ja',
  'ko',
  'fr',
  'de',
  'es',
  'ru',
]

export function ReadingPane({
  emailId,
  folder,
  unreadInboxCount,
  isStarred,
  isReadFallback = true,
  groupOptions,
  isMobile,
  onBack,
  onReply,
  onReplyAll,
  onForward,
  onEditDraft,
  onToggleStar,
  onToggleRead,
  onDelete,
  onRestore,
  onMove,
  onMarkSpam,
}: ReadingPaneProps) {
  const { t, i18n } = useTranslation()
  const { data: email, isLoading, isError, error, refetch } = useEmailDetailQuery(emailId)
  const { data: translationPrefs } = useUserTranslationPrefsQuery(true)
  const { data: userAI } = useUserAIQuery(true)

  const [detailsExpanded, setDetailsExpanded] = useState(false)
  const [hasExternalImages, setHasExternalImages] = useState(false)
  const [externalImagesAllowed, setExternalImagesAllowed] = useState(false)
  const [codeCopied, setCodeCopied] = useState(false)
  const [expandedRiskExcerpts, setExpandedRiskExcerpts] = useState<Set<number>>(new Set())
  const [confirmHardDelOpen, setConfirmHardDelOpen] = useState(false)

  // Translation state
  const [selectedLang, setSelectedLang] = useState<TranslationLanguage | ''>('')
  const [selectedEngine, setSelectedEngine] = useState<TranslationEngine | ''>('')
  const [translationResult, setTranslationResult] = useState<EmailTranslationResult | null>(null)
  const [translationLoading, setTranslationLoading] = useState(false)
  const [translationError, setTranslationError] = useState<string | null>(null)
  const [showTranslation, setShowTranslation] = useState(true)
  const autoTranslatedIdRef = useRef<number | null>(null)

  // AI Summary & Extract state
  const [summaryResult, setSummaryResult] = useState<EmailAISummaryResult | null>(null)
  const [summaryLoading, setSummaryLoading] = useState(false)
  const [summaryError, setSummaryError] = useState<string | null>(null)
  const [extractResult, setExtractResult] = useState<EmailAIExtractResult | null>(null)
  const [extractLoading, setExtractLoading] = useState(false)
  const [extractError, setExtractError] = useState<string | null>(null)
  const autoSummarizedIdRef = useRef<number | null>(null)

  const effectiveTargetLang: TranslationLanguage = useMemo(() => {
    if (selectedLang) return selectedLang
    if (translationPrefs?.target_lang) return translationPrefs.target_lang
    return i18n.language.toLowerCase().startsWith('zh') ? 'zh-CN' : 'en'
  }, [selectedLang, translationPrefs?.target_lang, i18n.language])

  const effectiveEngine: TranslationEngine | undefined = useMemo(() => {
    if (selectedEngine) return selectedEngine
    if (translationPrefs?.translate_engine) return translationPrefs.translate_engine
    return undefined
  }, [selectedEngine, translationPrefs?.translate_engine])

  // Drop late translation/AI responses that belong to a previously opened message.
  const currentEmailIdRef = useRef(emailId)
  currentEmailIdRef.current = emailId

  // Reset per-message ephemeral states when switching messages
  useEffect(() => {
    setDetailsExpanded(false)
    setHasExternalImages(false)
    setExternalImagesAllowed(false)
    setCodeCopied(false)
    setExpandedRiskExcerpts(new Set())
    setConfirmHardDelOpen(false)
    setTranslationResult(null)
    setTranslationLoading(false)
    setTranslationError(null)
    setShowTranslation(true)
    setSummaryResult(null)
    setSummaryLoading(false)
    setSummaryError(null)
    setExtractResult(null)
    setExtractLoading(false)
    setExtractError(null)
    autoTranslatedIdRef.current = null
    autoSummarizedIdRef.current = null
  }, [emailId])

  const runTranslate = useCallback(
    async (opts?: {
      targetLang?: TranslationLanguage
      engine?: TranslationEngine
      force?: boolean
    }) => {
      if (!email) return
      const target = opts?.targetLang ?? effectiveTargetLang
      const engine = opts?.engine ?? effectiveEngine
      setTranslationLoading(true)
      setTranslationError(null)
      setShowTranslation(true)
      try {
        const res = await mailApi.translate({
          id: email.id,
          target_lang: target,
          ...(engine ? { engine } : {}),
          ...(opts?.force ? { force: true } : {}),
        })
        if (currentEmailIdRef.current !== email.id) return
        setTranslationResult(res)
      } catch (err) {
        if (currentEmailIdRef.current !== email.id) return
        const msg = err instanceof Error ? err.message : t('common.unknownError')
        setTranslationError(msg)
      } finally {
        if (currentEmailIdRef.current === email.id) setTranslationLoading(false)
      }
    },
    [email, effectiveTargetLang, effectiveEngine, t],
  )

  const runAI = useCallback(
    async (action: 'summary' | 'extract', force = false) => {
      if (!email) return
      if (action === 'summary') {
        setSummaryLoading(true)
        setSummaryError(null)
      } else {
        setExtractLoading(true)
        setExtractError(null)
      }
      try {
        if (action === 'summary') {
          const res = await mailApi.analyzeAI<EmailAISummaryResult>({
            id: email.id,
            action: 'summary',
            lang: effectiveTargetLang,
            ...(force ? { force: true } : {}),
          })
          if (currentEmailIdRef.current !== email.id) return
          setSummaryResult(res)
        } else {
          const res = await mailApi.analyzeAI<EmailAIExtractResult>({
            id: email.id,
            action: 'extract',
            lang: effectiveTargetLang,
            ...(force ? { force: true } : {}),
          })
          if (currentEmailIdRef.current !== email.id) return
          setExtractResult(res)
        }
      } catch (err) {
        if (currentEmailIdRef.current !== email.id) return
        const msg = err instanceof Error ? err.message : t('common.unknownError')
        if (action === 'summary') setSummaryError(msg)
        else setExtractError(msg)
      } finally {
        if (currentEmailIdRef.current === email.id) {
          if (action === 'summary') setSummaryLoading(false)
          else setExtractLoading(false)
        }
      }
    },
    [email, effectiveTargetLang, t],
  )

  // Auto-translate when user enabled auto_translate and email needs_translation
  useEffect(() => {
    if (
      email &&
      email.translation_available &&
      email.needs_translation &&
      translationPrefs?.auto_translate &&
      autoTranslatedIdRef.current !== email.id
    ) {
      autoTranslatedIdRef.current = email.id
      void runTranslate()
    }
  }, [email, translationPrefs?.auto_translate, runTranslate])

  // Auto-summarize when user enabled ai_auto_summary and email ai_available
  useEffect(() => {
    if (
      email &&
      email.ai_available &&
      translationPrefs?.ai_auto_summary &&
      autoSummarizedIdRef.current !== email.id
    ) {
      autoSummarizedIdRef.current = email.id
      void runAI('summary')
    }
  }, [email, translationPrefs?.ai_auto_summary, runAI])

  const copyVerificationCode = useCallback(
    async (code: string) => {
      try {
        await navigator.clipboard.writeText(code)
        setCodeCopied(true)
        toast.success(t('mail.verificationCode.copiedWithCode', { code }))
        window.setTimeout(() => setCodeCopied(false), 1800)
      } catch {
        toast.error(t('common.copyFailed'))
      }
    },
    [t],
  )

  // Shift+C shortcut in detail view to copy verification code
  useEffect(() => {
    const code = email?.verification_code?.code
    if (!code) return
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (!e.shiftKey || e.key.toLowerCase() !== 'c') return
      const target = e.target as HTMLElement | null
      if (
        target &&
        (target.tagName === 'INPUT' ||
          target.tagName === 'TEXTAREA' ||
          target.tagName === 'SELECT' ||
          target.isContentEditable)
      ) {
        return
      }
      e.preventDefault()
      void copyVerificationCode(code)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [email?.verification_code?.code, copyVerificationCode])

  const handleDetectExternal = useCallback((detected: boolean) => {
    setHasExternalImages(detected)
  }, [])

  const toList = useMemo(
    () => resolveAddressList(email?.to_list, email?.to),
    [email?.to_list, email?.to],
  )
  const ccList = useMemo(
    () => resolveAddressList(email?.cc_list, email?.cc),
    [email?.cc_list, email?.cc],
  )
  const bccList = useMemo(
    () => resolveAddressList(email?.bcc_list, email?.bcc),
    [email?.bcc_list, email?.bcc],
  )
  const replyToList = useMemo(
    () => resolveAddressList(email?.reply_to_list, email?.reply_to),
    [email?.reply_to_list, email?.reply_to],
  )
  const normalizedAttachments = useMemo(
    () => resolveAttachmentsList(email?.attachments_list, email?.attachments),
    [email?.attachments_list, email?.attachments],
  )

  const isInTrash = folder.kind === 'system' && folder.key === 'trash'
  const isInJunk = folder.kind === 'system' && folder.key === 'junk'
  // Auto-loading (always through the image proxy) is skipped for junk and
  // risky mail; the reader can still load images there by hand.
  const { autoLoadImages } = usePreferences()
  const autoImagesAllowed =
    autoLoadImages &&
    !isInJunk &&
    email?.spam?.verdict !== 'spam' &&
    email?.risk?.level !== 'high' &&
    email?.risk?.level !== 'medium'
  const imagesShown = externalImagesAllowed || autoImagesAllowed

  const isDraftOrSentFolder =
    folder.kind === 'system' && (folder.key === 'drafts' || folder.key === 'sent')
  const isDangerous = (() => {
    if (!email) return false
    // Drafts (status=4) and outbound/sent messages (status=1 or type=1) never show inbound spoof warnings
    if (isDraftOrSentFolder || email.status === 4 || email.status === 1 || email.type === 1) {
      return false
    }
    if (email.auth_status !== undefined) {
      return email.auth_status === 'fail'
    }
    return email.spf_check === 0 && email.dkim_check === 0
  })()

  // Empty state (no message selected)
  if (!emailId) {
    return (
      <div className="flex h-full flex-col items-center justify-center bg-[var(--surface-reading)] px-8 text-center">
        <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-[var(--surface-subtle)] text-[var(--ink-muted)]">
          <Inbox className="h-6 w-6 stroke-[1.6]" />
        </div>
        <h2 className="mt-4 text-[15px] font-semibold text-[var(--ink-primary)]">
          {t('mail.emptyReadingTitle')}
        </h2>
        <p className="mt-1.5 max-w-sm text-[13px] leading-relaxed text-[var(--ink-muted)]">
          {t('mail.emptyReadingDesc')}
        </p>
        <div className="mt-5 inline-flex items-center gap-2 rounded-full border border-[var(--border-hairline)] bg-[var(--surface-subtle)] px-3.5 py-1.5 text-[12px] text-[var(--ink-secondary)]">
          <span
            className={cn(
              'h-2 w-2 rounded-full',
              unreadInboxCount > 0 ? 'bg-[var(--unread-dot)]' : 'bg-[var(--success)]',
            )}
          />
          {unreadInboxCount > 0
            ? t('mail.unreadSummary', { count: unreadInboxCount })
            : t('mail.allCaughtUp')}
        </div>
      </div>
    )
  }

  // Instant skeleton (<100ms perceived transition)
  if (isLoading) {
    return (
      <div className="flex h-full flex-col bg-[var(--surface-reading)]">
        <div className="flex h-12 shrink-0 items-center justify-between border-b border-[var(--border-hairline)] px-4">
          <div className="flex items-center gap-2">
            {isMobile && onBack && (
              <Button variant="ghost" size="iconSm" onClick={onBack} aria-label={t('common.back')}>
                <ArrowLeft className="h-4 w-4" />
              </Button>
            )}
            <Skeleton className="h-7 w-20" />
            <Skeleton className="h-7 w-20" />
            <Skeleton className="h-7 w-20" />
          </div>
          <div className="flex items-center gap-1.5">
            <Skeleton className="h-7 w-7" />
            <Skeleton className="h-7 w-7" />
          </div>
        </div>
        <div className="flex-1 space-y-5 p-6">
          <Skeleton className="h-7 w-3/4" />
          <div className="flex items-center gap-3">
            <Skeleton className="h-10 w-10 rounded-full" />
            <div className="space-y-1.5">
              <Skeleton className="h-4 w-44" />
              <Skeleton className="h-3.5 w-64" />
            </div>
          </div>
          <div className="space-y-2.5 pt-4">
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-11/12" />
            <Skeleton className="h-4 w-4/5" />
            <Skeleton className="h-4 w-3/5" />
          </div>
        </div>
      </div>
    )
  }

  if (isError || !email) {
    const isForbidden =
      error instanceof ApiError &&
      (error.status === 403 || error.errorNo === API_ERROR_NO.NO_ACCESS)

    return (
      <div className="flex h-full flex-col items-center justify-center bg-[var(--surface-reading)] px-6 text-center">
        {isForbidden ? (
          <>
            <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-[var(--danger-soft)] text-[var(--danger)]">
              <ShieldAlert className="h-5 w-5" />
            </div>
            <p className="mt-3 text-[15px] font-semibold text-[var(--ink-primary)]">
              {t('mail.security.detailForbiddenTitle')}
            </p>
            <p className="mt-1.5 max-w-md text-[13px] leading-relaxed text-[var(--ink-muted)]">
              {t('mail.security.detailForbiddenDesc')}
            </p>
            {isMobile && onBack && (
              <div className="mt-4">
                <Button variant="outline" size="sm" onClick={onBack}>
                  {t('common.back')}
                </Button>
              </div>
            )}
          </>
        ) : (
          <>
            <p className="text-[14px] font-medium text-[var(--ink-primary)]">
              {t('mail.detailError')}
            </p>
            <div className="mt-3 flex items-center gap-2">
              {isMobile && onBack && (
                <Button variant="outline" size="sm" onClick={onBack}>
                  {t('common.back')}
                </Button>
              )}
              <Button variant="secondary" size="sm" onClick={() => void refetch()}>
                {t('app.retry')}
              </Button>
            </div>
          </>
        )}
      </div>
    )
  }

  const isCurrentlyRead = email.is_read !== undefined ? email.is_read === 1 : isReadFallback
  const effectiveStarred = email.is_starred !== undefined ? email.is_starred : isStarred
  const isServerDraft = email.status === 4
  const displayDate = pickDate(
    email.send_date_iso,
    email.send_time_iso,
    email.create_time_iso,
    email.update_time_iso,
    email.send_date,
    email.send_time,
    email.create_time,
    email.update_time,
  )
  const hasHtml = Boolean(email.html && email.html.trim().length > 0)

  const handleMoveMenu = (groupId: number, label: string) => {
    onMove(email.id, groupId, label)
  }

  return (
    <div className="flex h-full flex-col bg-[var(--surface-reading)]">
      {/* Sticky Toolbar */}
      <header className="flex h-12 shrink-0 items-center justify-between gap-2 border-b border-[var(--border-hairline)] bg-[var(--surface-reading)]/95 px-3 backdrop-blur-sm sm:px-5 pt-safe">
        <div className="flex items-center gap-1">
          {isMobile && onBack && (
            <Button
              variant="ghost"
              size="iconSm"
              onClick={onBack}
              aria-label={t('common.back')}
              className="mr-1"
            >
              <ArrowLeft className="h-4 w-4" />
            </Button>
          )}

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => onReply(email, externalImagesAllowed)}
              >
                <Reply className="h-3.5 w-3.5" />
                <span className="hidden sm:inline">{t('mail.actions.reply')}</span>
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t('mail.actions.reply')} (r)</TooltipContent>
          </Tooltip>

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                className="hidden sm:inline-flex"
                onClick={() => onReplyAll(email, externalImagesAllowed)}
              >
                <ReplyAll className="h-3.5 w-3.5" />
                <span className="hidden xl:inline">{t('mail.actions.replyAll')}</span>
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t('mail.actions.replyAll')} (a)</TooltipContent>
          </Tooltip>

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                className="hidden sm:inline-flex"
                onClick={() => onForward(email, externalImagesAllowed)}
              >
                <Forward className="h-3.5 w-3.5" />
                <span className="hidden xl:inline">{t('mail.actions.forward')}</span>
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t('mail.actions.forward')} (f)</TooltipContent>
          </Tooltip>
        </div>

        <div className="flex items-center gap-1">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="iconSm"
                onClick={() => onToggleStar(email.id, !effectiveStarred)}
                aria-label={effectiveStarred ? t('mail.actions.unstar') : t('mail.actions.star')}
              >
                <Star
                  className={cn(
                    'h-4 w-4 transition-colors',
                    effectiveStarred
                      ? 'fill-[var(--star-active)] text-[var(--star-active)]'
                      : 'text-[var(--ink-muted)]',
                  )}
                />
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              {effectiveStarred ? t('mail.actions.unstar') : t('mail.actions.star')} (s)
            </TooltipContent>
          </Tooltip>

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="iconSm"
                onClick={() => onToggleRead(email.id, !isCurrentlyRead)}
                aria-label={
                  isCurrentlyRead ? t('mail.actions.markUnread') : t('mail.actions.markRead')
                }
              >
                {isCurrentlyRead ? (
                  <Mail className="h-4 w-4" />
                ) : (
                  <MailOpen className="h-4 w-4" />
                )}
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              {isCurrentlyRead ? t('mail.actions.markUnread') : t('mail.actions.markRead')}
            </TooltipContent>
          </Tooltip>

          {/* Move to folder dropdown */}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="iconSm"
                aria-label={t('mail.actions.move')}
                title={t('mail.actions.move')}
              >
                <FolderInput className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuLabel>{t('mail.actions.move')}</DropdownMenuLabel>
              <DropdownMenuItem
                onClick={() => handleMoveMenu(SYSTEM_GROUP_IDS.INBOX, t('nav.inbox'))}
              >
                {t('nav.inbox')}
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() => handleMoveMenu(SYSTEM_GROUP_IDS.JUNK, t('nav.junk'))}
              >
                {t('nav.junk')}
              </DropdownMenuItem>
              {groupOptions
                .filter((g) => g.id < SYSTEM_GROUP_IDS.INBOX)
                .map((g) => (
                  <DropdownMenuItem key={g.id} onClick={() => handleMoveMenu(g.id, g.name)}>
                    {g.name}
                  </DropdownMenuItem>
                ))}
            </DropdownMenuContent>
          </DropdownMenu>

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="iconSm"
                onClick={() => onMarkSpam(email.id, !isInJunk)}
                aria-label={isInJunk ? t('mail.actions.notSpam') : t('mail.actions.markSpam')}
              >
                {isInJunk ? (
                  <ShieldCheck className="h-4 w-4 text-[var(--success)]" />
                ) : (
                  <ShieldBan className="h-4 w-4" />
                )}
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              {isInJunk ? t('mail.actions.notSpam') : t('mail.actions.markSpam')}
            </TooltipContent>
          </Tooltip>

          {/* Translate Dropdown (only when translation_available) */}
          {email.translation_available && (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant={translationResult ? 'secondary' : 'ghost'}
                  size="xs"
                  aria-label={t('translation.button')}
                  title={t('translation.button')}
                >
                  <Languages className="h-3.5 w-3.5" />
                  <span className="hidden sm:inline">{t('translation.button')}</span>
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-52">
                {email.translation_engines && email.translation_engines.length > 1 && (
                  <>
                    <DropdownMenuLabel>{t('translation.chooseEngine')}</DropdownMenuLabel>
                    {(['web', 'ai'] as const).map((eng) => {
                      const active = (effectiveEngine ?? 'web') === eng
                      return (
                        <DropdownMenuItem
                          key={eng}
                          onClick={() => {
                            setSelectedEngine(eng)
                            void runTranslate({ engine: eng })
                          }}
                          className="justify-between"
                        >
                          <span>{t(`translation.engines.${eng}`)}</span>
                          {active && <Check className="h-3.5 w-3.5 text-[var(--accent)]" />}
                        </DropdownMenuItem>
                      )
                    })}
                    <DropdownMenuSeparator />
                  </>
                )}
                <DropdownMenuLabel>{t('translation.chooseLanguage')}</DropdownMenuLabel>
                {TRANSLATE_LANGUAGES.map((lang) => {
                  const active = effectiveTargetLang === lang
                  return (
                    <DropdownMenuItem
                      key={lang}
                      onClick={() => {
                        setSelectedLang(lang)
                        void runTranslate({ targetLang: lang })
                      }}
                      className="justify-between"
                    >
                      <span>{t(`translation.languages.${lang}`)}</span>
                      {active && <Check className="h-3.5 w-3.5 text-[var(--accent)]" />}
                    </DropdownMenuItem>
                  )
                })}
              </DropdownMenuContent>
            </DropdownMenu>
          )}

          {/* AI Summary & Extract Buttons */}
          {email.ai_available && (
            <>
              <Button
                variant={summaryResult ? 'secondary' : 'ghost'}
                size="xs"
                disabled={summaryLoading}
                onClick={() => void runAI('summary')}
                title={t('ai.summaryButton')}
              >
                <Sparkles className="h-3.5 w-3.5 text-[var(--accent)]" />
                <span className="hidden md:inline">{t('ai.summaryButton')}</span>
              </Button>
              <Button
                variant={extractResult ? 'secondary' : 'ghost'}
                size="xs"
                disabled={extractLoading}
                onClick={() => void runAI('extract')}
                title={t('ai.extractButton')}
              >
                <ListChecks className="h-3.5 w-3.5 text-[var(--accent)]" />
                <span className="hidden md:inline">{t('ai.extractButton')}</span>
              </Button>
            </>
          )}

          {/* Restore button when in Trash */}
          {isInTrash && onRestore && (
            <Button
              variant="secondary"
              size="xs"
              onClick={() => onRestore(email.id)}
              title={`${t('mail.trash.restore')} (z / u)`}
            >
              <RotateCcw className="h-3.5 w-3.5" />
              <span>{t('mail.trash.restore')}</span>
            </Button>
          )}

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="dangerGhost"
                size="iconSm"
                onClick={() => {
                  if (isInTrash) setConfirmHardDelOpen(true)
                  else onDelete(email.id, false)
                }}
                aria-label={
                  isInTrash ? t('mail.actions.deleteForever') : t('mail.actions.delete')
                }
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              {isInTrash ? t('mail.actions.deleteForever') : t('mail.actions.delete')} (#)
            </TooltipContent>
          </Tooltip>

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="iconSm" aria-label={t('common.more')}>
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={() => window.print()}>
                <Printer className="h-3.5 w-3.5 text-[var(--ink-muted)]" />
                {t('mail.actions.print')}
              </DropdownMenuItem>
              {isInTrash && (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    onClick={() => handleMoveMenu(SYSTEM_GROUP_IDS.INBOX, t('nav.inbox'))}
                  >
                    <Inbox className="h-3.5 w-3.5 text-[var(--ink-muted)]" />
                    {t('mail.actions.restoreToInbox')}
                  </DropdownMenuItem>
                </>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </header>

      {/* Scrollable Email Content */}
      <article className="mail-scroll flex-1 overflow-y-auto px-4 py-5 sm:px-7 sm:py-6">
        <div className="mx-auto max-w-3xl">
          {/* Subject */}
          <h1 className="text-[20px] font-semibold leading-snug tracking-tight text-[var(--ink-primary)] sm:text-[22px]">
            {email.subject || t('mail.noSubject')}
          </h1>

          {/* Verification Code Bar */}
          {email.antivirus?.status === 'not_scanned' && (
            <p className="mb-3 rounded-[var(--radius-sm)] bg-[var(--warning-soft)] px-3 py-2 text-[12px] text-[var(--warning)]">
              {t('mail.risk.antivirusNotScanned')}
            </p>
          )}
          {email.verification_code?.code && (
            <div className="mt-4 flex flex-wrap items-center justify-between gap-3 rounded-[var(--radius-md)] border border-[var(--accent)]/30 bg-[var(--accent-soft)]/70 px-4 py-3">
              <div className="flex flex-wrap items-center gap-3 min-w-0">
                <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-[var(--radius-sm)] bg-[var(--accent)]/15 text-[var(--accent)]">
                  <KeyRound className="h-4.5 w-4.5" />
                </div>
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-[12px] font-medium text-[var(--ink-secondary)]">
                      {email.verification_code.confidence < 0.6
                        ? t('mail.verificationCode.possibleLabel')
                        : t('mail.verificationCode.label')}
                    </span>
                    <span className="hidden text-[11px] text-[var(--ink-muted)] sm:inline">
                      · {t('mail.verificationCode.shortcutHint')}
                    </span>
                  </div>
                  <p className="mt-0.5 font-mono text-[22px] font-bold leading-tight tracking-[0.2em] text-[var(--ink-primary)] select-all">
                    {email.verification_code.code}
                  </p>
                </div>
              </div>
              <Button
                type="button"
                variant="primary"
                size="sm"
                onClick={() => void copyVerificationCode(email.verification_code!.code)}
                aria-label={t('mail.verificationCode.copyAria', {
                  code: email.verification_code.code,
                })}
              >
                {codeCopied ? (
                  <Check className="h-3.5 w-3.5" />
                ) : (
                  <Copy className="h-3.5 w-3.5" />
                )}
                {codeCopied
                  ? t('mail.verificationCode.copied')
                  : t('mail.verificationCode.copy')}
              </Button>
            </div>
          )}

          {/* Code / Attachment Risk Banner */}
          {email.risk && (email.risk.level === 'high' || email.risk.level === 'medium') && (
            <div
              className={cn(
                'mt-4 rounded-[var(--radius-md)] border px-3.5 py-3 text-[13px]',
                email.risk.level === 'high'
                  ? 'border-[var(--danger)]/35 bg-[var(--danger-soft)] text-[var(--danger)]'
                  : 'border-[var(--warning)]/35 bg-[var(--warning-soft)] text-[var(--warning)]',
              )}
            >
              <div className="flex items-start gap-2.5">
                <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0" />
                <div className="min-w-0 flex-1 space-y-2">
                  <div>
                    <p className="font-semibold">
                      {email.risk.level === 'high'
                        ? t('mail.risk.highTitle')
                        : t('mail.risk.mediumTitle')}
                    </p>
                    <p className="mt-0.5 text-[12.5px] opacity-90">
                      {email.risk.level === 'high'
                        ? t('mail.risk.highDesc')
                        : t('mail.risk.mediumDesc')}
                    </p>
                  </div>

                  {email.risk.findings.length > 0 && (
                    <ul className="space-y-1.5 border-t border-current/15 pt-2">
                      {email.risk.findings.map((finding, idx) => {
                        const isExcerptOpen = expandedRiskExcerpts.has(idx)
                        return (
                          <li key={idx} className="space-y-1 text-[12.5px]">
                            <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                              <Badge
                                variant={
                                  finding.severity === 'high'
                                    ? 'danger'
                                    : finding.severity === 'medium'
                                      ? 'warning'
                                      : 'default'
                                }
                              >
                                {t(`mail.risk.severity.${finding.severity}`)}
                              </Badge>
                              <span className="font-medium text-[var(--ink-primary)]">
                                {formatRiskFindingTitle(finding, t)}
                              </span>
                              <span className="text-[11.5px] opacity-80">
                                {finding.location === 'attachment' &&
                                typeof finding.attachment_index === 'number'
                                  ? t('mail.risk.locationAttachment', {
                                      index: finding.attachment_index + 1,
                                    })
                                  : t('mail.risk.locationBody')}
                                {typeof finding.line === 'number'
                                  ? ` · ${t('mail.risk.lineLabel', { line: finding.line })}`
                                  : ''}
                              </span>
                              {finding.excerpt && (
                                <button
                                  type="button"
                                  onClick={() =>
                                    setExpandedRiskExcerpts((prev) => {
                                      const next = new Set(prev)
                                      if (next.has(idx)) next.delete(idx)
                                      else next.add(idx)
                                      return next
                                    })
                                  }
                                  className="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[11.5px] font-medium underline underline-offset-2 hover:bg-black/5 dark:hover:bg-white/10"
                                >
                                  {isExcerptOpen
                                    ? t('mail.risk.hideExcerpt')
                                    : t('mail.risk.showExcerpt')}
                                </button>
                              )}
                            </div>
                            {finding.excerpt && isExcerptOpen && (
                              <pre className="mail-scroll overflow-x-auto rounded-[var(--radius-sm)] border border-current/20 bg-[var(--surface-canvas)] px-2.5 py-1.5 font-mono text-[11.5px] text-[var(--ink-primary)] whitespace-pre-wrap break-all">
                                {finding.excerpt}
                              </pre>
                            )}
                          </li>
                        )
                      })}
                    </ul>
                  )}
                </div>
              </div>
            </div>
          )}

          {/* Spam Assessment Banner (shown in Junk folder or when classified as spam) */}
          {(isInJunk || email.spam?.verdict === 'spam') && email.spam && (
            <div className="mt-4 flex items-start gap-2.5 rounded-[var(--radius-md)] border border-[var(--warning)]/35 bg-[var(--warning-soft)] px-3.5 py-2.5 text-[12.5px] text-[var(--warning)]">
              <ShieldBan className="mt-0.5 h-4 w-4 shrink-0" />
              <div className="min-w-0 flex-1 space-y-1.5">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-semibold">{t('mail.spam.bannerTitle')}</span>
                  <Badge variant="warning" className="font-mono">
                    {t('mail.spam.scoreLabel', { score: email.spam.score })}
                  </Badge>
                </div>
                {email.spam.reasons.length > 0 && (
                  <ul className="space-y-0.5 text-[12px] text-[var(--ink-primary)]">
                    {email.spam.reasons.map((reason, idx) => (
                      <li key={idx} className="flex flex-wrap items-center gap-1.5">
                        <span>• {formatSpamReason(reason, t)}</span>
                        <span className="font-mono text-[11px] text-[var(--ink-muted)]">
                          ({reason.weight >= 0 ? `+${reason.weight}` : reason.weight})
                        </span>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            </div>
          )}

          {/* SPF/DKIM security warning */}
          {isDangerous && (
            <div className="mt-4 flex items-start gap-2.5 rounded-[var(--radius-md)] border border-[var(--danger)]/30 bg-[var(--danger-soft)] px-3.5 py-2.5 text-[13px] text-[var(--danger)]">
              <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0" />
              <div>
                <p className="font-medium">{t('mail.security.dangerousTitle')}</p>
                <p className="mt-0.5 text-[12.5px] opacity-90">{t('mail.security.dangerousDesc')}</p>
              </div>
            </div>
          )}

          {/* Delivery error notice */}
          {email.error && (
            <div className="mt-4 flex items-start gap-2.5 rounded-[var(--radius-md)] border border-[var(--warning)]/30 bg-[var(--warning-soft)] px-3.5 py-2.5 text-[13px] text-[var(--warning)]">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
              <div>
                <p className="font-medium">{t('mail.security.deliveryErrorTitle')}</p>
                <p className="mt-0.5 break-all font-mono text-[12px] opacity-90">{email.error}</p>
              </div>
            </div>
          )}

          {/* Sender & Recipients Header */}
          <div className="mt-5 flex items-start gap-3">
            <div className="relative shrink-0">
              <SenderAvatar
                name={email.from_name}
                email={email.from_address}
                size="lg"
                verified={
                  !isServerDraft &&
                  email.status !== 1 &&
                  email.type !== 1 &&
                  isSenderAuthVerified(email.auth_status, email.risk)
                }
              />
              {!isServerDraft &&
                email.status !== 1 &&
                email.type !== 1 &&
                isSenderAuthVerified(email.auth_status, email.risk) && (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span
                        aria-label={t('mail.security.verifiedSenderBadge')}
                        title={t('mail.security.verifiedSenderTooltip')}
                        className="absolute -bottom-0.5 -right-0.5 inline-flex h-4 w-4 items-center justify-center rounded-full border-2 border-[var(--surface-reading)] bg-[var(--success)] text-white"
                      >
                        <Check className="h-2.5 w-2.5 stroke-[3]" />
                      </span>
                    </TooltipTrigger>
                    <TooltipContent>{t('mail.security.verifiedSenderTooltip')}</TooltipContent>
                  </Tooltip>
                )}
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-0.5">
                <div className="flex flex-wrap items-baseline gap-1.5 min-w-0">
                  <span className="text-[14.5px] font-semibold text-[var(--ink-primary)] truncate">
                    {email.from_name || email.from_address || t('mail.unknownSender')}
                  </span>
                  {email.from_address && (
                    <span className="text-[12.5px] text-[var(--ink-muted)] truncate">
                      &lt;{email.from_address}&gt;
                    </span>
                  )}
                </div>
                <time
                  dateTime={displayDate}
                  title={formatMailFullTime(displayDate, i18n.language)}
                  className="shrink-0 text-[12px] tabular-nums text-[var(--ink-muted)]"
                >
                  {formatMailFullTime(displayDate, i18n.language)} ·{' '}
                  {formatRelativeTime(displayDate, i18n.language)}
                </time>
              </div>

              {/* Compact recipients summary + expand button */}
              <div className="mt-1 flex flex-wrap items-center gap-1.5 text-[12.5px] text-[var(--ink-secondary)]">
                <span className="text-[var(--ink-muted)]">{t('mail.header.to')}:</span>
                <span className="truncate">
                  {toList.length > 0
                    ? toList
                        .slice(0, 2)
                        .map((r) => r.name || r.email)
                        .join('、')
                    : t('common.none')}
                  {toList.length > 2 &&
                    ` ${t('mail.header.toCount', { count: toList.length - 2 })}`}
                </span>
                {ccList.length > 0 && (
                  <span className="text-[var(--ink-muted)]">
                    · {t('mail.header.cc')} {ccList.length}
                  </span>
                )}
                <button
                  type="button"
                  onClick={() => setDetailsExpanded((v) => !v)}
                  className="ml-1 inline-flex items-center gap-0.5 rounded-[var(--radius-xs)] px-1.5 py-0.5 text-[11.5px] text-[var(--ink-muted)] transition-colors hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]"
                >
                  {detailsExpanded ? t('mail.header.hideDetails') : t('mail.header.showDetails')}
                  {detailsExpanded ? (
                    <ChevronUp className="h-3 w-3" />
                  ) : (
                    <ChevronDown className="h-3 w-3" />
                  )}
                </button>
              </div>
            </div>
          </div>

          {/* Expandable Full Header Metadata */}
          {detailsExpanded && (
            <dl className="mt-3 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/70 px-3.5 py-3 text-[12.5px]">
              <dt className="text-[var(--ink-muted)]">{t('mail.header.from')}</dt>
              <dd className="min-w-0 break-all text-[var(--ink-primary)]">
                {email.from_name ? `${email.from_name} <${email.from_address}>` : email.from_address}
              </dd>

              <dt className="text-[var(--ink-muted)]">{t('mail.header.to')}</dt>
              <dd className="min-w-0 break-all text-[var(--ink-primary)]">
                {toList.map((r) => (r.name ? `${r.name} <${r.email}>` : r.email)).join(', ') ||
                  t('common.none')}
              </dd>

              {ccList.length > 0 && (
                <>
                  <dt className="text-[var(--ink-muted)]">{t('mail.header.cc')}</dt>
                  <dd className="min-w-0 break-all text-[var(--ink-primary)]">
                    {ccList.map((r) => (r.name ? `${r.name} <${r.email}>` : r.email)).join(', ')}
                  </dd>
                </>
              )}

              {bccList.length > 0 && (
                <>
                  <dt className="text-[var(--ink-muted)]">{t('mail.header.bcc')}</dt>
                  <dd className="min-w-0 break-all text-[var(--ink-primary)]">
                    {bccList.map((r) => (r.name ? `${r.name} <${r.email}>` : r.email)).join(', ')}
                  </dd>
                </>
              )}

              {replyToList.length > 0 && (
                <>
                  <dt className="text-[var(--ink-muted)]">{t('mail.header.replyTo')}</dt>
                  <dd className="min-w-0 break-all text-[var(--ink-primary)]">
                    {replyToList.map((r) => (r.name ? `${r.name} <${r.email}>` : r.email)).join(', ')}
                  </dd>
                </>
              )}

              <dt className="text-[var(--ink-muted)]">{t('mail.header.date')}</dt>
              <dd className="tabular-nums text-[var(--ink-primary)]">
                {formatMailFullTime(displayDate, i18n.language)}
              </dd>

              {email.size > 0 && (
                <>
                  <dt className="text-[var(--ink-muted)]">{t('mail.header.size')}</dt>
                  <dd className="tabular-nums text-[var(--ink-primary)]">
                    {formatBytes(email.size)}
                  </dd>
                </>
              )}

              {email.msg_id && (
                <>
                  <dt className="text-[var(--ink-muted)]">{t('mail.header.messageId')}</dt>
                  <dd className="min-w-0 break-all font-mono text-[11.5px] text-[var(--ink-secondary)]">
                    {email.msg_id}
                  </dd>
                </>
              )}

              {email.auth_results && (
                <>
                  <dt className="border-t border-[var(--border-hairline)] pt-2.5 text-[var(--ink-muted)]">
                    {t('mail.security.authDetails.title')}
                  </dt>
                  <dd className="min-w-0 space-y-2 border-t border-[var(--border-hairline)] pt-2.5">
                    {email.auth_results.trusted_relay && (
                      <div>
                        <Badge variant="accent">
                          {t('mail.security.authDetails.trustedRelay')}
                        </Badge>
                      </div>
                    )}
                    {(
                      [
                        ['SPF', email.auth_results.spf],
                        ['DKIM', email.auth_results.dkim],
                        ['DMARC', email.auth_results.dmarc],
                      ] as const
                    ).map(([label, check]) => {
                      const reasonText = formatAuthReason(check.reason, t)
                      const showAlignment =
                        typeof check.aligned === 'boolean' &&
                        check.result !== 'none' &&
                        check.result !== 'not_applicable'

                      return (
                        <div
                          key={label}
                          className="flex flex-wrap items-center gap-x-2 gap-y-1 text-[12px]"
                        >
                          <span className="w-12 shrink-0 font-mono text-[11.5px] font-semibold text-[var(--ink-secondary)]">
                            {label}
                          </span>
                          <Badge variant={getAuthBadgeVariant(check.result)}>
                            {formatAuthResultLabel(check.result, t)}
                          </Badge>
                          {check.domain && (
                            <code className="rounded bg-[var(--surface-canvas)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--ink-secondary)]">
                              {check.domain}
                            </code>
                          )}
                          {showAlignment && (
                            <span
                              className={cn(
                                'text-[11px] font-medium',
                                check.aligned
                                  ? 'text-[var(--success)]'
                                  : 'text-[var(--ink-muted)]',
                              )}
                            >
                              {check.aligned
                                ? t('mail.security.authDetails.aligned')
                                : t('mail.security.authDetails.unaligned')}
                            </span>
                          )}
                          {label === 'DMARC' && check.policy && (
                            <span className="text-[11px] text-[var(--ink-muted)]">
                              {t('mail.security.authDetails.policyLabel', {
                                policy: formatDmarcPolicyLabel(check.policy, t),
                              })}
                            </span>
                          )}
                          {reasonText && (
                            <span className="text-[11.5px] text-[var(--ink-muted)]">
                              {reasonText}
                            </span>
                          )}
                        </div>
                      )
                    })}
                  </dd>
                </>
              )}
            </dl>
          )}

          {/* Trash Info & Actions Banner */}
          {isInTrash && (
            <div className="mt-4 flex flex-wrap items-center justify-between gap-2 rounded-[var(--radius-md)] border border-[var(--warning)]/35 bg-[var(--warning-soft)]/55 px-3.5 py-2.5 text-[12.5px]">
              <div className="flex flex-wrap items-center gap-x-3 gap-y-0.5 text-[var(--ink-primary)]">
                {email.deleted_at != null && (
                  <span>
                    {t('mail.trash.deletedAt', {
                      time: formatMailFullTime(
                        new Date(email.deleted_at).toISOString(),
                        i18n.language,
                      ),
                    })}
                  </span>
                )}
                {computePurgeDaysLeft(email.purge_at) !== null && (
                  <span className="font-medium text-[var(--warning)]">
                    {t('mail.trash.purgeInDays', {
                      days: computePurgeDaysLeft(email.purge_at) ?? 0,
                    })}
                  </span>
                )}
              </div>
              <div className="flex items-center gap-1.5">
                {onRestore && (
                  <Button
                    type="button"
                    variant="secondary"
                    size="xs"
                    onClick={() => onRestore(email.id)}
                  >
                    <RotateCcw className="h-3.5 w-3.5" />
                    {t('mail.trash.restore')}
                  </Button>
                )}
                <Button
                  type="button"
                  variant="dangerGhost"
                  size="xs"
                  onClick={() => setConfirmHardDelOpen(true)}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                  {t('mail.actions.deleteForever')}
                </Button>
              </div>
            </div>
          )}

          {/* Subtle prompt to configure personal AI when permitted but not yet configured */}
          {!email.ai_available && userAI?.ai_user_config_enabled && !userAI?.configured && (
            <div className="mt-4 flex flex-wrap items-center justify-between gap-2 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/60 px-3.5 py-2 text-[12px] text-[var(--ink-secondary)]">
              <span className="inline-flex items-center gap-1.5">
                <Sparkles className="h-3.5 w-3.5 text-[var(--accent)]" />
                <span>{t('ai.setupPrompt')}</span>
              </span>
              <Link
                to="/settings/translation"
                className="font-medium text-[var(--accent-ink)] hover:underline"
              >
                {t('ai.goToSettings')}
              </Link>
            </div>
          )}

          {/* External Images Privacy Blocker Banner */}
          {hasHtml && hasExternalImages && !imagesShown && (
            <div className="mt-4 flex flex-wrap items-center justify-between gap-2 rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-subtle)] px-3.5 py-2 text-[12.5px]">
              <div className="flex items-center gap-2 text-[var(--ink-secondary)]">
                <ImageIcon className="h-4 w-4 shrink-0 text-[var(--ink-muted)]" />
                <span>{t('mail.security.externalImagesBlocked')}</span>
              </div>
              <Button
                type="button"
                variant="secondary"
                size="xs"
                title={t('mail.security.loadExternalImagesHint')}
                onClick={() => setExternalImagesAllowed(true)}
              >
                {t('mail.security.loadExternalImages')}
              </Button>
            </div>
          )}

          {/* Message Body (+ Translation Card & AI Panels above body) */}
          <div className="mt-5 border-t border-[var(--border-hairline)] pt-5">
            <MailTranslationCard
              result={translationResult}
              loading={translationLoading}
              error={translationError}
              showTranslation={showTranslation}
              onToggleShow={() => setShowTranslation((v) => !v)}
              onRetranslate={() => void runTranslate({ force: true })}
              onClose={() => {
                setTranslationResult(null)
                setTranslationError(null)
              }}
            />

            <MailAIPanel
              summary={summaryResult}
              summaryLoading={summaryLoading}
              summaryError={summaryError}
              extract={extractResult}
              extractLoading={extractLoading}
              extractError={extractError}
              onRefreshSummary={() => void runAI('summary', true)}
              onRefreshExtract={() => void runAI('extract', true)}
              onCloseSummary={() => {
                setSummaryResult(null)
                setSummaryError(null)
              }}
              onCloseExtract={() => {
                setExtractResult(null)
                setExtractError(null)
              }}
            />

            {hasHtml ? (
              <SandboxedHtmlFrame
                html={email.html}
                emailId={email.id}
                imageProxyToken={email.image_proxy_token}
                allowExternalImages={imagesShown}
                onDetectExternalImages={handleDetectExternal}
              />
            ) : (
              <PlainTextMailView
                text={email.text || t('mail.noSnippet')}
                textFormat={email.text_format}
                hasCode={email.has_code}
                riskFindings={email.risk?.findings}
              />
            )}
          </div>

          {/* Attachments */}
          <AttachmentSection
            emailId={email.id}
            attachments={normalizedAttachments}
            riskFindings={email.risk?.findings}
          />

          {/* Quick Reply Actions Footer */}
          <div className="mt-7 flex flex-wrap items-center gap-2 border-t border-[var(--border-hairline)] pt-5 pb-6">
            {isServerDraft && onEditDraft ? (
              <Button variant="primary" size="sm" onClick={() => onEditDraft(email)}>
                {t('mail.draft.resume')}
              </Button>
            ) : (
              <>
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => onReply(email, externalImagesAllowed)}
                >
                  <Reply className="h-3.5 w-3.5" />
                  {t('mail.actions.reply')}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => onReplyAll(email, externalImagesAllowed)}
                >
                  <ReplyAll className="h-3.5 w-3.5" />
                  {t('mail.actions.replyAll')}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => onForward(email, externalImagesAllowed)}
                >
                  <Forward className="h-3.5 w-3.5" />
                  {t('mail.actions.forward')}
                </Button>
              </>
            )}
          </div>
        </div>
      </article>

      {/* Permanent Delete Confirm Dialog */}
      <Dialog open={confirmHardDelOpen} onOpenChange={setConfirmHardDelOpen}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{t('mail.trash.deleteForeverTitle')}</DialogTitle>
            <DialogDescription>
              {t('mail.trash.deleteForeverDesc', { count: 1 })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setConfirmHardDelOpen(false)}
            >
              {t('common.cancel')}
            </Button>
            <Button
              type="button"
              variant="danger"
              size="sm"
              onClick={() => {
                setConfirmHardDelOpen(false)
                onDelete(email.id, true)
              }}
            >
              {t('mail.actions.deleteForever')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
