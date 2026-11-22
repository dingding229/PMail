import { useMemo, useState } from 'react'
import { useForm, Controller } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  AlertTriangle,
  CheckCircle2,
  Edit3,
  Loader2,
  MailPlus,
  PlugZap,
  Plus,
  RefreshCw,
  Trash2,
} from 'lucide-react'
import {
  externalApi,
  SYSTEM_GROUP_IDS,
  type ExternalAccount,
  type ExternalPreset,
  type ExternalProtocol,
  type ExternalTlsMode,
} from '@/api'
import { formatMailFullTime, pickDate } from '@/lib/date'
import { queryKeys, useGroupOptionsQuery } from '@/hooks/useMailQueries'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

export function ExternalMailSection() {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()

  const { data: accounts = [], isLoading } = useQuery({
    queryKey: queryKeys.externalAccounts,
    queryFn: () => externalApi.list(),
  })
  const { data: presets = [] } = useQuery({
    queryKey: queryKeys.externalPresets,
    queryFn: () => externalApi.presets(),
    staleTime: 10 * 60_000,
  })
  const { data: groups = [] } = useGroupOptionsQuery(true)

  const customGroups = useMemo(
    () => groups.filter((g) => g.id < SYSTEM_GROUP_IDS.INBOX),
    [groups],
  )

  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<ExternalAccount | null>(null)
  const [deleting, setDeleting] = useState<ExternalAccount | null>(null)
  const [testedFolders, setTestedFolders] = useState<string[] | null>(null)

  const schema = useMemo(
    () =>
      z.object({
        name: z.string().trim(),
        protocol: z.enum(['imap', 'pop3']),
        host: z.string().trim().min(1, t('settings.external.validation.hostRequired')),
        port: z.coerce.number<number>().int().min(1).max(65535),
        tls_mode: z.enum(['tls', 'starttls', 'none']),
        username: z.string().trim().min(1, t('settings.external.validation.usernameRequired')),
        password: z.string(),
        folder: z.string().trim(),
        interval_minutes: z.coerce.number<number>().int().min(2).max(1440),
        initial_days: z.coerce.number<number>().int().min(0).max(3650),
        target_group_id: z.coerce.number<number>().int().min(0),
        enabled: z.boolean(),
        delete_remote: z.boolean(),
        insecure_skip_verify: z.boolean(),
      }),
    [t],
  )

  type FormValues = z.infer<typeof schema>

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: '',
      protocol: 'imap',
      host: '',
      port: 993,
      tls_mode: 'tls',
      username: '',
      password: '',
      folder: 'INBOX',
      interval_minutes: 10,
      initial_days: 30,
      target_group_id: 0,
      enabled: true,
      delete_remote: false,
      insecure_skip_verify: false,
    },
  })

  const openAdd = () => {
    setEditing(null)
    setTestedFolders(null)
    form.reset({
      name: '',
      protocol: 'imap',
      host: '',
      port: 993,
      tls_mode: 'tls',
      username: '',
      password: '',
      folder: 'INBOX',
      interval_minutes: 10,
      initial_days: 30,
      target_group_id: 0,
      enabled: true,
      delete_remote: false,
      insecure_skip_verify: false,
    })
    setModalOpen(true)
  }

  const openEdit = (acc: ExternalAccount) => {
    setEditing(acc)
    setTestedFolders(null)
    form.reset({
      name: acc.name,
      protocol: acc.protocol,
      host: acc.host,
      port: acc.port,
      tls_mode: acc.tls_mode,
      username: acc.username,
      password: '',
      folder: acc.folder || 'INBOX',
      interval_minutes: acc.interval_minutes || 10,
      initial_days: acc.initial_days ?? 30,
      target_group_id: acc.target_group_id ?? 0,
      enabled: acc.enabled,
      delete_remote: acc.delete_remote,
      insecure_skip_verify: acc.insecure_skip_verify,
    })
    setModalOpen(true)
  }

  const applyPreset = (preset: ExternalPreset) => {
    form.setValue('protocol', preset.protocol)
    form.setValue('host', preset.host)
    form.setValue('port', preset.port)
    form.setValue('tls_mode', preset.tls_mode)
    if (!form.getValues('name')) {
      form.setValue('name', preset.name)
    }
  }

  const saveMutation = useMutation({
    mutationFn: async (vals: FormValues) => {
      if (!editing && !vals.password) {
        throw new Error(t('settings.external.validation.passwordRequired'))
      }
      const payload = {
        ...vals,
        name: vals.name || vals.username,
        folder: vals.folder || 'INBOX',
        ...(vals.password ? { password: vals.password } : {}),
      }
      return editing
        ? externalApi.update({ ...payload, id: editing.id })
        : externalApi.create(payload)
    },
    onSuccess: () => {
      toast.success(t('settings.external.saved'))
      setModalOpen(false)
      void queryClient.invalidateQueries({ queryKey: queryKeys.externalAccounts })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const testMutation = useMutation({
    mutationFn: async () => {
      const vals = form.getValues()
      if (!vals.host || !vals.username || !vals.password) {
        throw new Error(t('settings.external.validation.passwordRequired'))
      }
      return externalApi.test({
        ...vals,
        folder: vals.folder || 'INBOX',
      })
    },
    onSuccess: (res) => {
      const list = res.folders ?? []
      setTestedFolders(list)
      toast.success(t('settings.external.testSuccess', { count: list.length }))
    },
    onError: (err) => {
      setTestedFolders(null)
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const syncMutation = useMutation({
    mutationFn: (id: number) => externalApi.syncNow(id),
    onSuccess: () => {
      toast.success(t('settings.external.syncStarted'))
      void queryClient.invalidateQueries({ queryKey: queryKeys.externalAccounts })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (acc: ExternalAccount) => externalApi.remove(acc.id),
    onSuccess: (_res, acc) => {
      toast.success(t('settings.external.deleted', { name: acc.name || acc.username }))
      setDeleting(null)
      void queryClient.invalidateQueries({ queryKey: queryKeys.externalAccounts })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  return (
    <section className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] p-5 shadow-[var(--shadow-xs)] space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-[15px] font-semibold text-[var(--ink-primary)]">
            {t('settings.external.title')}
          </h2>
          <p className="mt-0.5 text-[12.5px] text-[var(--ink-secondary)]">
            {t('settings.external.desc')}
          </p>
        </div>
        <Button size="sm" onClick={openAdd}>
          <Plus className="h-3.5 w-3.5" />
          {t('settings.external.addButton')}
        </Button>
      </div>

      {isLoading ? (
        <div className="space-y-2.5">
          <Skeleton className="h-24 w-full rounded-[var(--radius-md)]" />
          <Skeleton className="h-24 w-full rounded-[var(--radius-md)]" />
        </div>
      ) : accounts.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-[var(--radius-md)] border border-dashed border-[var(--border-subtle)] py-12 text-center">
          <MailPlus className="h-6 w-6 text-[var(--ink-muted)]" />
          <p className="mt-2 text-[13.5px] font-medium text-[var(--ink-primary)]">
            {t('settings.external.empty')}
          </p>
          <p className="mt-1 max-w-md text-[12.5px] text-[var(--ink-muted)]">
            {t('settings.external.emptyDesc')}
          </p>
        </div>
      ) : (
        <div className="space-y-3">
          {accounts.map((acc) => (
            <div
              key={acc.id}
              className="rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-4 space-y-2.5"
            >
              <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div className="min-w-0 space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-[14px] font-semibold text-[var(--ink-primary)]">
                      {acc.name || acc.username}
                    </span>
                    <Badge variant={acc.enabled ? 'success' : 'default'}>
                      {acc.enabled ? t('common.enabled') : t('common.disabled')}
                    </Badge>
                    <Badge variant="default" className="uppercase font-mono text-[10.5px]">
                      {acc.protocol} · {acc.tls_mode}
                    </Badge>
                  </div>
                  <p className="break-all font-mono text-[12px] text-[var(--ink-secondary)]">
                    {acc.username} @ {acc.host}:{acc.port} ({acc.folder || 'INBOX'})
                  </p>
                  <div className="flex flex-wrap gap-x-4 gap-y-1 text-[11.5px] text-[var(--ink-muted)]">
                    <span>
                      {t('settings.external.lastSync')}：
                      {pickDate(acc.last_sync_at_iso, acc.last_sync_at)
                        ? formatMailFullTime(
                            pickDate(acc.last_sync_at_iso, acc.last_sync_at),
                            i18n.language,
                          )
                        : t('common.never')}
                    </span>
                    <span>
                      {t('settings.external.lastSuccess')}：
                      {pickDate(acc.last_success_at_iso, acc.last_success_at)
                        ? formatMailFullTime(
                            pickDate(acc.last_success_at_iso, acc.last_success_at),
                            i18n.language,
                          )
                        : t('common.never')}
                    </span>
                  </div>
                </div>

                <div className="flex shrink-0 items-center gap-1.5">
                  <Button
                    variant="outline"
                    size="xs"
                    disabled={syncMutation.isPending}
                    onClick={() => syncMutation.mutate(acc.id)}
                  >
                    <RefreshCw className="h-3 w-3" />
                    {t('settings.external.syncNow')}
                  </Button>
                  <Button
                    variant="ghost"
                    size="iconSm"
                    aria-label={t('common.edit')}
                    onClick={() => openEdit(acc)}
                  >
                    <Edit3 className="h-3.5 w-3.5" />
                  </Button>
                  <Button
                    variant="dangerGhost"
                    size="iconSm"
                    aria-label={t('common.delete')}
                    onClick={() => setDeleting(acc)}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>

              {acc.last_error && (
                <div className="flex items-start gap-2 rounded-[var(--radius-sm)] border border-[var(--danger)]/25 bg-[var(--danger-soft)] px-3 py-2 text-[12px] text-[var(--danger)]">
                  <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                  <div className="min-w-0 break-all">
                    <span className="font-medium">{t('settings.external.lastError')}：</span>
                    {acc.last_error}
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      {/* Add / Edit External Account Modal */}
      <Dialog open={modalOpen} onOpenChange={setModalOpen}>
        <DialogContent className="max-w-xl max-h-[90vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>
              {editing ? t('settings.external.modalEdit') : t('settings.external.modalAdd')}
            </DialogTitle>
          </DialogHeader>

          {presets.length > 0 && (
            <div className="mt-3 space-y-1.5">
              <p className="text-[12px] font-medium text-[var(--ink-muted)]">
                {t('settings.external.presetsLabel')}
              </p>
              <div className="flex flex-wrap gap-1.5">
                {presets.map((p) => (
                  <Button
                    key={`${p.name}-${p.protocol}`}
                    type="button"
                    variant="outline"
                    size="xs"
                    onClick={() => applyPreset(p)}
                  >
                    {p.name}
                  </Button>
                ))}
              </div>
            </div>
          )}

          <form
            onSubmit={form.handleSubmit((vals) => saveMutation.mutate(vals))}
            className="mt-4 space-y-3.5"
          >
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div className="space-y-1">
                <Label>{t('settings.external.fields.name')}</Label>
                <Input
                  {...form.register('name')}
                  placeholder={t('settings.external.fields.namePlaceholder')}
                />
              </div>

              <div className="space-y-1">
                <Label>{t('settings.external.fields.protocol')}</Label>
                <Controller
                  control={form.control}
                  name="protocol"
                  render={({ field }) => (
                    <Select
                      value={field.value}
                      onValueChange={(v) => {
                        const proto = v as ExternalProtocol
                        field.onChange(proto)
                        form.setValue('port', proto === 'imap' ? 993 : 995)
                      }}
                    >
                      <SelectTrigger className="h-9">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="imap">IMAP</SelectItem>
                        <SelectItem value="pop3">POP3</SelectItem>
                      </SelectContent>
                    </Select>
                  )}
                />
              </div>
            </div>

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_92px_145px]">
              <div className="space-y-1">
                <Label>{t('settings.external.fields.host')}</Label>
                <Input
                  {...form.register('host')}
                  placeholder={t('settings.external.fields.hostPlaceholder')}
                />
                {form.formState.errors.host && (
                  <p className="text-[11.5px] text-[var(--danger)]">
                    {form.formState.errors.host.message}
                  </p>
                )}
              </div>

              <div className="space-y-1">
                <Label>{t('settings.external.fields.port')}</Label>
                <Input type="number" {...form.register('port')} />
              </div>

              <div className="space-y-1">
                <Label>{t('settings.external.fields.tlsMode')}</Label>
                <Controller
                  control={form.control}
                  name="tls_mode"
                  render={({ field }) => (
                    <Select
                      value={field.value}
                      onValueChange={(v) => field.onChange(v as ExternalTlsMode)}
                    >
                      <SelectTrigger className="h-9">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="tls">{t('settings.external.tlsModes.tls')}</SelectItem>
                        <SelectItem value="starttls">
                          {t('settings.external.tlsModes.starttls')}
                        </SelectItem>
                        <SelectItem value="none">{t('settings.external.tlsModes.none')}</SelectItem>
                      </SelectContent>
                    </Select>
                  )}
                />
              </div>
            </div>

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div className="space-y-1">
                <Label>{t('settings.external.fields.username')}</Label>
                <Input {...form.register('username')} autoComplete="off" />
                {form.formState.errors.username && (
                  <p className="text-[11.5px] text-[var(--danger)]">
                    {form.formState.errors.username.message}
                  </p>
                )}
              </div>

              <div className="space-y-1">
                <Label>{t('settings.external.fields.password')}</Label>
                <Input
                  type="password"
                  {...form.register('password')}
                  placeholder={editing ? t('settings.external.fields.passwordKeepHint') : ''}
                  autoComplete="new-password"
                />
              </div>
            </div>

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
              <div className="space-y-1">
                <Label>{t('settings.external.fields.folder')}</Label>
                <Input {...form.register('folder')} />
              </div>
              <div className="space-y-1">
                <Label>{t('settings.external.fields.interval')}</Label>
                <Input type="number" min={2} {...form.register('interval_minutes')} />
              </div>
              <div className="space-y-1">
                <Label>{t('settings.external.fields.initialDays')}</Label>
                <Input type="number" min={0} {...form.register('initial_days')} />
              </div>
            </div>

            <div className="space-y-1">
              <Label>{t('settings.external.fields.targetGroup')}</Label>
              <Controller
                control={form.control}
                name="target_group_id"
                render={({ field }) => (
                  <Select
                    value={String(field.value)}
                    onValueChange={(v) => field.onChange(Number(v))}
                  >
                    <SelectTrigger className="h-9">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="0">{t('settings.external.fields.inboxDefault')}</SelectItem>
                      {customGroups.map((g) => (
                        <SelectItem key={g.id} value={String(g.id)}>
                          {g.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            </div>

            <div className="space-y-2 rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/50 p-3">
              <div className="flex items-center justify-between text-[13px]">
                <span>{t('settings.external.fields.enabled')}</span>
                <Controller
                  control={form.control}
                  name="enabled"
                  render={({ field }) => (
                    <Switch checked={field.value} onCheckedChange={field.onChange} />
                  )}
                />
              </div>
              <div className="flex items-center justify-between text-[13px]">
                <span>{t('settings.external.fields.deleteRemote')}</span>
                <Controller
                  control={form.control}
                  name="delete_remote"
                  render={({ field }) => (
                    <Switch checked={field.value} onCheckedChange={field.onChange} />
                  )}
                />
              </div>
              <div className="flex items-center justify-between text-[13px]">
                <span>{t('settings.external.fields.insecureSkipVerify')}</span>
                <Controller
                  control={form.control}
                  name="insecure_skip_verify"
                  render={({ field }) => (
                    <Switch checked={field.value} onCheckedChange={field.onChange} />
                  )}
                />
              </div>
            </div>

            {testedFolders && (
              <div className="flex items-center gap-2 rounded-[var(--radius-sm)] border border-[var(--success)]/30 bg-[var(--success-soft)] px-3 py-2 text-[12px] text-[var(--success)]">
                <CheckCircle2 className="h-4 w-4 shrink-0" />
                <span className="truncate">
                  {t('settings.external.testSuccess', { count: testedFolders.length })}
                  {testedFolders.length > 0 ? `：${testedFolders.slice(0, 5).join(', ')}` : ''}
                </span>
              </div>
            )}

            <DialogFooter className="flex-wrap justify-between gap-2">
              <Button
                type="button"
                variant="secondary"
                size="sm"
                disabled={testMutation.isPending}
                onClick={() => testMutation.mutate()}
              >
                {testMutation.isPending ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <PlugZap className="h-3.5 w-3.5" />
                )}
                {testMutation.isPending
                  ? t('settings.external.testing')
                  : t('settings.external.testConn')}
              </Button>

              <div className="flex items-center gap-2">
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
              </div>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Delete Confirm */}
      <Dialog open={deleting !== null} onOpenChange={(open) => !open && setDeleting(null)}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{t('settings.external.deleteTitle')}</DialogTitle>
            <DialogDescription>
              {deleting
                ? t('settings.external.deleteConfirm', { name: deleting.name || deleting.username })
                : ''}
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
              onClick={() => deleting && deleteMutation.mutate(deleting)}
            >
              {t('common.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  )
}
