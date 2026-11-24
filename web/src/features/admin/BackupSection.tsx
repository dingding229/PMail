import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  AlertTriangle,
  DatabaseBackup,
  Download,
  Eye,
  EyeOff,
  Info,
  Loader2,
  RotateCcw,
  Trash2,
  Upload,
} from 'lucide-react'
import {
  adminBackupApi,
  type BackupFileItem,
  type RestoreStage,
  type RestoreStatusResponse,
} from '@/api'
import { queryKeys } from '@/hooks/useMailQueries'
import { formatBytes } from '@/lib/format'
import { formatMailFullTime } from '@/lib/date'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
  DataTable,
  DataTableBody,
  DataTableCell,
  DataTableHead,
  DataTableHeaderCell,
  DataTableRow,
  EmptyState,
  SectionCard,
  SectionErrorState,
  SectionFooter,
  SectionHeader,
  SectionLoadingState,
} from '@/components/settings'

const MIN_PASSPHRASE_LENGTH = 12

export function BackupSection() {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()

  // Create backup form state
  const [passphrase, setPassphrase] = useState('')
  const [confirmPassphrase, setConfirmPassphrase] = useState('')
  const [showPass, setShowPass] = useState(false)
  const [showConfirmPass, setShowConfirmPass] = useState(false)
  const [includeTls, setIncludeTls] = useState(true)

  // Upload state
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const [uploadPercent, setUploadPercent] = useState<number | null>(null)
  const [uploadingFileName, setUploadingFileName] = useState<string | null>(null)

  // Delete dialog
  const [deletingBackup, setDeletingBackup] = useState<BackupFileItem | null>(null)

  // Restore dialog & full-screen restore progress state
  const [restoringBackup, setRestoringBackup] = useState<BackupFileItem | null>(null)
  const [restorePassphrase, setRestorePassphrase] = useState('')
  const [showRestorePass, setShowRestorePass] = useState(false)
  const [restoreConfirmText, setRestoreConfirmText] = useState('')
  const [restoreError, setRestoreError] = useState<string | null>(null)
  const [restoreOverlayActive, setRestoreOverlayActive] = useState(false)
  const [restoreStatus, setRestoreStatus] = useState<RestoreStatusResponse | null>(null)
  const [waitingForRestart, setWaitingForRestart] = useState(false)

  const backupsQuery = useQuery({
    queryKey: queryKeys.adminBackups,
    queryFn: () => adminBackupApi.list(),
    refetchInterval: (query) =>
      query.state.data?.active_job?.state === 'running' ? 2000 : false,
  })

  const items = backupsQuery.data?.items ?? []
  const activeJob = backupsQuery.data?.active_job ?? null
  const isJobRunning = activeJob?.state === 'running'

  const passTooShort = passphrase.length > 0 && passphrase.length < MIN_PASSPHRASE_LENGTH
  const passMismatch = confirmPassphrase.length > 0 && passphrase !== confirmPassphrase
  const canCreateBackup =
    passphrase.length >= MIN_PASSPHRASE_LENGTH &&
    passphrase === confirmPassphrase &&
    !isJobRunning

  const createMutation = useMutation({
    mutationFn: () => adminBackupApi.create({ passphrase, include_tls: includeTls }),
    onSuccess: () => {
      toast.success(t('admin.backup.createStarted'))
      setPassphrase('')
      setConfirmPassphrase('')
      void queryClient.invalidateQueries({ queryKey: queryKeys.adminBackups })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (name: string) => adminBackupApi.remove(name),
    onSuccess: (_res, name) => {
      toast.success(t('admin.backup.deletedToast', { name }))
      setDeletingBackup(null)
      void queryClient.invalidateQueries({ queryKey: queryKeys.adminBackups })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const handleUploadFile = async (file: File) => {
    setUploadingFileName(file.name)
    setUploadPercent(0)
    try {
      const uploaded = await adminBackupApi.upload(file, (pct) => setUploadPercent(pct))
      toast.success(t('admin.backup.uploadedToast', { name: uploaded.name }))
      void queryClient.invalidateQueries({ queryKey: queryKeys.adminBackups })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    } finally {
      setUploadingFileName(null)
      setUploadPercent(null)
      if (fileInputRef.current) fileInputRef.current.value = ''
    }
  }

  const restoreMutation = useMutation({
    mutationFn: ({ name, pass }: { name: string; pass: string }) =>
      adminBackupApi.restore(name, { passphrase: pass, confirm: 'RESTORE' }),
    onSuccess: () => {
      setRestoreError(null)
      setRestoringBackup(null)
      setRestorePassphrase('')
      setRestoreConfirmText('')
      setWaitingForRestart(false)
      setRestoreStatus({ stage: 'verifying' })
      setRestoreOverlayActive(true)
    },
    onError: (err) => {
      const msg = err instanceof Error ? err.message : t('common.unknownError')
      setRestoreError(msg)
    },
  })

  // Poll restore-status every 2s while full-screen overlay is active;
  // switch to /api/ping when restarting or 503/network disconnect occurs.
  useEffect(() => {
    if (!restoreOverlayActive) return
    let cancelled = false

    const poll = async () => {
      if (cancelled) return
      if (waitingForRestart) {
        try {
          await adminBackupApi.ping()
          if (!cancelled) {
            window.location.assign('/login')
          }
        } catch {
          // Server still restarting
        }
        return
      }

      try {
        const res = await adminBackupApi.restoreStatus()
        if (cancelled) return
        setRestoreStatus(res)
        if (res.stage === 'restarting') {
          setWaitingForRestart(true)
        } else if (res.stage === 'done') {
          window.location.assign('/login')
        } else if (res.stage === 'failed') {
          setRestoreOverlayActive(false)
          toast.error(res.error || t('common.unknownError'))
        }
      } catch {
        // Service disconnected or returned 503 during restart
        if (!cancelled) {
          setWaitingForRestart(true)
          setRestoreStatus({ stage: 'restarting' })
        }
      }
    }

    const timer = window.setInterval(() => {
      void poll()
    }, 2000)

    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [restoreOverlayActive, waitingForRestart, t])

  const canConfirmRestore =
    restorePassphrase.length >= 1 &&
    restoreConfirmText.trim() === 'RESTORE' &&
    !restoreMutation.isPending

  const getRestoreStageLabel = (stage: RestoreStage): string =>
    t(`admin.backup.restoreStages.${stage}`)

  const jobPercent =
    activeJob && activeJob.tables_total > 0
      ? Math.min(100, Math.round((activeJob.tables_done / activeJob.tables_total) * 100))
      : 0

  return (
    <div className="space-y-5">
      {/* Card 1: Create Backup */}
      <SectionCard>
        <SectionHeader
          icon={DatabaseBackup}
          title={t('admin.backup.createTitle')}
          description={t('admin.backup.createDesc')}
        />

        <div className="flex items-start gap-2.5 rounded-[var(--radius-md)] border border-[var(--warning)]/35 bg-[var(--warning-soft)]/70 px-3.5 py-2.5 text-[12.5px] text-[var(--ink-primary)]">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-[var(--warning)]" />
          <span>{t('admin.backup.passphraseWarning')}</span>
        </div>

        <form
          onSubmit={(e) => {
            e.preventDefault()
            if (!canCreateBackup || createMutation.isPending) return
            createMutation.mutate()
          }}
          className="space-y-4"
        >
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="backup_passphrase">{t('admin.backup.passphraseLabel')}</Label>
              <div className="relative">
                <Input
                  id="backup_passphrase"
                  type={showPass ? 'text' : 'password'}
                  value={passphrase}
                  onChange={(e) => setPassphrase(e.target.value)}
                  placeholder={t('admin.backup.passphrasePlaceholder')}
                  autoComplete="new-password"
                  className="pr-9"
                />
                <button
                  type="button"
                  aria-label={showPass ? t('admin.backup.hidePassphrase') : t('admin.backup.showPassphrase')}
                  onClick={() => setShowPass((v) => !v)}
                  className="absolute right-2 top-1/2 -translate-y-1/2 rounded p-1 text-[var(--ink-muted)] hover:text-[var(--ink-primary)]"
                >
                  {showPass ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </button>
              </div>
              {passTooShort && (
                <p role="alert" className="text-[12px] text-[var(--danger)]">
                  {t('admin.backup.validation.passphraseMin')}
                </p>
              )}
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="backup_confirm_passphrase">
                {t('admin.backup.confirmPassphraseLabel')}
              </Label>
              <div className="relative">
                <Input
                  id="backup_confirm_passphrase"
                  type={showConfirmPass ? 'text' : 'password'}
                  value={confirmPassphrase}
                  onChange={(e) => setConfirmPassphrase(e.target.value)}
                  placeholder={t('admin.backup.confirmPassphrasePlaceholder')}
                  autoComplete="new-password"
                  className="pr-9"
                />
                <button
                  type="button"
                  aria-label={
                    showConfirmPass
                      ? t('admin.backup.hidePassphrase')
                      : t('admin.backup.showPassphrase')
                  }
                  onClick={() => setShowConfirmPass((v) => !v)}
                  className="absolute right-2 top-1/2 -translate-y-1/2 rounded p-1 text-[var(--ink-muted)] hover:text-[var(--ink-primary)]"
                >
                  {showConfirmPass ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </button>
              </div>
              {passMismatch && (
                <p role="alert" className="text-[12px] text-[var(--danger)]">
                  {t('admin.backup.validation.passphraseMismatch')}
                </p>
              )}
            </div>
          </div>

          <div className="flex items-center justify-between gap-4 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] px-4 py-3">
            <div className="space-y-0.5">
              <p className="text-[13px] font-medium text-[var(--ink-primary)]">
                {t('admin.backup.includeTlsLabel')}
              </p>
              <p className="text-[12px] text-[var(--ink-muted)]">
                {t('admin.backup.includeTlsDesc')}
              </p>
            </div>
            <Switch
              aria-label={t('admin.backup.includeTlsLabel')}
              checked={includeTls}
              onCheckedChange={setIncludeTls}
            />
          </div>

          {isJobRunning && activeJob && (
            <div
              role="status"
              aria-live="polite"
              className="space-y-2 rounded-[var(--radius-md)] border border-[var(--accent)]/30 bg-[var(--accent-soft)]/50 px-4 py-3"
            >
              <div className="flex items-center justify-between gap-2 text-[12.5px]">
                <span className="inline-flex items-center gap-2 font-medium text-[var(--ink-primary)]">
                  <Loader2 className="h-3.5 w-3.5 animate-spin text-[var(--accent)]" />
                  {t('admin.backup.jobProgress', {
                    done: activeJob.tables_done,
                    total: activeJob.tables_total,
                    rows: activeJob.rows_done.toLocaleString(),
                  })}
                </span>
                <span className="font-mono text-[12px] text-[var(--ink-secondary)]">
                  {jobPercent}%
                </span>
              </div>
              <div className="h-1.5 overflow-hidden rounded-full bg-[var(--surface-subtle)]">
                <div
                  className="h-full rounded-full bg-[var(--accent)] transition-all duration-300"
                  style={{ width: `${jobPercent}%` }}
                />
              </div>
            </div>
          )}

          <SectionFooter>
            <Button
              type="submit"
              size="sm"
              disabled={!canCreateBackup || createMutation.isPending}
            >
              {(createMutation.isPending || isJobRunning) && (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              )}
              {t('admin.backup.startBackup')}
            </Button>
          </SectionFooter>
        </form>
      </SectionCard>

      {/* Card 2: Upload Backup */}
      <SectionCard>
        <SectionHeader
          icon={Upload}
          title={t('admin.backup.uploadTitle')}
          description={t('admin.backup.uploadDesc')}
        />

        <div className="flex flex-wrap items-center gap-3">
          <input
            ref={fileInputRef}
            type="file"
            accept=".pmailbak"
            aria-label={t('admin.backup.uploadFileAria')}
            className="sr-only"
            onChange={(e) => {
              const file = e.target.files?.[0]
              if (file) void handleUploadFile(file)
            }}
          />
          <Button
            type="button"
            variant="secondary"
            size="sm"
            disabled={uploadPercent !== null}
            onClick={() => fileInputRef.current?.click()}
          >
            {uploadPercent !== null ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <Upload className="h-3.5 w-3.5" />
            )}
            {t('admin.backup.selectFileButton')}
          </Button>
          {uploadingFileName && uploadPercent !== null && (
            <span className="text-[12.5px] text-[var(--ink-secondary)]">
              {t('admin.backup.uploadingProgress', {
                name: uploadingFileName,
                percent: uploadPercent,
              })}
            </span>
          )}
        </div>

        {uploadPercent !== null && (
          <div className="h-1.5 overflow-hidden rounded-full bg-[var(--surface-subtle)]">
            <div
              className="h-full rounded-full bg-[var(--accent)] transition-all duration-200"
              style={{ width: `${uploadPercent}%` }}
            />
          </div>
        )}

        <div className="flex items-start gap-2 rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/60 px-3.5 py-2.5 text-[12px] text-[var(--ink-secondary)]">
          <Info className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[var(--ink-muted)]" />
          <span>{t('admin.backup.cloudflareLimitHint')}</span>
        </div>
      </SectionCard>

      {/* Card 3: Backup List */}
      <SectionCard>
        <SectionHeader
          icon={DatabaseBackup}
          title={t('admin.backup.listTitle')}
          description={t('admin.backup.listDesc')}
        />

        {backupsQuery.isLoading ? (
          <SectionLoadingState rows={3} rowHeight="h-12" />
        ) : backupsQuery.error ? (
          <SectionErrorState
            message={
              backupsQuery.error instanceof Error
                ? backupsQuery.error.message
                : t('common.unknownError')
            }
            retryLabel={t('app.retry')}
            onRetry={() => void backupsQuery.refetch()}
          />
        ) : items.length === 0 ? (
          <EmptyState icon={DatabaseBackup} title={t('admin.backup.emptyList')} compact />
        ) : (
          <>
            {/* Mobile card list (< md) */}
            <div className="space-y-3 md:hidden">
              {items.map((item) => {
                const isPreRestore = item.is_pre_restore || item.name.startsWith('pre-restore-')
                return (
                  <div
                    key={item.name}
                    className="space-y-2.5 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-3.5"
                  >
                    <div className="space-y-1">
                      <div className="flex flex-wrap items-center gap-1.5">
                        <span className="break-all font-mono text-[12.5px] font-medium text-[var(--ink-primary)]">
                          {item.name}
                        </span>
                        {isPreRestore && (
                          <Badge variant="warning">{t('admin.backup.preRestoreBadge')}</Badge>
                        )}
                      </div>
                      <p className="text-[12px] text-[var(--ink-secondary)]">
                        {formatBytes(item.size)} · {item.pmail_version}
                      </p>
                      <p className="text-[11.5px] text-[var(--ink-muted)]">
                        {formatMailFullTime(item.created_at, i18n.language)}
                      </p>
                    </div>
                    <div className="flex flex-wrap items-center gap-2 pt-1">
                      <Button asChild variant="outline" size="xs" className="flex-1">
                        <a href={adminBackupApi.downloadUrl(item.name)} download={item.name}>
                          <Download className="h-3 w-3" />
                          {t('admin.backup.download')}
                        </a>
                      </Button>
                      <Button
                        type="button"
                        variant="secondary"
                        size="xs"
                        className="flex-1"
                        onClick={() => {
                          setRestoringBackup(item)
                          setRestorePassphrase('')
                          setRestoreConfirmText('')
                          setRestoreError(null)
                        }}
                      >
                        <RotateCcw className="h-3 w-3" />
                        {t('admin.backup.restore')}
                      </Button>
                      <Button
                        type="button"
                        variant="dangerGhost"
                        size="iconSm"
                        aria-label={`${t('common.delete')} ${item.name}`}
                        onClick={() => setDeletingBackup(item)}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </div>
                )
              })}
            </div>

            {/* Desktop table (>= md) */}
            <DataTable className="hidden md:block">
              <DataTableHead>
                <tr>
                  <DataTableHeaderCell>{t('admin.backup.colFileName')}</DataTableHeaderCell>
                  <DataTableHeaderCell>{t('admin.backup.colSize')}</DataTableHeaderCell>
                  <DataTableHeaderCell>{t('admin.backup.colCreatedAt')}</DataTableHeaderCell>
                  <DataTableHeaderCell>{t('admin.backup.colVersion')}</DataTableHeaderCell>
                  <DataTableHeaderCell align="right">
                    {t('admin.backup.colActions')}
                  </DataTableHeaderCell>
                </tr>
              </DataTableHead>
              <DataTableBody>
                {items.map((item) => {
                  const isPreRestore = item.is_pre_restore || item.name.startsWith('pre-restore-')
                  return (
                    <DataTableRow key={item.name}>
                      <DataTableCell>
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="break-all font-mono text-[12.5px] font-medium text-[var(--ink-primary)]">
                            {item.name}
                          </span>
                          {isPreRestore && (
                            <Badge variant="warning">{t('admin.backup.preRestoreBadge')}</Badge>
                          )}
                        </div>
                      </DataTableCell>
                      <DataTableCell className="whitespace-nowrap font-mono text-[12.5px] text-[var(--ink-secondary)]">
                        {formatBytes(item.size)}
                      </DataTableCell>
                      <DataTableCell className="whitespace-nowrap text-[12.5px] text-[var(--ink-secondary)]">
                        {formatMailFullTime(item.created_at, i18n.language)}
                      </DataTableCell>
                      <DataTableCell className="whitespace-nowrap font-mono text-[12px] text-[var(--ink-secondary)]">
                        {item.pmail_version}
                      </DataTableCell>
                      <DataTableCell align="right" className="whitespace-nowrap">
                        <div className="inline-flex items-center gap-1.5">
                          <Button asChild variant="outline" size="xs">
                            <a href={adminBackupApi.downloadUrl(item.name)} download={item.name}>
                              <Download className="h-3 w-3" />
                              {t('admin.backup.download')}
                            </a>
                          </Button>
                          <Button
                            type="button"
                            variant="secondary"
                            size="xs"
                            onClick={() => {
                              setRestoringBackup(item)
                              setRestorePassphrase('')
                              setRestoreConfirmText('')
                              setRestoreError(null)
                            }}
                          >
                            <RotateCcw className="h-3 w-3" />
                            {t('admin.backup.restore')}
                          </Button>
                          <Button
                            type="button"
                            variant="dangerGhost"
                            size="iconSm"
                            aria-label={`${t('common.delete')} ${item.name}`}
                            onClick={() => setDeletingBackup(item)}
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </Button>
                        </div>
                      </DataTableCell>
                    </DataTableRow>
                  )
                })}
              </DataTableBody>
            </DataTable>
          </>
        )}
      </SectionCard>

      {/* Delete Confirm Dialog */}
      <Dialog
        open={deletingBackup !== null}
        onOpenChange={(open) => !open && setDeletingBackup(null)}
      >
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{t('admin.backup.deleteTitle')}</DialogTitle>
            <DialogDescription>
              {deletingBackup
                ? t('admin.backup.deleteConfirm', { name: deletingBackup.name })
                : ''}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setDeletingBackup(null)}
            >
              {t('common.cancel')}
            </Button>
            <Button
              type="button"
              variant="danger"
              size="sm"
              disabled={deleteMutation.isPending}
              onClick={() => deletingBackup && deleteMutation.mutate(deletingBackup.name)}
            >
              {deleteMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
              {t('common.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Restore Confirm Dialog */}
      <Dialog
        open={restoringBackup !== null}
        onOpenChange={(open) => {
          if (!open) {
            setRestoringBackup(null)
            setRestoreError(null)
          }
        }}
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>{t('admin.backup.restoreDialogTitle')}</DialogTitle>
            <DialogDescription>
              {restoringBackup ? restoringBackup.name : ''}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <div className="flex items-start gap-2.5 rounded-[var(--radius-md)] border border-[var(--danger)]/40 bg-[var(--danger-soft)] px-3.5 py-3 text-[12.5px] text-[var(--danger)]">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
              <span>{t('admin.backup.restoreDangerWarning')}</span>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="restore_passphrase">
                {t('admin.backup.restorePassphraseLabel')}
              </Label>
              <div className="relative">
                <Input
                  id="restore_passphrase"
                  type={showRestorePass ? 'text' : 'password'}
                  value={restorePassphrase}
                  onChange={(e) => setRestorePassphrase(e.target.value)}
                  placeholder={t('admin.backup.restorePassphrasePlaceholder')}
                  className="pr-9"
                />
                <button
                  type="button"
                  aria-label={
                    showRestorePass
                      ? t('admin.backup.hidePassphrase')
                      : t('admin.backup.showPassphrase')
                  }
                  onClick={() => setShowRestorePass((v) => !v)}
                  className="absolute right-2 top-1/2 -translate-y-1/2 rounded p-1 text-[var(--ink-muted)] hover:text-[var(--ink-primary)]"
                >
                  {showRestorePass ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </button>
              </div>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="restore_confirm_input">
                {t('admin.backup.restoreConfirmInputLabel')}
              </Label>
              <Input
                id="restore_confirm_input"
                value={restoreConfirmText}
                onChange={(e) => setRestoreConfirmText(e.target.value)}
                placeholder={'RESTORE'}
                autoComplete="off"
                className="font-mono"
              />
            </div>

            {restoreError && (
              <p role="alert" className="text-[12.5px] font-medium text-[var(--danger)]">
                {restoreError}
              </p>
            )}
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setRestoringBackup(null)}
            >
              {t('common.cancel')}
            </Button>
            <Button
              type="button"
              variant="danger"
              size="sm"
              disabled={!canConfirmRestore}
              onClick={() =>
                restoringBackup &&
                restoreMutation.mutate({
                  name: restoringBackup.name,
                  pass: restorePassphrase,
                })
              }
            >
              {restoreMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
              {t('admin.backup.confirmRestore')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Non-closable Full-Screen Restoring Overlay */}
      {restoreOverlayActive && (
        <div
          role="dialog"
          aria-modal="true"
          aria-label={t('admin.backup.restoringTitle')}
          className="fixed inset-0 z-[100] flex items-center justify-center bg-[var(--surface-canvas)]/95 px-4 backdrop-blur-sm"
        >
          <div className="w-full max-w-md rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] p-8 text-center shadow-[var(--shadow-lg)]">
            <Loader2 className="mx-auto h-9 w-9 animate-spin text-[var(--accent)]" />
            <h2 className="mt-4 text-[17px] font-semibold text-[var(--ink-primary)]">
              {waitingForRestart
                ? t('admin.backup.restartingTitle')
                : t('admin.backup.restoringTitle')}
            </h2>
            <p className="mt-2 text-[13px] text-[var(--ink-secondary)]">
              {waitingForRestart
                ? t('admin.backup.restartingDesc')
                : getRestoreStageLabel(restoreStatus?.stage ?? 'verifying')}
            </p>
            {!waitingForRestart &&
              restoreStatus?.tables_total != null &&
              restoreStatus.tables_total > 0 && (
                <p className="mt-2 font-mono text-[12px] text-[var(--ink-muted)]">
                  {t('admin.backup.jobProgress', {
                    done: restoreStatus.tables_done ?? 0,
                    total: restoreStatus.tables_total,
                    rows: (restoreStatus.rows_done ?? 0).toLocaleString(),
                  })}
                </p>
              )}
          </div>
        </div>
      )}
    </div>
  )
}
