import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Editor } from '@tiptap/react'
import { Check, ExternalLink, Link2, Unlink, X } from 'lucide-react'
import { Button } from '@/components/ui/button'

interface LinkBubbleProps {
  editor: Editor
  open: boolean
  onClose: () => void
}

function normalizeUrl(input: string): string {
  const trimmed = input.trim()
  if (!trimmed) return ''
  if (/^(?:https?:|mailto:|tel:)/i.test(trimmed)) return trimmed
  if (/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(trimmed)) return `mailto:${trimmed}`
  return `https://${trimmed}`
}

export function LinkBubble({ editor, open, onClose }: LinkBubbleProps) {
  const { t } = useTranslation()
  const inputRef = useRef<HTMLInputElement | null>(null)
  const existingHref = (editor.getAttributes('link').href as string | undefined) || ''
  const [url, setUrl] = useState(existingHref)

  useEffect(() => {
    if (open) {
      setUrl((editor.getAttributes('link').href as string | undefined) || '')
      window.setTimeout(() => inputRef.current?.focus(), 20)
    }
  }, [open, editor])

  if (!open) return null

  const applyLink = () => {
    const normalized = normalizeUrl(url)
    if (!normalized) {
      editor.chain().focus().extendMarkRange('link').unsetLink().run()
    } else {
      editor
        .chain()
        .focus()
        .extendMarkRange('link')
        .setLink({ href: normalized })
        .run()
    }
    onClose()
  }

  const removeLink = () => {
    editor.chain().focus().extendMarkRange('link').unsetLink().run()
    onClose()
  }

  const normalizedPreview = normalizeUrl(url)

  return (
    <div
      role="dialog"
      aria-label={t('compose.toolbar.linkEditorTitle')}
      className="flex flex-wrap items-center gap-1.5 border-b border-[var(--border-hairline)] bg-[var(--surface-elevated)] px-3 py-1.5 shadow-[var(--shadow-xs)]"
    >
      <Link2 className="h-3.5 w-3.5 shrink-0 text-[var(--accent-ink)]" />
      <input
        ref={inputRef}
        type="url"
        value={url}
        onChange={(e) => setUrl(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault()
            applyLink()
          } else if (e.key === 'Escape') {
            e.preventDefault()
            onClose()
          }
        }}
        placeholder={t('compose.toolbar.linkPlaceholder')}
        className="h-7 min-w-[180px] flex-1 rounded-[var(--radius-xs)] border border-[var(--border-subtle)] bg-[var(--surface-canvas)] px-2 text-[12.5px] text-[var(--ink-primary)] placeholder:text-[var(--ink-subtle)] outline-none focus:border-[var(--accent)]"
      />
      <Button type="button" variant="primary" size="xs" onClick={applyLink}>
        <Check className="h-3.5 w-3.5" />
        <span>{t('compose.toolbar.linkApply')}</span>
      </Button>
      {normalizedPreview && (
        <Button asChild type="button" variant="ghost" size="iconSm">
          <a
            href={normalizedPreview}
            target="_blank"
            rel="noopener noreferrer"
            title={t('compose.toolbar.linkOpen')}
            aria-label={t('compose.toolbar.linkOpen')}
          >
            <ExternalLink className="h-3.5 w-3.5" />
          </a>
        </Button>
      )}
      {existingHref && (
        <Button
          type="button"
          variant="dangerGhost"
          size="iconSm"
          onClick={removeLink}
          title={t('compose.toolbar.linkRemove')}
          aria-label={t('compose.toolbar.linkRemove')}
        >
          <Unlink className="h-3.5 w-3.5" />
        </Button>
      )}
      <Button
        type="button"
        variant="ghost"
        size="iconSm"
        onClick={onClose}
        title={t('common.close')}
        aria-label={t('common.close')}
      >
        <X className="h-3.5 w-3.5" />
      </Button>
    </div>
  )
}
