import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Download,
  Eye,
  FileArchive,
  FileCode2,
  FileImage,
  FileSpreadsheet,
  FileText,
  Loader2,
  Paperclip,
} from 'lucide-react'
import { mailApi, type AttachmentPreviewResponse } from '@/api'
import { formatBytes, type NormalizedAttachmentItem } from '@/lib/format'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { CodeBlockView } from './CodeBlockView'

interface AttachmentSectionProps {
  emailId: number
  attachments: NormalizedAttachmentItem[]
}

function getFileIcon(contentType: string, filename: string) {
  const lower = `${contentType} ${filename}`.toLowerCase()
  if (lower.includes('image/')) return FileImage
  if (
    lower.includes('text/') ||
    lower.includes('json') ||
    lower.includes('yaml') ||
    /\.(ts|tsx|js|jsx|go|py|rs|sh|sql|md|diff|patch|yml|yaml|json|xml|css|html)$/i.test(filename)
  ) {
    return FileCode2
  }
  if (lower.includes('zip') || lower.includes('tar') || lower.includes('gz') || lower.includes('rar')) {
    return FileArchive
  }
  if (lower.includes('sheet') || lower.includes('csv') || lower.includes('excel')) {
    return FileSpreadsheet
  }
  return FileText
}

function isImageFile(att: NormalizedAttachmentItem): boolean {
  return (
    att.contentType.toLowerCase().startsWith('image/') ||
    /\.(png|jpe?g|gif|webp|svg)$/i.test(att.filename)
  )
}

function isTextPreviewable(att: NormalizedAttachmentItem): boolean {
  if (att.previewable === true) return true
  const ct = att.contentType.toLowerCase()
  return (
    ct.startsWith('text/') ||
    ct.includes('json') ||
    ct.includes('xml') ||
    ct.includes('javascript') ||
    /\.(txt|md|log|json|ya?ml|toml|go|ts|tsx|js|jsx|py|sh|sql|css|html|diff|patch|csv)$/i.test(
      att.filename,
    )
  )
}

export function AttachmentSection({ emailId, attachments }: AttachmentSectionProps) {
  const { t } = useTranslation()
  const [previewAtt, setPreviewAtt] = useState<NormalizedAttachmentItem | null>(null)
  const [previewData, setPreviewData] = useState<AttachmentPreviewResponse | null>(null)
  const [previewLoading, setPreviewLoading] = useState(false)
  const [previewError, setPreviewError] = useState<string | null>(null)

  const visibleAttachments = attachments.filter((a) => !a.inline)
  if (visibleAttachments.length === 0) return null

  const openPreview = async (att: NormalizedAttachmentItem) => {
    setPreviewAtt(att)
    setPreviewData(null)
    setPreviewError(null)

    if (isImageFile(att)) return

    if (isTextPreviewable(att)) {
      setPreviewLoading(true)
      try {
        const res = await mailApi.previewAttachment(emailId, att.index)
        setPreviewData(res)
      } catch (err) {
        setPreviewError(err instanceof Error ? err.message : t('mail.attachments.previewUnavailable'))
      } finally {
        setPreviewLoading(false)
      }
    }
  }

  return (
    <section className="mt-6 border-t border-[var(--border-hairline)] pt-4">
      <div className="mb-2.5 flex items-center gap-1.5 text-[12.5px] font-medium text-[var(--ink-secondary)]">
        <Paperclip className="h-3.5 w-3.5" />
        <span>{t('mail.attachments.title', { count: visibleAttachments.length })}</span>
      </div>

      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
        {visibleAttachments.map((att) => {
          const Icon = getFileIcon(att.contentType, att.filename)
          const downloadUrl = mailApi.attachmentDownloadUrl(emailId, att.index)
          const canPreview = isImageFile(att) || isTextPreviewable(att)

          return (
            <div
              key={`${att.index}-${att.filename}`}
              className="group flex items-center gap-3 rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-subtle)]/60 px-3 py-2.5 transition-colors hover:border-[var(--border-strong)] hover:bg-[var(--surface-hover)]"
            >
              <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-[var(--radius-sm)] bg-[var(--surface-canvas)] text-[var(--accent)] shadow-[var(--shadow-xs)]">
                <Icon className="h-4 w-4" />
              </div>

              <div className="min-w-0 flex-1">
                <p className="truncate text-[13px] font-medium text-[var(--ink-primary)]" title={att.filename}>
                  {att.filename}
                </p>
                <p className="truncate text-[11.5px] text-[var(--ink-muted)]">
                  {att.size ? `${formatBytes(att.size)} · ` : ''}
                  {att.contentType || 'application/octet-stream'}
                </p>
              </div>

              <div className="flex items-center gap-1 shrink-0">
                {canPreview && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="iconSm"
                    onClick={() => void openPreview(att)}
                    aria-label={t('mail.attachments.preview')}
                    title={t('mail.attachments.preview')}
                  >
                    <Eye className="h-3.5 w-3.5" />
                  </Button>
                )}
                <Button
                  asChild
                  variant="ghost"
                  size="iconSm"
                  aria-label={t('mail.attachments.download')}
                  title={t('mail.attachments.download')}
                >
                  <a href={downloadUrl} download={att.filename}>
                    <Download className="h-3.5 w-3.5" />
                  </a>
                </Button>
              </div>
            </div>
          )
        })}
      </div>

      {/* Attachment preview modal */}
      <Dialog open={previewAtt !== null} onOpenChange={(open) => !open && setPreviewAtt(null)}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle className="truncate">{previewAtt?.filename}</DialogTitle>
            <DialogDescription>
              {previewAtt?.contentType}
              {previewAtt?.size ? ` · ${formatBytes(previewAtt.size)}` : ''}
            </DialogDescription>
          </DialogHeader>

          <div className="mt-3 max-h-[68vh] overflow-auto">
            {previewAtt && isImageFile(previewAtt) ? (
              <div className="rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)] p-3">
                <img
                  src={mailApi.attachmentInlineUrl(emailId, previewAtt.index)}
                  alt={previewAtt.filename}
                  className="mx-auto max-h-[58vh] rounded-[var(--radius-sm)] object-contain"
                />
              </div>
            ) : previewLoading ? (
              <div className="flex items-center justify-center gap-2 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)] py-12 text-[13px] text-[var(--ink-muted)]">
                <Loader2 className="h-4 w-4 animate-spin" />
                <span>{t('mail.attachments.previewLoading')}</span>
              </div>
            ) : previewData ? (
              <div className="space-y-2">
                {previewData.truncated && (
                  <div className="flex flex-wrap items-center justify-between gap-2 rounded-[var(--radius-sm)] border border-[var(--warning)]/30 bg-[var(--warning-soft)] px-3 py-2 text-[12px] text-[var(--warning)]">
                    <span>{t('mail.attachments.truncatedHint')}</span>
                    {previewAtt && (
                      <a
                        href={mailApi.attachmentDownloadUrl(emailId, previewAtt.index)}
                        download={previewAtt.filename}
                        className="font-semibold underline underline-offset-2"
                      >
                        {t('mail.attachments.downloadFull')}
                      </a>
                    )}
                  </div>
                )}
                <CodeBlockView
                  code={previewData.content}
                  language={previewData.language || undefined}
                  maxHeightClass="max-h-[54vh]"
                  className="my-0"
                />
              </div>
            ) : (
              <p className="rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)] py-10 text-center text-[13px] text-[var(--ink-muted)]">
                {previewError || t('mail.attachments.previewUnavailable')}
              </p>
            )}
          </div>

          {previewAtt && (
            <div className="mt-3 flex justify-end">
              <Button asChild size="sm">
                <a
                  href={mailApi.attachmentDownloadUrl(emailId, previewAtt.index)}
                  download={previewAtt.filename}
                >
                  <Download className="h-3.5 w-3.5" />
                  {t('mail.attachments.download')}
                </a>
              </Button>
            </div>
          )}
        </DialogContent>
      </Dialog>
    </section>
  )
}
