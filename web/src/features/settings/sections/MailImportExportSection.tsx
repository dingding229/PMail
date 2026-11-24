import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  AlertTriangle,
  CheckCircle2,
  Download,
  FileArchive,
  Info,
  Loader2,
  Upload,
} from 'lucide-react'
import {
  mailImportExportApi,
  SYSTEM_GROUP_IDS,
  type MailExportJobStatus,
  type MailImportJobStatus,
} from '@/api'
import { queryKeys, useGroupOptionsQuery } from '@/hooks/useMailQueries'
import { formatBytes } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  SectionCard,
  SectionFooter,
  SectionHeader,
} from '@/components/settings'

interface FolderChoice {
  tag: string
  label: string
}

export function MailImportExportSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { data: groups = [] } = useGroupOptionsQuery(true)

  const customFolders = useMemo<FolderChoice[]>(
    () =>
      groups
        .filter((g) => g.id < SYSTEM_GROUP_IDS.INBOX)
        .map((g) => ({
          tag: `group:${g.id}`,
          label: g.full_path ? g.full_path.replace(/^\//, '') : g.name,
        })),
    [groups],
  )

  const allFolderChoices = useMemo<FolderChoice[]>(
    () => [
      { tag: 'inbox', label: t('nav.inbox') },
      { tag: 'sent', label: t('nav.sent') },
      { tag: 'drafts', label: t('nav.drafts') },
      { tag: 'junk', label: t('nav.junk') },
      ...customFolders,
    ],
    [customFolders, t],
  )

  // Export state
  const [deselectedTags, setDeselectedTags] = useState<Set<string>>(new Set())
  const [includeTrash, setIncludeTrash] = useState(false)
  const [exportJob, setExportJob] = useState<MailExportJobStatus | null>(null)

  const selectedTags = useMemo(
    () => allFolderChoices.filter((f) => !deselectedTags.has(f.tag)).map((f) => f.tag),
    [allFolderChoices, deselectedTags],
  )

  const toggleFolderTag = (tag: string) => {
    setDeselectedTags((prev) => {
      const next = new Set(prev)
      if (next.has(tag)) next.delete(tag)
      else next.add(tag)
      return next
    })
  }

  const startExportMutation = useMutation({
    mutationFn: () =>
      mailImportExportApi.startExport({
        folders: selectedTags.length === allFolderChoices.length ? [] : selectedTags,
        format: 'mbox',
        include_trash: includeTrash,
      }),
    onSuccess: (job) => {
      setExportJob(job)
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  // Poll export progress
  useEffect(() => {
    if (!exportJob || exportJob.state !== 'running') return
    let cancelled = false
    const timer = window.setInterval(() => {
      void mailImportExportApi
        .exportStatus(exportJob.id)
        .then((next) => {
          if (!cancelled) setExportJob(next)
        })
        .catch(() => {
          // keep polling
        })
    }, 1500)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [exportJob])

  // Import state
  const importFileInputRef = useRef<HTMLInputElement | null>(null)
  const [importTarget, setImportTarget] = useState('inbox')
  const [importUploadPercent, setImportUploadPercent] = useState<number | null>(null)
  const [importFileName, setImportFileName] = useState<string | null>(null)
  const [importJob, setImportJob] = useState<MailImportJobStatus | null>(null)

  const handleImportFile = async (file: File) => {
    setImportFileName(file.name)
    setImportUploadPercent(0)
    setImportJob(null)
    try {
      const job = await mailImportExportApi.startImport(file, importTarget, (pct) =>
        setImportUploadPercent(pct),
      )
      setImportJob(job)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    } finally {
      setImportUploadPercent(null)
      if (importFileInputRef.current) importFileInputRef.current.value = ''
    }
  }

  // Poll import progress & invalidate mail queries on completion
  useEffect(() => {
    if (!importJob || importJob.state !== 'running') return
    let cancelled = false
    const timer = window.setInterval(() => {
      void mailImportExportApi
        .importStatus(importJob.id)
        .then((next) => {
          if (cancelled) return
          setImportJob(next)
          if (next.state === 'done') {
            void queryClient.invalidateQueries({ queryKey: queryKeys.emailListsRoot })
            void queryClient.invalidateQueries({ queryKey: queryKeys.sidebarGroups })
            void queryClient.invalidateQueries({ queryKey: queryKeys.addressCounts })
            void queryClient.invalidateQueries({ queryKey: queryKeys.userMe })
            toast.success(
              t('settings.importExport.importDoneToast', { count: next.imported }),
            )
          }
        })
        .catch(() => {
          // keep polling
        })
    }, 1500)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [importJob, queryClient, t])

  const exportPercent =
    exportJob && exportJob.total_count > 0
      ? Math.min(100, Math.round((exportJob.exported_count / exportJob.total_count) * 100))
      : 0

  return (
    <div className="space-y-5">
      {/* Card 1: Mail Export */}
      <SectionCard>
        <SectionHeader
          icon={Download}
          title={t('settings.importExport.exportTitle')}
          description={t('settings.importExport.exportDesc')}
        />

        <div className="space-y-2.5">
          <Label>{t('settings.importExport.foldersLabel')}</Label>
          <div className="flex flex-wrap gap-2">
            {allFolderChoices.map((f) => {
              const checked = !deselectedTags.has(f.tag)
              return (
                <label
                  key={f.tag}
                  className="inline-flex cursor-pointer items-center gap-2 rounded-[var(--radius-sm)] border border-[var(--border-subtle)] bg-[var(--surface-canvas)] px-3 py-1.5 text-[12.5px]"
                >
                  <Checkbox
                    aria-label={f.label}
                    checked={checked}
                    onCheckedChange={() => toggleFolderTag(f.tag)}
                  />
                  <span>{f.label}</span>
                </label>
              )
            })}
          </div>
        </div>

        <div className="flex items-center justify-between gap-4 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] px-4 py-3">
          <div className="space-y-0.5">
            <p className="text-[13px] font-medium text-[var(--ink-primary)]">
              {t('settings.importExport.includeTrashLabel')}
            </p>
            <p className="text-[12px] text-[var(--ink-muted)]">
              {t('settings.importExport.includeTrashDesc')}
            </p>
          </div>
          <Switch
            aria-label={t('settings.importExport.includeTrashLabel')}
            checked={includeTrash}
            onCheckedChange={setIncludeTrash}
          />
        </div>

        {exportJob && (
          <div
            role="status"
            aria-live="polite"
            className="space-y-2.5 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] px-4 py-3.5"
          >
            {exportJob.state === 'running' ? (
              <>
                <div className="flex items-center justify-between gap-2 text-[12.5px]">
                  <span className="inline-flex items-center gap-2 font-medium text-[var(--ink-primary)]">
                    <Loader2 className="h-3.5 w-3.5 animate-spin text-[var(--accent)]" />
                    {t('settings.importExport.exportRunning', {
                      done: exportJob.exported_count,
                      total: exportJob.total_count,
                    })}
                  </span>
                  <span className="font-mono text-[12px] text-[var(--ink-secondary)]">
                    {exportPercent}%
                  </span>
                </div>
                <div className="h-1.5 overflow-hidden rounded-full bg-[var(--surface-subtle)]">
                  <div
                    className="h-full rounded-full bg-[var(--accent)] transition-all duration-300"
                    style={{ width: `${exportPercent}%` }}
                  />
                </div>
              </>
            ) : exportJob.state === 'done' ? (
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="space-y-0.5">
                  <p className="inline-flex items-center gap-1.5 text-[13px] font-medium text-[var(--success)]">
                    <CheckCircle2 className="h-4 w-4" />
                    {t('settings.importExport.exportReady', {
                      count: exportJob.exported_count,
                    })}
                    {exportJob.size ? ` · ${formatBytes(exportJob.size)}` : ''}
                  </p>
                  <p className="text-[12px] text-[var(--ink-muted)]">
                    {t('settings.importExport.exportExpiryHint')}
                  </p>
                </div>
                <Button asChild size="sm">
                  <a href={mailImportExportApi.exportDownloadUrl(exportJob.id)} download>
                    <Download className="h-3.5 w-3.5" />
                    {t('settings.importExport.downloadExport')}
                  </a>
                </Button>
              </div>
            ) : (
              <p className="text-[12.5px] text-[var(--danger)]">
                {exportJob.error || t('common.unknownError')}
              </p>
            )}
          </div>
        )}

        <SectionFooter>
          <Button
            type="button"
            size="sm"
            disabled={
              selectedTags.length === 0 ||
              startExportMutation.isPending ||
              exportJob?.state === 'running'
            }
            onClick={() => startExportMutation.mutate()}
          >
            {(startExportMutation.isPending || exportJob?.state === 'running') && (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            )}
            <FileArchive className="h-3.5 w-3.5" />
            {t('settings.importExport.startExport')}
          </Button>
        </SectionFooter>
      </SectionCard>

      {/* Card 2: Mail Import */}
      <SectionCard>
        <SectionHeader
          icon={Upload}
          title={t('settings.importExport.importTitle')}
          description={t('settings.importExport.importDesc')}
        />

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 sm:items-end">
          <div className="space-y-1.5">
            <Label htmlFor="import_target_folder">
              {t('settings.importExport.targetFolderLabel')}
            </Label>
            <Select value={importTarget} onValueChange={setImportTarget}>
              <SelectTrigger id="import_target_folder" className="h-9">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {allFolderChoices.map((f) => (
                  <SelectItem key={f.tag} value={f.tag}>
                    {f.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <input
              ref={importFileInputRef}
              type="file"
              accept=".eml,.mbox,.zip"
              aria-label={t('settings.importExport.importFileAria')}
              className="sr-only"
              onChange={(e) => {
                const file = e.target.files?.[0]
                if (file) void handleImportFile(file)
              }}
            />
            <Button
              type="button"
              size="sm"
              disabled={importUploadPercent !== null || importJob?.state === 'running'}
              onClick={() => importFileInputRef.current?.click()}
            >
              {importUploadPercent !== null || importJob?.state === 'running' ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Upload className="h-3.5 w-3.5" />
              )}
              {t('settings.importExport.selectImportFile')}
            </Button>
            {importFileName && (
              <span className="max-w-[200px] truncate font-mono text-[12px] text-[var(--ink-muted)]">
                {importFileName}
              </span>
            )}
          </div>
        </div>

        {importUploadPercent !== null && (
          <div className="space-y-1">
            <p className="text-[12px] text-[var(--ink-secondary)]">
              {t('settings.importExport.uploading', { percent: importUploadPercent })}
            </p>
            <div className="h-1.5 overflow-hidden rounded-full bg-[var(--surface-subtle)]">
              <div
                className="h-full rounded-full bg-[var(--accent)] transition-all duration-200"
                style={{ width: `${importUploadPercent}%` }}
              />
            </div>
          </div>
        )}

        {importJob && (
          <div
            role="status"
            aria-live="polite"
            className="space-y-3 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-4"
          >
            <div className="flex items-center justify-between gap-2 text-[13px] font-medium text-[var(--ink-primary)]">
              <span className="inline-flex items-center gap-2">
                {importJob.state === 'running' ? (
                  <Loader2 className="h-4 w-4 animate-spin text-[var(--accent)]" />
                ) : importJob.state === 'done' ? (
                  <CheckCircle2 className="h-4 w-4 text-[var(--success)]" />
                ) : (
                  <AlertTriangle className="h-4 w-4 text-[var(--danger)]" />
                )}
                {importJob.state === 'running'
                  ? t('settings.importExport.importRunning')
                  : importJob.state === 'done'
                    ? t('settings.importExport.importDone')
                    : t('settings.importExport.importFailed')}
              </span>
            </div>

            <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
              <div className="rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/50 px-3 py-2">
                <p className="text-[11px] text-[var(--ink-muted)]">
                  {t('settings.importExport.statTotal')}
                </p>
                <p data-import-stat="total" className="mt-0.5 font-mono text-[16px] font-semibold">
                  {importJob.total}
                </p>
              </div>
              <div className="rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/50 px-3 py-2">
                <p className="text-[11px] text-[var(--ink-muted)]">
                  {t('settings.importExport.statImported')}
                </p>
                <p
                  data-import-stat="imported"
                  className="mt-0.5 font-mono text-[16px] font-semibold text-[var(--success)]"
                >
                  {importJob.imported}
                </p>
              </div>
              <div className="rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/50 px-3 py-2">
                <p className="text-[11px] text-[var(--ink-muted)]">
                  {t('settings.importExport.statSkipped')}
                </p>
                <p
                  data-import-stat="skipped"
                  className="mt-0.5 font-mono text-[16px] font-semibold text-[var(--ink-secondary)]"
                >
                  {importJob.skipped}
                </p>
              </div>
              <div className="rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/50 px-3 py-2">
                <p className="text-[11px] text-[var(--ink-muted)]">
                  {t('settings.importExport.statFailed')}
                </p>
                <p
                  data-import-stat="failed"
                  className="mt-0.5 font-mono text-[16px] font-semibold text-[var(--danger)]"
                >
                  {importJob.failed}
                </p>
              </div>
            </div>

            {importJob.error_code === 'quota_exceeded' && (
              <div className="flex items-start gap-2 rounded-[var(--radius-sm)] border border-[var(--danger)]/35 bg-[var(--danger-soft)] px-3 py-2 text-[12px] text-[var(--danger)]">
                <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                <span>{t('settings.importExport.errors.quotaExceeded')}</span>
              </div>
            )}
          </div>
        )}

        <div className="flex items-start gap-2 rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/60 px-3.5 py-2.5 text-[12px] text-[var(--ink-secondary)]">
          <Info className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[var(--ink-muted)]" />
          <span>{t('settings.importExport.importNoSideEffectsHint')}</span>
        </div>
      </SectionCard>
    </div>
  )
}
