import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useEditor, EditorContent } from '@tiptap/react'
import StarterKit from '@tiptap/starter-kit'
import { Placeholder } from '@tiptap/extensions'
import dayjs from 'dayjs'
import { toast } from 'sonner'
import {
  AlertCircle,
  Bold,
  CheckCircle2,
  Clock,
  Code,
  Code2,
  Heading2,
  Italic,
  Link2,
  List,
  ListOrdered,
  Loader2,
  Maximize2,
  Minimize2,
  Minus,
  Paperclip,
  Quote,
  Redo2,
  Send,
  Strikethrough,
  Trash2,
  Underline as UnderlineIcon,
  Undo2,
  Upload,
  X,
} from 'lucide-react'
import {
  mailApi,
  type MailboxAddress,
  type SendEmailAttachment,
  type SendEmailResponse,
} from '@/api'
import {
  formatBytes,
  isValidEmail,
  resolveAddressList,
  stripHtmlToText,
} from '@/lib/format'
import { COMPOSE_CODE_LANGUAGES } from '@/lib/code-languages'
import { formatMailFullTime, pickDate } from '@/lib/date'
import { cn } from '@/lib/cn'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Badge } from '@/components/ui/badge'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { RecipientChipInput, type ContactSuggestion } from './RecipientChipInput'
import type { ComposeContext, LocalDraft } from './types'

interface UploadedAttachment extends SendEmailAttachment {
  id: string
  size: number
}

interface ComposeModalProps {
  context: ComposeContext
  sendableAddresses: MailboxAddress[]
  userDisplayName: string
  suggestions: ContactSuggestion[]
  isMobile: boolean
  onSaveDraft: (draft: LocalDraft) => void
  onDeleteDraft: (draftId: string) => void
  onSentSuccess: () => void
  onClose: () => void
}

const MAX_ATTACHMENT_BYTES = 25 * 1024 * 1024

function buildQuotedHtml(srcHtml?: string, srcText?: string): string {
  // Preserve <pre><code> blocks when replying/forwarding!
  if (srcHtml && /<pre[\s>]/i.test(srcHtml)) {
    return srcHtml
  }
  const text = srcText || ''
  if (!text) return ''
  // Convert markdown fenced code blocks into <pre><code class="language-x"> inside quote
  const parts: string[] = []
  const fenceRe = /```([a-zA-Z0-9_+#.-]*)\r?\n([\s\S]*?)```/g
  let last = 0
  let m: RegExpExecArray | null
  while ((m = fenceRe.exec(text)) !== null) {
    if (m.index > last) {
      parts.push(
        text
          .slice(last, m.index)
          .replace(/&/g, '&amp;')
          .replace(/</g, '&lt;')
          .replace(/>/g, '&gt;')
          .replace(/\n/g, '<br>'),
      )
    }
    const lang = (m[1] ?? '').trim()
    const code = (m[2] ?? '')
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
    parts.push(
      lang
        ? `<pre><code class="language-${lang}">${code}</code></pre>`
        : `<pre><code>${code}</code></pre>`,
    )
    last = m.index + m[0].length
  }
  if (last < text.length) {
    parts.push(
      text
        .slice(last)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/\n/g, '<br>'),
    )
  }
  return parts.join('')
}

function readFileAsDataUrl(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result || ''))
    reader.onerror = () => reject(new Error('File read failed'))
    reader.readAsDataURL(file)
  })
}

export function ComposeModal({
  context,
  sendableAddresses,
  userDisplayName,
  suggestions,
  isMobile,
  onSaveDraft,
  onDeleteDraft,
  onSentSuccess,
  onClose,
}: ComposeModalProps) {
  const { t, i18n } = useTranslation()

  const defaultFrom = useMemo(() => {
    const primary = sendableAddresses.find((a) => a.is_primary)
    return primary?.address || sendableAddresses[0]?.address || ''
  }, [sendableAddresses])

  const initialFields = useMemo(() => {
    if (context.mode === 'draft' && context.draft) {
      return {
        draftId: context.draft.id,
        serverDraftId: undefined as number | undefined,
        fromAddress: context.draft.fromAddress || defaultFrom,
        to: context.draft.to,
        cc: context.draft.cc,
        bcc: context.draft.bcc,
        subject: context.draft.subject,
        html: context.draft.html,
      }
    }

    const src = context.sourceEmail
    const draftId = `draft_${Date.now()}_${Math.random().toString(36).slice(2, 7)}`

    if (context.mode === 'serverDraft' && src) {
      return {
        draftId,
        serverDraftId: src.id,
        fromAddress: src.from_address || defaultFrom,
        to: resolveAddressList(src.to_list, src.to).map((x) => x.email),
        cc: resolveAddressList(src.cc_list, src.cc).map((x) => x.email),
        bcc: resolveAddressList(src.bcc_list, src.bcc).map((x) => x.email),
        subject: src.subject || '',
        html: src.html || (src.text ? `<p>${src.text.replace(/\n/g, '<br>')}</p>` : ''),
      }
    }

    if (!src) {
      return {
        draftId,
        serverDraftId: undefined as number | undefined,
        fromAddress: defaultFrom,
        to: [] as string[],
        cc: [] as string[],
        bcc: [] as string[],
        subject: '',
        html: '',
      }
    }

    const senderAddr = src.from_address
    const senderLabel = src.from_name ? `${src.from_name} <${senderAddr}>` : senderAddr
    const dateLabel = formatMailFullTime(
      pickDate(
        src.send_date_iso,
        src.send_time_iso,
        src.create_time_iso,
        src.send_date,
        src.create_time,
      ),
      i18n.language,
    )
    const ownSet = new Set(sendableAddresses.map((a) => a.address.toLowerCase()))

    if (context.mode === 'reply' || context.mode === 'replyAll') {
      const replyTargets = resolveAddressList(src.reply_to_list, src.reply_to)
      const primaryTo = replyTargets.length > 0 ? replyTargets.map((x) => x.email) : senderAddr ? [senderAddr] : []
      let ccList: string[] = []
      if (context.mode === 'replyAll') {
        const allOthers = [
          ...resolveAddressList(src.to_list, src.to),
          ...resolveAddressList(src.cc_list, src.cc),
        ]
          .map((x) => x.email)
          .filter((addr) => addr && !ownSet.has(addr.toLowerCase()) && !primaryTo.includes(addr))
        ccList = Array.from(new Set(allOthers))
      }
      const subject = /^re:/i.test(src.subject) ? src.subject : `Re: ${src.subject}`
      const quoteBody = buildQuotedHtml(src.html, src.text)
      const html = `<p></p><p>${t('compose.quote.wroteAt', { date: dateLabel, sender: senderLabel })}</p><blockquote>${quoteBody}</blockquote>`
      return {
        draftId,
        serverDraftId: undefined as number | undefined,
        fromAddress: defaultFrom,
        to: primaryTo,
        cc: ccList,
        bcc: [] as string[],
        subject,
        html,
      }
    }

    // Forward
    const subject = /^fwd:/i.test(src.subject) ? src.subject : `Fwd: ${src.subject}`
    const quoteBody = buildQuotedHtml(src.html, src.text)
    const html = `<p></p><p>${t('compose.quote.forwardedHeader')}<br>${t('mail.header.from')}: ${senderLabel}<br>${t('mail.header.date')}: ${dateLabel}<br>${t('compose.subject')}: ${src.subject}</p><blockquote>${quoteBody}</blockquote>`
    return {
      draftId,
      serverDraftId: undefined as number | undefined,
      fromAddress: defaultFrom,
      to: [] as string[],
      cc: [] as string[],
      bcc: [] as string[],
      subject,
      html,
    }
  }, [context, defaultFrom, i18n.language, sendableAddresses, t])

  const [draftId] = useState(initialFields.draftId)
  const serverDraftIdRef = useRef<number | undefined>(initialFields.serverDraftId)
  const [fromAddress, setFromAddress] = useState(initialFields.fromAddress)
  const [to, setTo] = useState<string[]>(initialFields.to)
  const [cc, setCc] = useState<string[]>(initialFields.cc)
  const [bcc, setBcc] = useState<string[]>(initialFields.bcc)
  const [showCc, setShowCc] = useState(initialFields.cc.length > 0)
  const [showBcc, setShowBcc] = useState(initialFields.bcc.length > 0)
  const [subject, setSubject] = useState(initialFields.subject)
  const [attachments, setAttachments] = useState<UploadedAttachment[]>([])
  const [includeOrigAttachments, setIncludeOrigAttachments] = useState(
    context.mode === 'forward' && (context.sourceEmail?.attachments?.length ?? 0) > 0,
  )

  const [minimized, setMinimized] = useState(false)
  const [expanded, setExpanded] = useState(false)
  const [draggingFiles, setDraggingFiles] = useState(false)
  const [lastSavedAt, setLastSavedAt] = useState<string | null>(null)
  const [sending, setSending] = useState(false)
  const [sendResult, setSendResult] = useState<SendEmailResponse | null>(null)

  const fileInputRef = useRef<HTMLInputElement | null>(null)

  // Ensure fromAddress syncs if addresses load asynchronously
  useEffect(() => {
    if (!fromAddress && defaultFrom) {
      setFromAddress(defaultFrom)
    }
  }, [fromAddress, defaultFrom])

  const editor = useEditor({
    extensions: [
      StarterKit.configure({
        heading: { levels: [1, 2, 3] },
        link: {
          openOnClick: false,
          autolink: true,
          HTMLAttributes: { target: '_blank', rel: 'noopener noreferrer' },
        },
      }),
      Placeholder.configure({
        placeholder: t('compose.bodyPlaceholder'),
      }),
    ],
    content: initialFields.html,
    immediatelyRender: false,
  })

  // Auto-save draft (debounced 2s)
  const [editorTick, setEditorTick] = useState(0)

  useEffect(() => {
    if (!editor) return
    const onUpdate = () => setEditorTick((n) => n + 1)
    editor.on('update', onUpdate)
    return () => {
      editor.off('update', onUpdate)
    }
  }, [editor])

  useEffect(() => {
    if (sendResult || !editor) return
    const timer = window.setTimeout(() => {
      const html = editor.getHTML()
      const text = editor.getText().trim()
      const hasContent =
        to.length > 0 || cc.length > 0 || bcc.length > 0 || subject.trim().length > 0 || text.length > 0
      if (!hasContent) return

      const nowIso = new Date().toISOString()
      // Save draft to server first (C3 /api/email/draft/save); fall back to localStorage only if offline/unavailable
      void mailApi
        .saveDraft({
          id: serverDraftIdRef.current,
          from_address: fromAddress,
          to: to.map((email) => ({ name: '', email })),
          cc: cc.map((email) => ({ name: '', email })),
          bcc: bcc.map((email) => ({ name: '', email })),
          subject,
          html,
          text,
          attrs: attachments.map(({ name, data }) => ({ name, data })),
        })
        .then((savedId) => {
          serverDraftIdRef.current = savedId
          onDeleteDraft(draftId)
          setLastSavedAt(dayjs(nowIso).format('HH:mm:ss'))
          onSentSuccess()
        })
        .catch(() => {
          onSaveDraft({
            id: draftId,
            fromAddress,
            to,
            cc,
            bcc,
            subject,
            html,
            text,
            updatedAt: nowIso,
            mode:
              context.mode === 'draft' || context.mode === 'serverDraft' ? 'new' : context.mode,
            sourceEmailId: context.sourceEmail?.id,
          })
          setLastSavedAt(dayjs(nowIso).format('HH:mm:ss'))
        })
    }, 2500)

    return () => window.clearTimeout(timer)
  }, [
    draftId,
    fromAddress,
    to,
    cc,
    bcc,
    subject,
    attachments,
    editor,
    editorTick,
    onSaveDraft,
    onDeleteDraft,
    onSentSuccess,
    sendResult,
    context.mode,
    context.sourceEmail?.id,
  ])

  const handleFilesAdded = useCallback(
    async (files: FileList | File[]) => {
      const list = Array.from(files)
      const added: UploadedAttachment[] = []
      for (const file of list) {
        if (file.size > MAX_ATTACHMENT_BYTES) {
          toast.error(t('compose.attachmentTooLarge', { name: file.name }))
          continue
        }
        try {
          const dataUrl = await readFileAsDataUrl(file)
          added.push({
            id: `${Date.now()}_${Math.random().toString(36).slice(2, 7)}`,
            name: file.name,
            data: dataUrl,
            size: file.size,
          })
        } catch {
          toast.error(t('common.unknownError'))
        }
      }
      if (added.length > 0) {
        setAttachments((prev) => [...prev, ...added])
      }
    },
    [t],
  )

  const handleSend = useCallback(async () => {
    if (sending) return

    if (!fromAddress) {
      toast.error(t('compose.validation.fromRequired'))
      return
    }
    if (to.length === 0) {
      toast.error(t('compose.validation.toRequired'))
      return
    }
    const allRecipients = [...to, ...cc, ...bcc]
    const invalid = allRecipients.find((addr) => !isValidEmail(addr))
    if (invalid) {
      toast.error(t('compose.validation.invalidRecipient', { email: invalid }))
      return
    }
    if (!subject.trim()) {
      toast.error(t('compose.validation.subjectRequired'))
      return
    }

    const html = editor?.getHTML() ?? ''
    const text = editor?.getText() || stripHtmlToText(html)

    setSending(true)
    try {
      let result: SendEmailResponse
      const useForwardEndpoint =
        context.mode === 'forward' &&
        context.sourceEmail &&
        includeOrigAttachments &&
        attachments.length === 0 &&
        bcc.length === 0

      if (useForwardEndpoint && context.sourceEmail) {
        result = await mailApi.forward({
          email_id: context.sourceEmail.id,
          to,
          cc: cc.length > 0 ? cc : undefined,
          from_address: fromAddress,
          html,
          text,
          include_attachments: true,
        })
      } else {
        result = await mailApi.send({
          from: { name: userDisplayName, email: fromAddress },
          from_address: fromAddress,
          to: to.map((email) => ({ name: '', email })),
          cc: cc.length > 0 ? cc.map((email) => ({ name: '', email })) : undefined,
          bcc: bcc.length > 0 ? bcc.map((email) => ({ name: '', email })) : undefined,
          subject: subject.trim(),
          html,
          text,
          attrs:
            attachments.length > 0
              ? attachments.map(({ name, data }) => ({ name, data }))
              : undefined,
        })
      }

      setSendResult(result)
      onDeleteDraft(draftId)
      if (serverDraftIdRef.current) {
        const sid = serverDraftIdRef.current
        serverDraftIdRef.current = undefined
        void mailApi.deleteDraft(sid).catch(() => {})
      }
      onSentSuccess()
    } catch (err) {
      const msg = err instanceof Error ? err.message : t('common.unknownError')
      toast.error(msg)
    } finally {
      setSending(false)
    }
  }, [
    sending,
    fromAddress,
    to,
    cc,
    bcc,
    subject,
    editor,
    context.mode,
    context.sourceEmail,
    includeOrigAttachments,
    attachments,
    userDisplayName,
    draftId,
    onDeleteDraft,
    onSentSuccess,
    t,
  ])

  // Cmd/Ctrl + Enter to send
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'Enter' && !minimized && !sendResult) {
        e.preventDefault()
        void handleSend()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [handleSend, minimized, sendResult])

  const handleDiscard = () => {
    onDeleteDraft(draftId)
    if (serverDraftIdRef.current) {
      const sid = serverDraftIdRef.current
      serverDraftIdRef.current = undefined
      void mailApi
        .deleteDraft(sid)
        .then(() => onSentSuccess())
        .catch(() => {})
    }
    toast(t('mail.draft.discarded'))
    onClose()
  }

  const handleInsertLink = () => {
    if (!editor) return
    const previousUrl = editor.getAttributes('link').href as string | undefined
    const url = window.prompt(t('compose.toolbar.linkPrompt'), previousUrl || 'https://')
    if (url === null) return
    if (url.trim() === '') {
      editor.chain().focus().extendMarkRange('link').unsetLink().run()
      return
    }
    editor.chain().focus().extendMarkRange('link').setLink({ href: url.trim() }).run()
  }

  const titleMap: Record<ComposeContext['mode'], string> = {
    new: t('compose.titleNew'),
    reply: t('compose.titleReply'),
    replyAll: t('compose.titleReplyAll'),
    forward: t('compose.titleForward'),
    draft: t('compose.titleDraft'),
    serverDraft: t('compose.titleDraft'),
  }

  const modalTitle = subject.trim() ? `${titleMap[context.mode]} · ${subject.trim()}` : titleMap[context.mode]

  const resultTitle = useMemo(() => {
    if (!sendResult) return ''
    const hasFailed = sendResult.recipients.some((r) => r.status === 'failed')
    if (sendResult.status === 'failed') return t('compose.result.titleFailed')
    if (hasFailed) return t('compose.result.titlePartial')
    if (sendResult.status === 'queued') return t('compose.result.titleQueued')
    return t('compose.result.titleSent')
  }, [sendResult, t])

  // Minimized desktop bottom tray
  if (!isMobile && minimized) {
    return (
      <div className="fixed bottom-0 right-6 z-40 flex h-11 w-80 items-center justify-between gap-2 rounded-t-[var(--radius-md)] border border-b-0 border-[var(--border-subtle)] bg-[var(--surface-elevated)] px-3.5 shadow-[var(--shadow-lg)]">
        <button
          type="button"
          onClick={() => setMinimized(false)}
          className="min-w-0 flex-1 truncate text-left text-[13px] font-medium text-[var(--ink-primary)]"
        >
          {modalTitle}
        </button>
        <div className="flex items-center gap-0.5">
          <Button
            variant="ghost"
            size="iconSm"
            onClick={() => setMinimized(false)}
            aria-label={t('compose.expand')}
          >
            <Maximize2 className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="iconSm"
            onClick={onClose}
            aria-label={t('common.close')}
          >
            <X className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>
    )
  }

  const containerClass = isMobile
    ? 'fixed inset-0 z-50 flex flex-col bg-[var(--surface-elevated)] pt-safe pb-safe'
    : expanded
      ? 'fixed inset-6 z-50 mx-auto flex max-w-4xl flex-col overflow-hidden rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] shadow-[var(--shadow-lg)]'
      : 'fixed bottom-0 right-6 z-50 flex h-[min(640px,calc(100vh-3rem))] w-[min(620px,calc(100vw-3rem))] flex-col overflow-hidden rounded-t-[var(--radius-lg)] border border-b-0 border-[var(--border-subtle)] bg-[var(--surface-elevated)] shadow-[var(--shadow-lg)]'

  return (
    <>
      {!isMobile && expanded && (
        <div
          aria-hidden="true"
          onClick={() => setExpanded(false)}
          className="fixed inset-0 z-40 bg-slate-950/35 backdrop-blur-[1px]"
        />
      )}

      <div
        role="dialog"
        aria-label={titleMap[context.mode]}
        onDragOver={(e) => {
          e.preventDefault()
          setDraggingFiles(true)
        }}
        onDragLeave={(e) => {
          if (e.currentTarget === e.target) setDraggingFiles(false)
        }}
        onDrop={(e) => {
          e.preventDefault()
          setDraggingFiles(false)
          if (e.dataTransfer.files?.length) {
            void handleFilesAdded(e.dataTransfer.files)
          }
        }}
        className={containerClass}
      >
        {/* Drag-and-drop overlay */}
        {draggingFiles && (
          <div className="pointer-events-none absolute inset-0 z-30 flex flex-col items-center justify-center gap-2 bg-[var(--accent)]/10 backdrop-blur-[2px] border-2 border-dashed border-[var(--accent)] rounded-[var(--radius-lg)]">
            <Upload className="h-8 w-8 text-[var(--accent)]" />
            <p className="text-[14px] font-medium text-[var(--accent)]">
              {t('compose.dropFilesHint')}
            </p>
          </div>
        )}

        {/* Header bar */}
        <header className="flex h-11 shrink-0 items-center justify-between gap-2 border-b border-[var(--border-hairline)] bg-[var(--surface-sidebar)] px-3.5">
          <div className="flex min-w-0 items-center gap-2">
            <span className="truncate text-[13.5px] font-semibold text-[var(--ink-primary)]">
              {titleMap[context.mode]}
            </span>
            {lastSavedAt && !sendResult && (
              <span className="hidden text-[11.5px] text-[var(--ink-muted)] sm:inline">
                · {t('compose.savedAt', { time: lastSavedAt })}
              </span>
            )}
          </div>
          <div className="flex items-center gap-0.5">
            {!isMobile && (
              <>
                <Button
                  type="button"
                  variant="ghost"
                  size="iconSm"
                  onClick={() => setMinimized(true)}
                  aria-label={t('compose.minimize')}
                  title={t('compose.minimize')}
                >
                  <Minus className="h-3.5 w-3.5" />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="iconSm"
                  onClick={() => setExpanded((v) => !v)}
                  aria-label={t('compose.expand')}
                  title={t('compose.expand')}
                >
                  {expanded ? (
                    <Minimize2 className="h-3.5 w-3.5" />
                  ) : (
                    <Maximize2 className="h-3.5 w-3.5" />
                  )}
                </Button>
              </>
            )}
            <Button
              type="button"
              variant="ghost"
              size="iconSm"
              onClick={onClose}
              aria-label={t('common.close')}
              title={t('common.close')}
            >
              <X className="h-4 w-4" />
            </Button>
          </div>
        </header>

        {/* Per-Recipient Delivery Result View */}
        {sendResult ? (
          <div className="flex flex-1 flex-col justify-between p-6">
            <div>
              <div className="flex items-center gap-2.5">
                {sendResult.status === 'failed' ? (
                  <AlertCircle className="h-6 w-6 text-[var(--danger)]" />
                ) : sendResult.status === 'queued' ? (
                  <Clock className="h-6 w-6 text-[var(--accent)]" />
                ) : (
                  <CheckCircle2 className="h-6 w-6 text-[var(--success)]" />
                )}
                <h2 className="text-[16px] font-semibold text-[var(--ink-primary)]">
                  {resultTitle}
                </h2>
              </div>

              <div className="mt-4 divide-y divide-[var(--border-hairline)] rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-subtle)]/50">
                {sendResult.recipients.map((rec) => (
                  <div
                    key={rec.address}
                    className="flex flex-wrap items-center justify-between gap-2 px-3.5 py-2.5 text-[13px]"
                  >
                    <div className="min-w-0 flex-1">
                      <p className="truncate font-medium text-[var(--ink-primary)]">
                        {rec.address}
                      </p>
                      {rec.error && (
                        <p className="mt-0.5 break-all text-[12px] text-[var(--danger)]">
                          {rec.error}
                        </p>
                      )}
                    </div>
                    <Badge
                      variant={
                        rec.status === 'delivered'
                          ? 'success'
                          : rec.status === 'queued'
                            ? 'accent'
                            : 'danger'
                      }
                    >
                      {rec.status === 'delivered'
                        ? t('compose.result.statusDelivered')
                        : rec.status === 'queued'
                          ? t('compose.result.statusQueued')
                          : t('compose.result.statusFailed')}
                    </Badge>
                  </div>
                ))}
              </div>
            </div>

            <div className="mt-6 flex items-center justify-end gap-2">
              {sendResult.recipients.some((r) => r.status === 'failed') && (
                <Button variant="outline" size="sm" onClick={() => setSendResult(null)}>
                  {t('compose.result.keepEditing')}
                </Button>
              )}
              <Button variant="primary" size="sm" onClick={onClose}>
                {t('compose.result.done')}
              </Button>
            </div>
          </div>
        ) : (
          <>
            {/* Envelope Fields */}
            <div className="shrink-0 divide-y divide-[var(--border-hairline)] border-b border-[var(--border-hairline)] px-4">
              {/* From */}
              <div className="flex items-center gap-3 py-1.5">
                <label
                  htmlFor="compose_from"
                  className="w-12 shrink-0 text-[12.5px] text-[var(--ink-muted)]"
                >
                  {t('compose.from')}
                </label>
                {sendableAddresses.length > 0 ? (
                  <Select value={fromAddress} onValueChange={setFromAddress}>
                    <SelectTrigger id="compose_from" className="h-7 border-0 bg-transparent px-1 shadow-none hover:bg-[var(--surface-hover)]">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {sendableAddresses.map((addr) => (
                        <SelectItem key={addr.id} value={addr.address}>
                          {userDisplayName ? `${userDisplayName} <${addr.address}>` : addr.address}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                ) : (
                  <span className="text-[12.5px] text-[var(--danger)]">
                    {t('compose.noSendAddress')}
                  </span>
                )}
              </div>

              {/* To */}
              <div className="flex items-start gap-3">
                <label
                  htmlFor="compose_to"
                  className="w-12 shrink-0 pt-2 text-[12.5px] text-[var(--ink-muted)]"
                >
                  {t('compose.to')}
                </label>
                <RecipientChipInput
                  id="compose_to"
                  values={to}
                  onChange={setTo}
                  suggestions={suggestions}
                  placeholder={t('compose.recipientPlaceholder')}
                  autoFocus={context.mode === 'new' || context.mode === 'forward'}
                />
                <div className="flex shrink-0 items-center gap-1 pt-1.5">
                  {!showCc && (
                    <button
                      type="button"
                      onClick={() => setShowCc(true)}
                      className="rounded px-1.5 py-0.5 text-[12px] text-[var(--ink-muted)] hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]"
                    >
                      {t('compose.showCc')}
                    </button>
                  )}
                  {!showBcc && (
                    <button
                      type="button"
                      onClick={() => setShowBcc(true)}
                      className="rounded px-1.5 py-0.5 text-[12px] text-[var(--ink-muted)] hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]"
                    >
                      {t('compose.showBcc')}
                    </button>
                  )}
                </div>
              </div>

              {/* Cc */}
              {showCc && (
                <div className="flex items-start gap-3">
                  <label
                    htmlFor="compose_cc"
                    className="w-12 shrink-0 pt-2 text-[12.5px] text-[var(--ink-muted)]"
                  >
                    {t('compose.cc')}
                  </label>
                  <RecipientChipInput
                    id="compose_cc"
                    values={cc}
                    onChange={setCc}
                    suggestions={suggestions}
                    placeholder={t('compose.recipientPlaceholder')}
                  />
                </div>
              )}

              {/* Bcc */}
              {showBcc && (
                <div className="flex items-start gap-3">
                  <label
                    htmlFor="compose_bcc"
                    className="w-12 shrink-0 pt-2 text-[12.5px] text-[var(--ink-muted)]"
                  >
                    {t('compose.bcc')}
                  </label>
                  <RecipientChipInput
                    id="compose_bcc"
                    values={bcc}
                    onChange={setBcc}
                    suggestions={suggestions}
                    placeholder={t('compose.recipientPlaceholder')}
                  />
                </div>
              )}

              {/* Subject */}
              <div className="flex items-center gap-3 py-1.5">
                <label
                  htmlFor="compose_subject"
                  className="w-12 shrink-0 text-[12.5px] text-[var(--ink-muted)]"
                >
                  {t('compose.subject')}
                </label>
                <input
                  id="compose_subject"
                  type="text"
                  value={subject}
                  onChange={(e) => setSubject(e.target.value)}
                  placeholder={t('compose.subjectPlaceholder')}
                  className="h-7 flex-1 bg-transparent text-[13.5px] font-medium text-[var(--ink-primary)] placeholder:font-normal placeholder:text-[var(--ink-subtle)] outline-none"
                />
              </div>
            </div>

            {/* Formatting Toolbar */}
            {editor && (
              <div className="flex shrink-0 flex-wrap items-center gap-0.5 border-b border-[var(--border-hairline)] bg-[var(--surface-subtle)]/50 px-3 py-1">
                <ToolbarBtn
                  active={editor.isActive('bold')}
                  onClick={() => editor.chain().focus().toggleBold().run()}
                  label={t('compose.toolbar.bold')}
                  icon={Bold}
                />
                <ToolbarBtn
                  active={editor.isActive('italic')}
                  onClick={() => editor.chain().focus().toggleItalic().run()}
                  label={t('compose.toolbar.italic')}
                  icon={Italic}
                />
                <ToolbarBtn
                  active={editor.isActive('underline')}
                  onClick={() => editor.chain().focus().toggleUnderline().run()}
                  label={t('compose.toolbar.underline')}
                  icon={UnderlineIcon}
                />
                <ToolbarBtn
                  active={editor.isActive('strike')}
                  onClick={() => editor.chain().focus().toggleStrike().run()}
                  label={t('compose.toolbar.strike')}
                  icon={Strikethrough}
                />
                <span className="mx-1 h-4 w-px bg-[var(--border-subtle)]" />
                <ToolbarBtn
                  active={editor.isActive('heading', { level: 2 })}
                  onClick={() => editor.chain().focus().toggleHeading({ level: 2 }).run()}
                  label={t('compose.toolbar.heading')}
                  icon={Heading2}
                />
                <ToolbarBtn
                  active={editor.isActive('bulletList')}
                  onClick={() => editor.chain().focus().toggleBulletList().run()}
                  label={t('compose.toolbar.bulletList')}
                  icon={List}
                />
                <ToolbarBtn
                  active={editor.isActive('orderedList')}
                  onClick={() => editor.chain().focus().toggleOrderedList().run()}
                  label={t('compose.toolbar.orderedList')}
                  icon={ListOrdered}
                />
                <ToolbarBtn
                  active={editor.isActive('blockquote')}
                  onClick={() => editor.chain().focus().toggleBlockquote().run()}
                  label={t('compose.toolbar.blockquote')}
                  icon={Quote}
                />
                <ToolbarBtn
                  active={editor.isActive('code')}
                  onClick={() => editor.chain().focus().toggleCode().run()}
                  label={t('compose.toolbar.code')}
                  icon={Code}
                />
                <ToolbarBtn
                  active={editor.isActive('codeBlock')}
                  onClick={() => editor.chain().focus().toggleCodeBlock().run()}
                  label={t('compose.toolbar.codeBlock')}
                  icon={Code2}
                />
                {editor.isActive('codeBlock') && (
                  <select
                    aria-label={t('compose.toolbar.codeLanguage')}
                    value={(editor.getAttributes('codeBlock').language as string) || 'text'}
                    onChange={(e) =>
                      editor
                        .chain()
                        .focus()
                        .updateAttributes('codeBlock', {
                          language: e.target.value === 'text' ? null : e.target.value,
                        })
                        .run()
                    }
                    className="h-6 rounded-[4px] border border-[var(--border-subtle)] bg-[var(--surface-canvas)] px-1.5 text-[11.5px] text-[var(--ink-primary)] outline-none focus:border-[var(--accent)]"
                  >
                    {COMPOSE_CODE_LANGUAGES.map((lang) => (
                      <option key={lang} value={lang}>
                        {t(`mail.code.languages.${lang}` as const)}
                      </option>
                    ))}
                  </select>
                )}
                <ToolbarBtn
                  active={editor.isActive('link')}
                  onClick={handleInsertLink}
                  label={t('compose.toolbar.link')}
                  icon={Link2}
                />
                <span className="mx-1 h-4 w-px bg-[var(--border-subtle)]" />
                <ToolbarBtn
                  active={false}
                  disabled={!editor.can().undo()}
                  onClick={() => editor.chain().focus().undo().run()}
                  label={t('compose.toolbar.undo')}
                  icon={Undo2}
                />
                <ToolbarBtn
                  active={false}
                  disabled={!editor.can().redo()}
                  onClick={() => editor.chain().focus().redo().run()}
                  label={t('compose.toolbar.redo')}
                  icon={Redo2}
                />
              </div>
            )}

            {/* TipTap Scrollable Editor */}
            <div className="tiptap-editor mail-scroll flex-1 overflow-y-auto px-4 py-3">
              <EditorContent editor={editor} />
            </div>

            {/* Forward original attachments option + Uploaded attachments chips */}
            {((context.mode === 'forward' && (context.sourceEmail?.attachments?.length ?? 0) > 0) ||
              attachments.length > 0) && (
              <div className="shrink-0 space-y-2 border-t border-[var(--border-hairline)] bg-[var(--surface-subtle)]/40 px-4 py-2.5">
                {context.mode === 'forward' && (context.sourceEmail?.attachments?.length ?? 0) > 0 && (
                  <label className="flex items-center gap-2 text-[12.5px] text-[var(--ink-secondary)] cursor-pointer">
                    <Checkbox
                      checked={includeOrigAttachments}
                      onCheckedChange={(v) => setIncludeOrigAttachments(v === true)}
                    />
                    <span>
                      {t('compose.includeOriginalAttachments', {
                        count: context.sourceEmail?.attachments?.length ?? 0,
                      })}
                    </span>
                  </label>
                )}

                {attachments.length > 0 && (
                  <div className="flex flex-wrap gap-1.5">
                    {attachments.map((att) => (
                      <span
                        key={att.id}
                        className="inline-flex max-w-[240px] items-center gap-1.5 rounded-[var(--radius-sm)] border border-[var(--border-subtle)] bg-[var(--surface-canvas)] px-2 py-1 text-[12px] text-[var(--ink-primary)]"
                      >
                        <Paperclip className="h-3 w-3 shrink-0 text-[var(--ink-muted)]" />
                        <span className="truncate">{att.name}</span>
                        <span className="shrink-0 text-[11px] text-[var(--ink-muted)]">
                          ({formatBytes(att.size)})
                        </span>
                        <button
                          type="button"
                          onClick={() =>
                            setAttachments((prev) => prev.filter((x) => x.id !== att.id))
                          }
                          aria-label={t('compose.removeAttachment')}
                          className="rounded p-0.5 text-[var(--ink-muted)] hover:bg-[var(--danger-soft)] hover:text-[var(--danger)]"
                        >
                          <X className="h-3 w-3" />
                        </button>
                      </span>
                    ))}
                  </div>
                )}
              </div>
            )}

            {/* Footer Action Bar */}
            <footer className="flex shrink-0 items-center justify-between gap-2 border-t border-[var(--border-hairline)] bg-[var(--surface-sidebar)] px-4 py-2.5">
              <div className="flex items-center gap-1.5">
                <Button
                  type="button"
                  variant="primary"
                  size="sm"
                  disabled={sending}
                  onClick={() => void handleSend()}
                  title={t('compose.sendShortcut')}
                >
                  {sending ? (
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  ) : (
                    <Send className="h-3.5 w-3.5" />
                  )}
                  {sending ? t('compose.sending') : t('compose.send')}
                </Button>

                <input
                  ref={fileInputRef}
                  type="file"
                  multiple
                  className="hidden"
                  onChange={(e) => {
                    if (e.target.files?.length) {
                      void handleFilesAdded(e.target.files)
                      e.target.value = ''
                    }
                  }}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => fileInputRef.current?.click()}
                >
                  <Paperclip className="h-3.5 w-3.5" />
                  <span className="hidden sm:inline">{t('compose.addAttachment')}</span>
                </Button>
              </div>

              <div className="flex items-center gap-2">
                <span className="hidden text-[11.5px] text-[var(--ink-muted)] md:inline">
                  {t('compose.sendShortcut')}
                </span>
                <Button
                  type="button"
                  variant="dangerGhost"
                  size="iconSm"
                  onClick={handleDiscard}
                  aria-label={t('compose.discard')}
                  title={t('compose.discard')}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            </footer>
          </>
        )}
      </div>
    </>
  )
}

function ToolbarBtn({
  active,
  disabled,
  onClick,
  label,
  icon: Icon,
}: {
  active: boolean
  disabled?: boolean
  onClick: () => void
  label: string
  icon: typeof Bold
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      aria-label={label}
      title={label}
      className={cn(
        'inline-flex h-7 w-7 items-center justify-center rounded-[var(--radius-xs)] transition-colors disabled:opacity-40',
        active
          ? 'bg-[var(--accent-soft)] text-[var(--accent)]'
          : 'text-[var(--ink-secondary)] hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]',
      )}
    >
      <Icon className="h-3.5 w-3.5" />
    </button>
  )
}
