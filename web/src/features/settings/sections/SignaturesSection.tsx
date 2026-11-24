import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { EditorContent, useEditor } from '@tiptap/react'
import StarterKit from '@tiptap/starter-kit'
import { Placeholder } from '@tiptap/extensions'
import { Color, TextStyle } from '@tiptap/extension-text-style'
import { toast } from 'sonner'
import {
  AlertTriangle,
  Bold,
  Edit3,
  Italic,
  Link2,
  Loader2,
  PenLine,
  Plus,
  Redo2,
  Star,
  Trash2,
  Underline as UnderlineIcon,
  Undo2,
} from 'lucide-react'
import { signaturesApi, type SignatureInput, type SignatureRecord } from '@/api'
import { queryKeys, useAddressesQuery, useSignaturesQuery } from '@/hooks/useMailQueries'
import { sanitizePastedHtml } from '@/features/mail/editor/paste-sanitizer'
import { cn } from '@/lib/cn'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  EmptyState,
  SectionCard,
  SectionHeader,
  SectionLoadingState,
} from '@/components/settings'

const MAX_SIGNATURES = 20
const MAX_SIGNATURE_HTML_BYTES = 64 * 1024

export function SignaturesSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const { data: signatures = [], isLoading } = useSignaturesQuery(true)
  const { data: addresses = [] } = useAddressesQuery(true)

  const sendableAddresses = useMemo(
    () => addresses.filter((a) => a.can_send && (!a.status || a.status === 'active')),
    [addresses],
  )
  const addressMap = useMemo(
    () => new Map(addresses.map((a) => [a.id, a.address])),
    [addresses],
  )

  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<SignatureRecord | null>(null)
  const [deleting, setDeleting] = useState<SignatureRecord | null>(null)

  const [name, setName] = useState('')
  const [selectedAddressIds, setSelectedAddressIds] = useState<number[]>([])
  const [isDefault, setIsDefault] = useState(false)

  const editor = useEditor({
    extensions: [
      StarterKit.configure({
        heading: false,
        codeBlock: false,
        link: {
          openOnClick: false,
          autolink: true,
          HTMLAttributes: { target: '_blank', rel: 'noopener noreferrer' },
        },
      }),
      Placeholder.configure({
        placeholder: t('settings.signatures.contentPlaceholder'),
      }),
      TextStyle,
      Color,
    ],
    content: '',
    immediatelyRender: false,
    editorProps: {
      transformPastedHTML(html) {
        return sanitizePastedHtml(html, {
          blockExternalImages: true,
          imagePlaceholder: (label) => t('compose.quote.blockedImage', { label }),
          fallbackImageLabel: t('compose.quote.imageFallback'),
        })
      },
    },
  })

  useEffect(() => {
    if (!modalOpen || !editor) return
    editor.commands.setContent(editing?.html ?? '')
  }, [modalOpen, editing, editor])

  const openCreate = () => {
    setEditing(null)
    setName('')
    setSelectedAddressIds([])
    setIsDefault(signatures.length === 0)
    setModalOpen(true)
  }

  const openEdit = (sig: SignatureRecord) => {
    setEditing(sig)
    setName(sig.name)
    setSelectedAddressIds(sig.address_ids ?? [])
    setIsDefault(Boolean(sig.is_default))
    setModalOpen(true)
  }

  const saveMutation = useMutation({
    mutationFn: async () => {
      const trimmedName = name.trim()
      if (!trimmedName) {
        throw new Error(t('settings.signatures.validation.nameRequired'))
      }
      const rawHtml = editor?.getHTML() ?? ''
      const cleanHtml = sanitizePastedHtml(rawHtml, {
        blockExternalImages: true,
        imagePlaceholder: (label) => t('compose.quote.blockedImage', { label }),
        fallbackImageLabel: t('compose.quote.imageFallback'),
      })
      if (new Blob([cleanHtml]).size > MAX_SIGNATURE_HTML_BYTES) {
        throw new Error(t('settings.signatures.validation.contentTooLarge'))
      }
      const payload: SignatureInput = {
        name: trimmedName,
        html: cleanHtml,
        address_ids: selectedAddressIds,
        is_default: isDefault,
      }
      return editing
        ? signaturesApi.update(editing.id, payload)
        : signaturesApi.create(payload)
    },
    onSuccess: () => {
      setModalOpen(false)
      toast.success(t('settings.signatures.saved'))
      void queryClient.invalidateQueries({ queryKey: queryKeys.signatures })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const setDefaultMutation = useMutation({
    mutationFn: (sig: SignatureRecord) =>
      signaturesApi.update(sig.id, {
        name: sig.name,
        html: sig.html,
        address_ids: sig.address_ids,
        is_default: true,
      }),
    onSuccess: () => {
      toast.success(t('settings.signatures.saved'))
      void queryClient.invalidateQueries({ queryKey: queryKeys.signatures })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: number) => signaturesApi.remove(id),
    onSuccess: () => {
      setDeleting(null)
      toast.success(t('settings.signatures.deleted'))
      void queryClient.invalidateQueries({ queryKey: queryKeys.signatures })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const toggleAddressId = (id: number) => {
    setSelectedAddressIds((prev) =>
      prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id],
    )
  }

  const handleSetLink = () => {
    if (!editor) return
    const prev = String(editor.getAttributes('link').href || '')
    const url = window.prompt('URL', prev || 'https://')
    if (url === null) return
    const trimmed = url.trim()
    if (!trimmed) {
      editor.chain().focus().unsetLink().run()
      return
    }
    if (!/^https?:\/\//i.test(trimmed) && !trimmed.startsWith('mailto:')) {
      return
    }
    editor.chain().focus().setLink({ href: trimmed }).run()
  }

  const isAtLimit = signatures.length >= MAX_SIGNATURES

  return (
    <SectionCard>
      <SectionHeader
        icon={PenLine}
        title={t('settings.signatures.title')}
        description={t('settings.signatures.desc')}
        badge={
          <Badge variant={isAtLimit ? 'danger' : 'default'}>
            {t('settings.signatures.count', { count: signatures.length })}
          </Badge>
        }
        action={
          <Button size="sm" disabled={isAtLimit} onClick={openCreate}>
            <Plus className="h-3.5 w-3.5" />
            {t('settings.signatures.createButton')}
          </Button>
        }
      />

      {isAtLimit && (
        <div className="flex items-start gap-2 rounded-[var(--radius-sm)] border border-[var(--danger)]/35 bg-[var(--danger-soft)] px-3 py-2 text-[12.5px] text-[var(--danger)]">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
          <span>{t('settings.signatures.limitWarning')}</span>
        </div>
      )}

      {isLoading ? (
        <SectionLoadingState rows={2} rowHeight="h-24" />
      ) : signatures.length === 0 ? (
        <EmptyState
          icon={PenLine}
          title={t('settings.signatures.empty')}
          description={t('settings.signatures.emptyDesc')}
          action={
            <Button size="sm" disabled={isAtLimit} onClick={openCreate}>
              <Plus className="h-3.5 w-3.5" />
              {t('settings.signatures.createButton')}
            </Button>
          }
        />
      ) : (
        <div className="space-y-3">
          {signatures.map((sig) => {
            const safePreview = sanitizePastedHtml(sig.html, {
              blockExternalImages: true,
              imagePlaceholder: (label) => t('compose.quote.blockedImage', { label }),
              fallbackImageLabel: t('compose.quote.imageFallback'),
            })
            const boundEmails = (sig.address_ids ?? [])
              .map((id) => addressMap.get(id))
              .filter((x): x is string => Boolean(x))

            return (
              <div
                key={sig.id}
                className="rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-4 space-y-2.5"
              >
                <div className="flex flex-wrap items-start justify-between gap-2">
                  <div className="min-w-0 space-y-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-[14px] font-semibold text-[var(--ink-primary)]">
                        {sig.name}
                      </span>
                      {sig.is_default && (
                        <Badge variant="accent">
                          <Star className="h-3 w-3" />
                          {t('settings.signatures.defaultBadge')}
                        </Badge>
                      )}
                    </div>
                    <div className="flex flex-wrap items-center gap-1.5 text-[11.5px] text-[var(--ink-muted)]">
                      {boundEmails.length > 0 ? (
                        <>
                          <span>{t('settings.signatures.boundAddresses')}：</span>
                          {boundEmails.map((email) => (
                            <code
                              key={email}
                              className="rounded bg-[var(--surface-subtle)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--ink-secondary)]"
                            >
                              {email}
                            </code>
                          ))}
                        </>
                      ) : (
                        <span>{t('settings.signatures.allAddressesHint')}</span>
                      )}
                    </div>
                  </div>

                  <div className="flex shrink-0 items-center gap-1.5">
                    {!sig.is_default && (
                      <Button
                        type="button"
                        variant="ghost"
                        size="xs"
                        disabled={setDefaultMutation.isPending}
                        onClick={() => setDefaultMutation.mutate(sig)}
                      >
                        <Star className="h-3.5 w-3.5" />
                        {t('settings.signatures.setDefault')}
                      </Button>
                    )}
                    <Button
                      type="button"
                      variant="secondary"
                      size="xs"
                      onClick={() => openEdit(sig)}
                    >
                      <Edit3 className="h-3.5 w-3.5" />
                      {t('common.edit')}
                    </Button>
                    <Button
                      type="button"
                      variant="dangerGhost"
                      size="xs"
                      onClick={() => setDeleting(sig)}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                      {t('common.delete')}
                    </Button>
                  </div>
                </div>

                {/* Rendered Signature Preview */}
                <div
                  className="rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/50 px-3 py-2 text-[13px] leading-relaxed text-[var(--ink-primary)] [&_a]:text-[var(--accent)] [&_a]:underline"
                  dangerouslySetInnerHTML={{ __html: safePreview }}
                />
              </div>
            )
          })}
        </div>
      )}

      {/* Create / Edit Signature Dialog */}
      <Dialog open={modalOpen} onOpenChange={setModalOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>
              {editing
                ? t('settings.signatures.modalEdit')
                : t('settings.signatures.modalCreate')}
            </DialogTitle>
          </DialogHeader>

          <form
            onSubmit={(e) => {
              e.preventDefault()
              saveMutation.mutate()
            }}
            className="mt-3 space-y-4"
          >
            <div className="space-y-1.5">
              <Label htmlFor="sig_name">{t('settings.signatures.nameLabel')}</Label>
              <Input
                id="sig_name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder={t('settings.signatures.namePlaceholder')}
                autoFocus
              />
            </div>

            {/* Compact TipTap Editor */}
            <div className="space-y-1.5">
              <Label>{t('settings.signatures.contentLabel')}</Label>
              <div className="overflow-hidden rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-canvas)]">
                <div className="flex flex-wrap items-center gap-0.5 border-b border-[var(--border-hairline)] bg-[var(--surface-subtle)]/70 px-2 py-1">
                  <ToolbarBtn
                    active={Boolean(editor?.isActive('bold'))}
                    onClick={() => editor?.chain().focus().toggleBold().run()}
                    label={t('compose.toolbar.bold')}
                  >
                    <Bold className="h-3.5 w-3.5" />
                  </ToolbarBtn>
                  <ToolbarBtn
                    active={Boolean(editor?.isActive('italic'))}
                    onClick={() => editor?.chain().focus().toggleItalic().run()}
                    label={t('compose.toolbar.italic')}
                  >
                    <Italic className="h-3.5 w-3.5" />
                  </ToolbarBtn>
                  <ToolbarBtn
                    active={Boolean(editor?.isActive('underline'))}
                    onClick={() => editor?.chain().focus().toggleUnderline().run()}
                    label={t('compose.toolbar.underline')}
                  >
                    <UnderlineIcon className="h-3.5 w-3.5" />
                  </ToolbarBtn>
                  <ToolbarBtn
                    active={Boolean(editor?.isActive('link'))}
                    onClick={handleSetLink}
                    label={t('compose.toolbar.link')}
                  >
                    <Link2 className="h-3.5 w-3.5" />
                  </ToolbarBtn>
                  <span className="mx-1 h-4 w-px bg-[var(--border-hairline)]" />
                  <ToolbarBtn
                    active={false}
                    onClick={() => editor?.chain().focus().undo().run()}
                    label={t('compose.toolbar.undo')}
                  >
                    <Undo2 className="h-3.5 w-3.5" />
                  </ToolbarBtn>
                  <ToolbarBtn
                    active={false}
                    onClick={() => editor?.chain().focus().redo().run()}
                    label={t('compose.toolbar.redo')}
                  >
                    <Redo2 className="h-3.5 w-3.5" />
                  </ToolbarBtn>
                </div>

                <div className="tiptap-editor min-h-[120px] max-h-[240px] overflow-y-auto px-3 py-2.5 text-[13.5px]">
                  <EditorContent editor={editor} />
                </div>
              </div>
            </div>

            {/* Bind to Sender Addresses */}
            {sendableAddresses.length > 0 && (
              <div className="space-y-1.5">
                <Label>{t('settings.signatures.addressBindingLabel')}</Label>
                <div className="flex flex-wrap gap-2">
                  {sendableAddresses.map((addr) => (
                    <label
                      key={addr.id}
                      className="inline-flex cursor-pointer items-center gap-2 rounded-[var(--radius-sm)] border border-[var(--border-subtle)] bg-[var(--surface-canvas)] px-2.5 py-1.5 text-[12px]"
                    >
                      <Checkbox
                        checked={selectedAddressIds.includes(addr.id)}
                        onCheckedChange={() => toggleAddressId(addr.id)}
                      />
                      <span className="font-mono">{addr.address}</span>
                    </label>
                  ))}
                </div>
                <p className="text-[11.5px] text-[var(--ink-muted)]">
                  {t('settings.signatures.addressBindingHint')}
                </p>
              </div>
            )}

            {/* Global Default */}
            <div className="flex items-center justify-between rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/50 px-3 py-2.5">
              <span className="text-[13px] text-[var(--ink-primary)]">
                {t('settings.signatures.isDefaultLabel')}
              </span>
              <Switch checked={isDefault} onCheckedChange={setIsDefault} />
            </div>

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setModalOpen(false)}
              >
                {t('common.cancel')}
              </Button>
              <Button type="submit" size="sm" disabled={saveMutation.isPending}>
                {saveMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                {t('common.save')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Delete Confirm */}
      <Dialog open={deleting !== null} onOpenChange={(open) => !open && setDeleting(null)}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{t('settings.signatures.deleteTitle')}</DialogTitle>
            <DialogDescription>
              {deleting ? t('settings.signatures.deleteConfirm', { name: deleting.name }) : ''}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" size="sm" onClick={() => setDeleting(null)}>
              {t('common.cancel')}
            </Button>
            <Button
              variant="danger"
              size="sm"
              disabled={deleteMutation.isPending}
              onClick={() => deleting && deleteMutation.mutate(deleting.id)}
            >
              {t('common.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SectionCard>
  )
}

function ToolbarBtn({
  active,
  onClick,
  label,
  children,
}: {
  active: boolean
  onClick: () => void
  label: string
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={label}
      title={label}
      className={cn(
        'inline-flex h-7 w-7 items-center justify-center rounded-[var(--radius-xs)] transition-colors',
        active
          ? 'bg-[var(--accent-soft)] text-[var(--accent)]'
          : 'text-[var(--ink-secondary)] hover:bg-[var(--surface-hover)] hover:text-[var(--ink-primary)]',
      )}
    >
      {children}
    </button>
  )
}
