import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useEditor, EditorContent } from '@tiptap/react'
import dayjs from 'dayjs'
import { toast } from 'sonner'
import {
  AlertCircle,
  CheckCircle2,
  Clock,
  Image as ImageIcon,
  Loader2,
  Maximize2,
  Minimize2,
  Minus,
  Paperclip,
  PenLine,
  Send,
  Trash2,
  Upload,
  X,
} from 'lucide-react'
import {
  mailApi,
  type EmailDetail,
  type InlineImageRef,
  type MailboxAddress,
  type SendEmailAttachment,
  type SendEmailResponse,
  type SignatureRecord,
} from '@/api'
import { useSignaturesQuery } from '@/hooks/useMailQueries'
import { formatBytes, isValidEmail, resolveAddressList } from '@/lib/format'
import { formatMailFullTime, pickDate } from '@/lib/date'
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
import { createComposeExtensions } from './editor/extensions'
import { EditorToolbar } from './editor/EditorToolbar'
import { LinkBubble } from './editor/LinkBubble'
import {
  detectPastedCode,
  MAX_INLINE_IMAGE_BYTES,
  sanitizePastedHtml,
} from './editor/paste-sanitizer'
import { htmlToMailPlainText } from './editor/html-to-text'
import { SIGNATURE_BLOCK_ATTR, type InlineImageDraft } from './editor/types'
import type { ComposeContext, LocalDraft } from './types'

const ALLOWED_INLINE_IMAGE_TYPES = new Set([
  'image/png',
  'image/jpeg',
  'image/gif',
  'image/webp',
])

function prepareOutgoingHtml(html: string): { html: string; inlineImages: InlineImageRef[] } {
  if (typeof DOMParser === 'undefined' || !html) {
    return { html, inlineImages: [] }
  }
  const doc = new DOMParser().parseFromString(html, 'text/html')
  const seen = new Set<string>()
  const inlineImages: InlineImageRef[] = []

  for (const img of Array.from(doc.querySelectorAll('img[data-cid]'))) {
    const cid = img.getAttribute('data-cid')?.trim()
    if (!cid || cid.startsWith('uploading-')) continue
    img.setAttribute('src', `cid:${cid}`)
    if (!seen.has(cid)) {
      seen.add(cid)
      inlineImages.push({ cid })
    }
  }
  return { html: doc.body.innerHTML, inlineImages }
}

function restoreDraftCidImages(html: string, sourceEmail?: EmailDetail): string {
  if (typeof DOMParser === 'undefined' || !html) return html
  const doc = new DOMParser().parseFromString(html, 'text/html')
  const attachmentResolver = buildCidResolver(sourceEmail)

  for (const img of Array.from(doc.querySelectorAll('img'))) {
    const src = img.getAttribute('src') || ''
    const dataCid = img.getAttribute('data-cid')?.trim()
    const cidMatch = /^cid:(.+)$/i.exec(src.trim())
    const cid = cidMatch?.[1]?.trim() || dataCid
    if (cid && (cidMatch || !src)) {
      const previewUrl =
        attachmentResolver?.(cid) ?? `/api/compose/inline-image/${encodeURIComponent(cid)}`
      img.setAttribute('src', previewUrl)
      img.setAttribute('data-cid', cid)
    }
  }
  return doc.body.innerHTML
}

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
  signatures?: SignatureRecord[]
  onSaveDraft: (draft: LocalDraft) => void
  onDeleteDraft: (draftId: string) => void
  onSentSuccess: () => void
  onClose: () => void
}

const MAX_ATTACHMENT_BYTES = 25 * 1024 * 1024

function escapeHtml(str: string): string {
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

function buildCidResolver(sourceEmail?: EmailDetail): ((cid: string) => string | null) | undefined {
  if (!sourceEmail) return undefined
  const cidMap = new Map<string, number>()

  for (const att of sourceEmail.attachments_list ?? []) {
    if (att.content_id) {
      const clean = att.content_id.replace(/^[<\s]+|[>\s]+$/g, '').toLowerCase()
      if (clean) cidMap.set(clean, att.index)
    }
  }
  for (const att of sourceEmail.attachments ?? []) {
    if (att.ContentID) {
      const clean = att.ContentID.replace(/^[<\s]+|[>\s]+$/g, '').toLowerCase()
      if (clean && !cidMap.has(clean)) cidMap.set(clean, att.Index)
    }
  }

  if (cidMap.size === 0) return undefined
  return (cid: string) => {
    const idx = cidMap.get(cid.trim().toLowerCase())
    return idx !== undefined ? mailApi.attachmentInlineUrl(sourceEmail.id, idx) : null
  }
}

interface BuildQuotedHtmlOptions {
  allowExternalImages: boolean
  sourceEmail?: EmailDetail
  imagePlaceholder: (label: string) => string
  fallbackImageLabel: string
}

function buildQuotedHtml(
  srcHtml: string | undefined,
  srcText: string | undefined,
  opts: BuildQuotedHtmlOptions,
): string {
  // Preserve rich HTML (code blocks, tables, lists) while strictly blocking external tracking images unless explicitly allowed
  if (srcHtml && srcHtml.trim().length > 0) {
    return sanitizePastedHtml(srcHtml, {
      blockExternalImages: !opts.allowExternalImages,
      maxDataImageBytes: MAX_INLINE_IMAGE_BYTES,
      resolveCidUrl: buildCidResolver(opts.sourceEmail),
      imagePlaceholder: opts.imagePlaceholder,
      fallbackImageLabel: opts.fallbackImageLabel,
    })
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
      parts.push(`<p>${escapeHtml(text.slice(last, m.index)).replace(/\n/g, '<br>')}</p>`)
    }
    const lang = (m[1] ?? '').trim()
    const code = escapeHtml(m[2] ?? '')
    parts.push(
      lang
        ? `<pre><code class="language-${escapeHtml(lang)}">${code}</code></pre>`
        : `<pre><code>${code}</code></pre>`,
    )
    last = m.index + m[0].length
  }
  if (last < text.length) {
    parts.push(`<p>${escapeHtml(text.slice(last)).replace(/\n/g, '<br>')}</p>`)
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
  signatures,
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
        html: restoreDraftCidImages(context.draft.html),
      }
    }

    const src = context.sourceEmail
    const draftId = `draft_${Date.now()}_${Math.random().toString(36).slice(2, 7)}`

    if (context.mode === 'serverDraft' && src) {
      const rawDraftHtml =
        src.html || (src.text ? `<p>${escapeHtml(src.text).replace(/\n/g, '<br>')}</p>` : '')
      return {
        draftId,
        serverDraftId: src.id,
        fromAddress: src.from_address || defaultFrom,
        to: resolveAddressList(src.to_list, src.to).map((x) => x.email),
        cc: resolveAddressList(src.cc_list, src.cc).map((x) => x.email),
        bcc: resolveAddressList(src.bcc_list, src.bcc).map((x) => x.email),
        subject: src.subject || '',
        html: restoreDraftCidImages(rawDraftHtml, src),
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

    const quoteOpts: BuildQuotedHtmlOptions = {
      allowExternalImages: Boolean(context.allowExternalImages),
      sourceEmail: src,
      imagePlaceholder: (label) => t('compose.quote.blockedImage', { label }),
      fallbackImageLabel: t('compose.quote.imageFallback'),
    }

    if (context.mode === 'reply' || context.mode === 'replyAll') {
      const replyTargets = resolveAddressList(src.reply_to_list, src.reply_to)
      const primaryTo =
        replyTargets.length > 0 ? replyTargets.map((x) => x.email) : senderAddr ? [senderAddr] : []
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
      const quoteBody = buildQuotedHtml(src.html, src.text, quoteOpts)
      const html = `<p></p><p>${escapeHtml(
        t('compose.quote.wroteAt', { date: dateLabel, sender: senderLabel }),
      )}</p><blockquote>${quoteBody}</blockquote>`
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

    // Forward: include structured original headers (From / Date / Subject / To / Cc)
    const subject = /^fwd:/i.test(src.subject) ? src.subject : `Fwd: ${src.subject}`
    const origTo = resolveAddressList(src.to_list, src.to)
      .map((x) => (x.name ? `${x.name} <${x.email}>` : x.email))
      .join(', ')
    const origCc = resolveAddressList(src.cc_list, src.cc)
      .map((x) => (x.name ? `${x.name} <${x.email}>` : x.email))
      .join(', ')
    const headerLines = [
      escapeHtml(t('compose.quote.forwardedHeader')),
      `${escapeHtml(t('mail.header.from'))}: ${escapeHtml(senderLabel)}`,
      `${escapeHtml(t('mail.header.date'))}: ${escapeHtml(dateLabel)}`,
      `${escapeHtml(t('compose.subject'))}: ${escapeHtml(src.subject || '')}`,
      origTo ? `${escapeHtml(t('mail.header.to'))}: ${escapeHtml(origTo)}` : '',
      origCc ? `${escapeHtml(t('mail.header.cc'))}: ${escapeHtml(origCc)}` : '',
    ].filter(Boolean)

    const quoteBody = buildQuotedHtml(src.html, src.text, quoteOpts)
    const html = `<p></p><p>${headerLines.join('<br>')}</p><blockquote>${quoteBody}</blockquote>`
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
  const [inlineImages, setInlineImages] = useState<InlineImageDraft[]>([])
  const [uploadingImageNames, setUploadingImageNames] = useState<string[]>([])
  const [includeOrigAttachments, setIncludeOrigAttachments] = useState(
    context.mode === 'forward' && (context.sourceEmail?.attachments?.length ?? 0) > 0,
  )

  const [minimized, setMinimized] = useState(false)
  const [expanded, setExpanded] = useState(false)
  const [draggingFiles, setDraggingFiles] = useState(false)
  const [linkBubbleOpen, setLinkBubbleOpen] = useState(false)
  const [lastSavedAt, setLastSavedAt] = useState<string | null>(null)
  const [sending, setSending] = useState(false)
  const [sendResult, setSendResult] = useState<SendEmailResponse | null>(null)
  const [mobileViewportHeight, setMobileViewportHeight] = useState<number | null>(null)

  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const imageInputRef = useRef<HTMLInputElement | null>(null)

  // Signatures from GET /api/settings/signatures (C9)
  const { data: fetchedSignatures = [] } = useSignaturesQuery(!signatures)
  const allSignatures = signatures ?? fetchedSignatures

  const [selectedSignatureId, setSelectedSignatureId] = useState<string>('none')
  const manualSignatureChoiceRef = useRef(false)

  // Auto-select signature by C9 rules: address-bound > default > none.
  // Re-runs when sender changes unless user manually picked a signature.
  useEffect(() => {
    if (manualSignatureChoiceRef.current) return
    if (context.mode === 'draft' || context.mode === 'serverDraft') return
    if (allSignatures.length === 0) return

    const activeEmail = (fromAddress || defaultFrom).toLowerCase()
    const addrObj = sendableAddresses.find((a) => a.address.toLowerCase() === activeEmail)
    const boundSig = addrObj
      ? allSignatures.find((s) => (s.address_ids ?? []).includes(addrObj.id))
      : undefined
    const defaultSig = allSignatures.find((s) => s.is_default)
    const chosen = boundSig ?? defaultSig
    setSelectedSignatureId(chosen ? String(chosen.id) : 'none')
  }, [allSignatures, fromAddress, defaultFrom, sendableAddresses, context.mode])

  // Track visualViewport height on mobile so virtual keyboard never causes modal jump
  useEffect(() => {
    if (!isMobile || typeof window === 'undefined' || !window.visualViewport) return
    const vv = window.visualViewport
    const syncHeight = () => setMobileViewportHeight(Math.round(vv.height))
    syncHeight()
    vv.addEventListener('resize', syncHeight)
    return () => vv.removeEventListener('resize', syncHeight)
  }, [isMobile])

  // Ensure fromAddress syncs if addresses load asynchronously
  useEffect(() => {
    if (!fromAddress && defaultFrom) {
      setFromAddress(defaultFrom)
    }
  }, [fromAddress, defaultFrom])

  // Ref for inline image handler so ProseMirror handlePaste / handleDrop stays fresh
  const insertInlineImageRef = useRef<(file: File) => Promise<void>>(async () => {})

  const extensions = useMemo(
    () => createComposeExtensions(t('compose.bodyPlaceholder')),
    [t],
  )

  const editor = useEditor({
    extensions,
    content: initialFields.html,
    immediatelyRender: false,
    editorProps: {
      transformPastedHTML(html) {
        const cleaned = sanitizePastedHtml(html)
        // Notify quietly when external scripts/styles/Word XML were stripped
        if (/<(?:script|style|xml|o:|w:)|mso-|on[a-z]+\s*=/i.test(html)) {
          toast.info(t('compose.paste.cleanedHtml'))
        }
        return cleaned
      },
      handlePaste(view, event) {
        const clipboard = event.clipboardData
        if (!clipboard) return false

        // 1. Pasted image file -> reserved C9 inline image preview
        const items = Array.from(clipboard.items || [])
        const imageItem = items.find((item) => item.type.startsWith('image/'))
        if (imageItem) {
          const file = imageItem.getAsFile()
          if (file) {
            event.preventDefault()
            void insertInlineImageRef.current(file)
            return true
          }
        }

        // 2. Smart plain-text code detection (when pasting plain text outside an existing codeBlock)
        const htmlData = clipboard.getData('text/html')
        const plainData = clipboard.getData('text/plain')
        const inCodeBlock = view.state.selection.$from.parent.type.name === 'codeBlock'

        if (!htmlData && plainData && !inCodeBlock) {
          const detected = detectPastedCode(plainData)
          if (detected.isCode) {
            event.preventDefault()
            const normalizedCode = plainData.replace(/\r\n/g, '\n')
            const codeBlockType = view.state.schema.nodes.codeBlock
            if (!codeBlockType) return false

            const fromPos = view.state.selection.from
            const node = codeBlockType.create(
              { language: detected.language === 'text' ? null : detected.language },
              view.state.schema.text(normalizedCode),
            )
            const tr = view.state.tr.replaceSelectionWith(node)
            view.dispatch(tr)

            const langLabel = t(`mail.code.languages.${detected.language}` as const)
            toast(t('compose.paste.codeDetected', { lang: langLabel }), {
              action: {
                label: t('compose.paste.undoToPlain'),
                onClick: () => {
                  const paragraphs = normalizedCode
                    .split('\n')
                    .map((line) =>
                      view.state.schema.nodes.paragraph!.create(
                        null,
                        line ? view.state.schema.text(line) : undefined,
                      ),
                    )
                  const endPos = Math.min(
                    fromPos + node.nodeSize,
                    view.state.doc.content.size,
                  )
                  const replaceTr = view.state.tr.replaceWith(fromPos, endPos, paragraphs)
                  view.dispatch(replaceTr)
                },
              },
            })
            return true
          }
        }
        return false
      },
      handleDrop(_view, event) {
        const files = Array.from(event.dataTransfer?.files || [])
        const imageFiles = files.filter((f) => f.type.startsWith('image/'))
        if (imageFiles.length > 0) {
          event.preventDefault()
          for (const img of imageFiles) {
            void insertInlineImageRef.current(img)
          }
          return true
        }
        return false
      },
    },
  })

  // Keep insertInlineImageRef synced with latest editor instance (C9 POST /api/compose/inline-image)
  const insertInlineImageFile = useCallback(
    async (file: File) => {
      if (!editor) return
      const mobileToastOpts = isMobile
        ? { position: 'top-center' as const, duration: 1800 }
        : undefined
      if (file.type && !ALLOWED_INLINE_IMAGE_TYPES.has(file.type.toLowerCase())) {
        toast.error(t('compose.inlineImage.errorType'), mobileToastOpts)
        return
      }
      if (file.size > MAX_INLINE_IMAGE_BYTES) {
        toast.error(t('compose.inlineImage.tooLarge', { name: file.name }), mobileToastOpts)
        return
      }

      const filename = file.name || 'image.png'
      const tempCid = `uploading-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`
      const tempObjectUrl = URL.createObjectURL(file)

      // Insert temporary placeholder image immediately so user sees visual feedback
      editor
        .chain()
        .focus()
        .insertContent({
          type: 'image',
          attrs: {
            src: tempObjectUrl,
            alt: t('compose.inlineImage.uploading', { name: filename }),
            'data-cid': tempCid,
            'data-filename': filename,
          },
        })
        .run()

      setUploadingImageNames((prev) => [...prev, filename])

      try {
        const res = await mailApi.uploadInlineImage(file)
        // Update placeholder node in-place with server preview url & real cid
        const { state, view } = editor
        let tr = state.tr
        let updated = false
        state.doc.descendants((node, pos) => {
          if (node.type.name === 'image' && node.attrs['data-cid'] === tempCid) {
            tr = tr.setNodeMarkup(pos, undefined, {
              ...node.attrs,
              src: res.url,
              alt: res.filename || filename,
              'data-cid': res.cid,
              'data-filename': res.filename || filename,
            })
            updated = true
          }
        })
        if (updated) {
          view.dispatch(tr)
        }
        setInlineImages((prev) => [
          ...prev,
          {
            cid: res.cid,
            filename: res.filename || filename,
            contentType: res.content_type || file.type || 'image/png',
            size: res.size || file.size,
            url: res.url,
          },
        ])
        toast.success(
          t('compose.inlineImage.uploaded', { name: res.filename || filename }),
          mobileToastOpts,
        )
      } catch (err) {
        // Remove failed placeholder node from editor
        const { state, view } = editor
        let tr = state.tr
        const positions: { from: number; to: number }[] = []
        state.doc.descendants((node, pos) => {
          if (node.type.name === 'image' && node.attrs['data-cid'] === tempCid) {
            positions.push({ from: pos, to: pos + node.nodeSize })
          }
        })
        for (let i = positions.length - 1; i >= 0; i--) {
          const range = positions[i]!
          tr = tr.delete(range.from, range.to)
        }
        if (positions.length > 0) {
          view.dispatch(tr)
        }
        toast.error(
          err instanceof Error
            ? err.message
            : t('compose.inlineImage.uploadFailed', { name: filename }),
          mobileToastOpts,
        )
      } finally {
        URL.revokeObjectURL(tempObjectUrl)
        setUploadingImageNames((prev) => {
          const idx = prev.indexOf(filename)
          if (idx < 0) return prev
          const copy = [...prev]
          copy.splice(idx, 1)
          return copy
        })
      }
    },
    [editor, isMobile, t],
  )

  useEffect(() => {
    insertInlineImageRef.current = insertInlineImageFile
  }, [insertInlineImageFile])

  // Track editor updates & selection changes so toolbar active states stay live
  const [editorTick, setEditorTick] = useState(0)
  useEffect(() => {
    if (!editor) return
    const onTick = () => setEditorTick((n) => n + 1)
    editor.on('update', onTick)
    editor.on('selectionUpdate', onTick)
    return () => {
      editor.off('update', onTick)
      editor.off('selectionUpdate', onTick)
    }
  }, [editor])

  // Compose final HTML including active rich-text signature block (C9 compatible)
  const selectedSignature = useMemo(
    () => allSignatures.find((s) => String(s.id) === selectedSignatureId) ?? null,
    [allSignatures, selectedSignatureId],
  )

  const sanitizedSignatureHtml = useMemo(() => {
    if (!selectedSignature) return ''
    return sanitizePastedHtml(selectedSignature.html, {
      blockExternalImages: true,
      maxDataImageBytes: MAX_INLINE_IMAGE_BYTES,
      imagePlaceholder: (label) => t('compose.quote.blockedImage', { label }),
      fallbackImageLabel: t('compose.quote.imageFallback'),
    })
  }, [selectedSignature, t])

  const buildFinalHtml = useCallback(() => {
    const bodyHtml = editor?.getHTML() ?? ''
    if (!sanitizedSignatureHtml) return bodyHtml
    return `${bodyHtml}<div ${SIGNATURE_BLOCK_ATTR}="true" style="margin-top:16px;padding-top:12px;border-top:1px solid #e2e8f0;">${sanitizedSignatureHtml}</div>`
  }, [editor, sanitizedSignatureHtml])

  // Auto-save draft (debounced 2.5s)
  useEffect(() => {
    if (sendResult || !editor || uploadingImageNames.length > 0) return
    const timer = window.setTimeout(() => {
      const rawFinalHtml = buildFinalHtml()
      const { html, inlineImages: cidRefs } = prepareOutgoingHtml(rawFinalHtml)
      const text = htmlToMailPlainText(rawFinalHtml)
      const hasContent =
        to.length > 0 ||
        cc.length > 0 ||
        bcc.length > 0 ||
        subject.trim().length > 0 ||
        text.length > 0 ||
        cidRefs.length > 0
      if (!hasContent) return

      const nowIso = new Date().toISOString()
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
          ...(cidRefs.length > 0 ? { inline_images: cidRefs } : {}),
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
    buildFinalHtml,
    onSaveDraft,
    onDeleteDraft,
    onSentSuccess,
    sendResult,
    uploadingImageNames.length,
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

    const rawFinalHtml = buildFinalHtml()
    const { html, inlineImages: cidRefs } = prepareOutgoingHtml(rawFinalHtml)
    const text = htmlToMailPlainText(rawFinalHtml)

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
          ...(cidRefs.length > 0 ? { inline_images: cidRefs } : {}),
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
          attrs: attachments.map(({ name, data }) => ({ name, data })),
          ...(cidRefs.length > 0 ? { inline_images: cidRefs } : {}),
          ...(serverDraftIdRef.current ? { draft_id: serverDraftIdRef.current } : {}),
        })
      }

      onDeleteDraft(draftId)
      serverDraftIdRef.current = undefined
      setSendResult(result)
      onSentSuccess()

      const hasFailed = result.recipients.some((r) => r.status === 'failed')
      if (!hasFailed && result.status !== 'failed') {
        toast.success(
          result.status === 'queued'
            ? t('compose.result.titleQueued')
            : t('compose.result.titleSent'),
        )
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    } finally {
      setSending(false)
    }
  }, [
    attachments,
    bcc,
    buildFinalHtml,
    cc,
    context.mode,
    context.sourceEmail,
    draftId,
    fromAddress,
    includeOrigAttachments,
    onDeleteDraft,
    onSentSuccess,
    sending,
    subject,
    t,
    to,
    userDisplayName,
  ])

  // Keyboard shortcuts: Cmd/Ctrl + Enter to send, Cmd/Ctrl + K to toggle LinkBubble
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (minimized || sendResult) return
      if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
        e.preventDefault()
        void handleSend()
      } else if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setLinkBubbleOpen((v) => !v)
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

  const titleMap: Record<ComposeContext['mode'], string> = {
    new: t('compose.titleNew'),
    reply: t('compose.titleReply'),
    replyAll: t('compose.titleReplyAll'),
    forward: t('compose.titleForward'),
    draft: t('compose.titleDraft'),
    serverDraft: t('compose.titleDraft'),
  }

  const modalTitle = subject.trim()
    ? `${titleMap[context.mode]} · ${subject.trim()}`
    : titleMap[context.mode]

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
    ? 'fixed inset-x-0 top-0 z-50 flex h-[100dvh] flex-col bg-[var(--surface-elevated)] pt-safe pb-safe'
    : expanded
      ? 'fixed inset-6 z-50 mx-auto flex max-w-4xl flex-col overflow-hidden rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] shadow-[var(--shadow-lg)]'
      : 'fixed bottom-0 right-6 z-50 flex h-[min(680px,calc(100vh-3rem))] w-[min(680px,calc(100vw-3rem))] flex-col overflow-hidden rounded-t-[var(--radius-lg)] border border-b-0 border-[var(--border-subtle)] bg-[var(--surface-elevated)] shadow-[var(--shadow-lg)]'

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
        style={isMobile && mobileViewportHeight ? { height: `${mobileViewportHeight}px` } : undefined}
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
          <div className="pointer-events-none absolute inset-0 z-30 flex flex-col items-center justify-center gap-2 rounded-[var(--radius-lg)] border-2 border-dashed border-[var(--accent)] bg-[var(--accent)]/10 backdrop-blur-[2px]">
            <Upload className="h-8 w-8 text-[var(--accent)]" />
            <p className="text-[14px] font-medium text-[var(--accent-ink)]">
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
                    <SelectTrigger
                      id="compose_from"
                      className="h-7 border-0 bg-transparent px-1 shadow-none hover:bg-[var(--surface-hover)]"
                    >
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

            {/* Enhanced Rich-Text Formatting Toolbar + Inline Link Editor */}
            <EditorToolbar
              editor={editor}
              isMobile={isMobile}
              onToggleLinkBubble={() => setLinkBubbleOpen((v) => !v)}
              onTriggerImageUpload={() => imageInputRef.current?.click()}
            />
            {editor && (
              <LinkBubble
                editor={editor}
                open={linkBubbleOpen}
                onClose={() => setLinkBubbleOpen(false)}
              />
            )}

            {/* Hidden inline image file input (reserved C9 CID flow) */}
            <input
              ref={imageInputRef}
              type="file"
              accept="image/*"
              multiple
              className="hidden"
              onChange={(e) => {
                if (e.target.files?.length) {
                  for (const img of Array.from(e.target.files)) {
                    void insertInlineImageFile(img)
                  }
                  e.target.value = ''
                }
              }}
            />

            {/* TipTap Scrollable Editor + Live Signature Preview */}
            <div className="tiptap-editor mail-scroll min-h-0 flex-1 overflow-y-auto px-4 py-3">
              <EditorContent editor={editor} />

              {sanitizedSignatureHtml && (
                <div
                  className="mt-4 border-t border-dashed border-[var(--border-subtle)] pt-3 text-[12.5px] text-[var(--ink-secondary)]"
                  dangerouslySetInnerHTML={{
                    __html: sanitizedSignatureHtml,
                  }}
                />
              )}
            </div>

            {/* Attachments & Inline Image Indicator */}
            {((context.mode === 'forward' && (context.sourceEmail?.attachments?.length ?? 0) > 0) ||
              attachments.length > 0 ||
              inlineImages.length > 0 ||
              uploadingImageNames.length > 0) && (
              <div className="shrink-0 space-y-2 border-t border-[var(--border-hairline)] bg-[var(--surface-subtle)]/40 px-4 py-2">
                {uploadingImageNames.length > 0 && (
                  <div className="flex items-center gap-2 text-[12px] text-[var(--accent-ink)]">
                    <Loader2 className="h-3.5 w-3.5 animate-spin text-[var(--accent)]" />
                    <span className="truncate">
                      {t('compose.inlineImage.uploading', { name: uploadingImageNames[0] })}
                    </span>
                  </div>
                )}
                {context.mode === 'forward' &&
                  (context.sourceEmail?.attachments?.length ?? 0) > 0 && (
                    <label className="flex cursor-pointer items-center gap-2 text-[12.5px] text-[var(--ink-secondary)]">
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

                {inlineImages.length > 0 && (
                  <div className="flex flex-wrap items-center gap-2 text-[11.5px] text-[var(--ink-muted)]">
                    <Badge variant="accent">
                      <ImageIcon className="h-3 w-3" />
                      {t('compose.inlineImage.pendingCount', { count: inlineImages.length })}
                    </Badge>
                    <span className="truncate">{t('compose.inlineImage.reservedHint')}</span>
                  </div>
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
              <div className="flex min-w-0 items-center gap-1.5">
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
                  title={t('compose.addAttachment')}
                >
                  <Paperclip className="h-3.5 w-3.5" />
                  <span className="hidden sm:inline">{t('compose.addAttachment')}</span>
                </Button>

                {/* Per-Address Rich-Text Signature Selector (C9 /api/settings/signatures) */}
                {allSignatures.length > 0 && (
                  <label className="inline-flex min-w-0 items-center gap-1 rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)]/70 px-2 py-1 text-[12px] text-[var(--ink-secondary)]">
                    <PenLine className="h-3.5 w-3.5 shrink-0 text-[var(--ink-muted)]" />
                    <select
                      aria-label={t('compose.signature.label')}
                      value={selectedSignatureId}
                      onChange={(e) => {
                        const nextId = e.target.value
                        manualSignatureChoiceRef.current = true
                        setSelectedSignatureId(nextId)
                        if (nextId === 'none') {
                          toast(t('compose.signature.removed'))
                        } else {
                          const found = allSignatures.find((s) => String(s.id) === nextId)
                          if (found) {
                            toast.success(t('compose.signature.applied', { name: found.name }))
                          }
                        }
                      }}
                      className="max-w-[120px] truncate bg-transparent text-[12px] text-[var(--ink-primary)] outline-none sm:max-w-[150px]"
                    >
                      <option value="none">{t('compose.signature.none')}</option>
                      {allSignatures.map((sig) => (
                        <option key={sig.id} value={String(sig.id)}>
                          {sig.name}
                        </option>
                      ))}
                    </select>
                  </label>
                )}
              </div>

              <div className="flex shrink-0 items-center gap-2">
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
