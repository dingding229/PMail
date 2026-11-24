import { useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  AlertTriangle,
  AtSign,
  Ban,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Edit3,
  KeyRound,
  Loader2,
  Plus,
  ShieldCheck,
  ShieldOff,
  Users,
} from 'lucide-react'
import { userApi, type UserProfile } from '@/api'
import { formatBytes } from '@/lib/format'
import { formatMailFullTime, pickDate } from '@/lib/date'
import { queryKeys } from '@/hooks/useMailQueries'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
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
  SectionHeader,
  SectionLoadingState,
} from '@/components/settings'
import { SenderAvatar } from '@/components/common/SenderAvatar'
import { AddressesSection } from '@/features/settings/sections/AddressesSection'

const PAGE_SIZE = 10

export function UsersSection({ currentUserID }: { currentUserID?: number }) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()

  const [page, setPage] = useState(1)
  const [createOpen, setCreateOpen] = useState(false)
  const [editingUser, setEditingUser] = useState<UserProfile | null>(null)
  const [resetFactorUser, setResetFactorUser] = useState<UserProfile | null>(null)
  const [resetPwdUser, setResetPwdUser] = useState<UserProfile | null>(null)
  const [newPassword, setNewPassword] = useState('')
  const [disablingUser, setDisablingUser] = useState<UserProfile | null>(null)
  const [managingAddrUser, setManagingAddrUser] = useState<UserProfile | null>(null)

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: queryKeys.users(page, PAGE_SIZE),
    queryFn: () => userApi.list({ current_page: page, page_size: PAGE_SIZE }),
  })

  const users = data?.list ?? []
  const totalPages = Math.max(1, data?.total_page ?? 1)

  const createSchema = useMemo(
    () =>
      z.object({
        account: z
          .string()
          .trim()
          .min(1, t('admin.users.validation.accountRequired'))
          .regex(/^[a-zA-Z0-9][a-zA-Z0-9._-]{1,19}$/, t('admin.users.validation.accountFormat')),
        display_name: z
          .string()
          .trim()
          .min(1, t('admin.users.validation.displayNameRequired'))
          .max(50),
        password: z.string().min(8, t('admin.users.validation.passwordMin')),
        quota_mb: z.coerce.number<number>().int().min(0),
      }),
    [t],
  )

  const editSchema = useMemo(
    () =>
      z.object({
        display_name: z
          .string()
          .trim()
          .min(1, t('admin.users.validation.displayNameRequired'))
          .max(50),
        quota_mb: z.coerce.number<number>().int().min(0),
      }),
    [t],
  )

  const createForm = useForm<z.infer<typeof createSchema>>({
    resolver: zodResolver(createSchema),
    defaultValues: { account: '', display_name: '', password: '', quota_mb: 1024 },
  })

  const editForm = useForm<z.infer<typeof editSchema>>({
    resolver: zodResolver(editSchema),
    defaultValues: { display_name: '', quota_mb: 0 },
  })

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: queryKeys.usersRoot })
  }

  const createMutation = useMutation({
    mutationFn: (vals: z.infer<typeof createSchema>) =>
      userApi.create({
        account: vals.account.toLowerCase(),
        display_name: vals.display_name,
        password: vals.password,
        quota_mb: vals.quota_mb,
      }),
    onSuccess: (created) => {
      toast.success(t('admin.users.created', { account: created.account }))
      setCreateOpen(false)
      createForm.reset({ account: '', display_name: '', password: '', quota_mb: 1024 })
      invalidate()
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const editMutation = useMutation({
    mutationFn: (vals: z.infer<typeof editSchema>) => {
      if (!editingUser) throw new Error('No user selected')
      return userApi.edit({
        id: editingUser.id,
        display_name: vals.display_name,
        quota_mb: vals.quota_mb,
      })
    },
    onSuccess: () => {
      toast.success(t('admin.users.updated'))
      setEditingUser(null)
      invalidate()
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const resetFactorMutation = useMutation({
    mutationFn: (id: number) => userApi.resetSecondFactor(id),
    onSuccess: () => {
      toast.success(t('security.resetSuccess'))
      setResetFactorUser(null)
      invalidate()
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : t('common.unknownError')),
  })

  const renderFactorStatus = (u: UserProfile) => (
    <Badge
      variant={u.totp_enabled ? 'success' : 'default'}
      title={u.totp_enabled ? t('security.enabled') : t('security.disabled')}
    >
      {u.totp_enabled ? (
        <ShieldCheck className="h-3 w-3" />
      ) : (
        <ShieldOff className="h-3 w-3 opacity-70" />
      )}
      {u.totp_enabled ? t('security.enabled') : t('security.disabled')}
    </Badge>
  )
  const canResetFactor = (u: UserProfile) => !!u.totp_enabled && currentUserID != null && u.id !== currentUserID

  const resetPwdMutation = useMutation({
    mutationFn: () => {
      if (!resetPwdUser) throw new Error('No user')
      if (newPassword.length < 8) {
        throw new Error(t('admin.users.validation.passwordMin'))
      }
      return userApi.resetPassword({ id: resetPwdUser.id, password: newPassword })
    },
    onSuccess: () => {
      if (resetPwdUser) {
        toast.success(t('admin.users.passwordReset', { account: resetPwdUser.account }))
      }
      setResetPwdUser(null)
      setNewPassword('')
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const toggleDisableMutation = useMutation({
    mutationFn: ({ user, disabled }: { user: UserProfile; disabled: number }) =>
      disabled === 1 ? userApi.remove(user.id) : userApi.edit({ id: user.id, disabled: 0 }),
    onSuccess: (_res, { user, disabled }) => {
      toast.success(
        disabled === 1
          ? t('admin.users.disabledToast', { account: user.account })
          : t('admin.users.enabledToast', { account: user.account }),
      )
      setDisablingUser(null)
      invalidate()
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const openEdit = (u: UserProfile) => {
    setEditingUser(u)
    editForm.reset({
      display_name: u.display_name || u.name,
      quota_mb: u.quota_mb ?? 0,
    })
  }

  const renderQuotaBar = (u: UserProfile) => {
    const quotaBytes = (u.quota_mb ?? 0) * 1024 * 1024
    const pct = quotaBytes > 0 ? Math.min(100, Math.round((u.used_bytes / quotaBytes) * 100)) : 0
    return (
      <div className="min-w-[130px] space-y-1">
        <div className="flex items-center justify-between text-[11.5px] tabular-nums text-[var(--ink-secondary)]">
          <span>{formatBytes(u.used_bytes)}</span>
          <span className="text-[var(--ink-muted)]">
            {quotaBytes > 0 ? `${u.quota_mb} MB` : t('common.unlimited')}
          </span>
        </div>
        {quotaBytes > 0 && (
          <div className="h-1.5 w-full overflow-hidden rounded-full bg-[var(--surface-subtle)]">
            <div
              style={{ width: `${pct}%` }}
              className={`h-full rounded-full ${
                pct >= 90
                  ? 'bg-[var(--danger)]'
                  : pct >= 75
                    ? 'bg-[var(--warning)]'
                    : 'bg-[var(--accent)]'
              }`}
            />
          </div>
        )}
      </div>
    )
  }

  return (
    <SectionCard>
      <SectionHeader
        icon={Users}
        title={t('admin.users.title')}
        description={t('admin.users.desc')}
        action={
          <Button size="sm" onClick={() => setCreateOpen(true)}>
            <Plus className="h-3.5 w-3.5" />
            {t('admin.users.addButton')}
          </Button>
        }
      />

      {error ? (
        <SectionErrorState
          message={error instanceof Error ? error.message : t('common.unknownError')}
          retryLabel={t('app.retry')}
          onRetry={() => void refetch()}
        />
      ) : isLoading ? (
        <SectionLoadingState rows={3} rowHeight="h-16" />
      ) : users.length === 0 ? (
        <EmptyState
          icon={Users}
          title={t('admin.users.empty')}
          action={
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <Plus className="h-3.5 w-3.5" />
              {t('admin.users.addButton')}
            </Button>
          }
        />
      ) : (
        <>
          {/* Desktop Table */}
          <DataTable className="hidden md:block">
            <DataTableHead>
              <tr>
                <DataTableHeaderCell className="px-3">{t('admin.users.account')}</DataTableHeaderCell>
                <DataTableHeaderCell className="px-3 whitespace-nowrap">{t('common.status')}</DataTableHeaderCell>
                <DataTableHeaderCell className="px-3 whitespace-nowrap">{t('security.status2fa')}</DataTableHeaderCell>
                <DataTableHeaderCell className="px-3 whitespace-nowrap">{t('admin.users.quota')}</DataTableHeaderCell>
                <DataTableHeaderCell className="px-3 whitespace-nowrap">{t('admin.users.lastLogin')}</DataTableHeaderCell>
                <DataTableHeaderCell align="right" className="px-3 whitespace-nowrap">{t('common.actions')}</DataTableHeaderCell>
              </tr>
            </DataTableHead>
            <DataTableBody>
              {users.map((u) => (
                <DataTableRow key={u.id}>
                  <DataTableCell className="px-3">
                    <div className="flex items-center gap-2.5">
                      <SenderAvatar
                        name={u.display_name || u.name}
                        email={u.account}
                        size="sm"
                      />
                      <div className="min-w-0">
                        <div className="flex items-center gap-1.5">
                          <span className="whitespace-nowrap font-medium text-[var(--ink-primary)]">
                            {u.display_name || u.name}
                          </span>
                          {u.is_admin === 1 && (
                            <Badge variant="accent">{t('admin.users.roleAdmin')}</Badge>
                          )}
                        </div>
                        <p className="whitespace-nowrap font-mono text-[11.5px] text-[var(--ink-muted)]">
                          {u.account} ·{' '}
                          {t('admin.users.addressesCount', {
                            count: u.addresses?.length ?? 0,
                          })}
                        </p>
                      </div>
                    </div>
                  </DataTableCell>
                  <DataTableCell className="px-3 whitespace-nowrap">
                    <Badge variant={u.disabled === 1 ? 'danger' : 'success'}>
                      {u.disabled === 1
                        ? t('admin.users.statusDisabled')
                        : t('admin.users.statusActive')}
                    </Badge>
                  </DataTableCell>
                  <DataTableCell className="px-3 whitespace-nowrap">{renderFactorStatus(u)}</DataTableCell>
                  <DataTableCell className="px-3">{renderQuotaBar(u)}</DataTableCell>
                  <DataTableCell className="px-3 whitespace-nowrap text-[12px] tabular-nums text-[var(--ink-muted)]">
                    {pickDate(u.last_login_at_iso, u.last_login_at)
                      ? formatMailFullTime(
                          pickDate(u.last_login_at_iso, u.last_login_at),
                          i18n.language,
                        )
                      : t('common.never')}
                  </DataTableCell>
                  <DataTableCell align="right" className="px-3 whitespace-nowrap">
                    <div className="flex items-center justify-end gap-0.5">
                      <Button
                        variant="ghost"
                        size="iconSm"
                        title={t('admin.users.manageAddresses')}
                        aria-label={t('admin.users.manageAddresses')}
                        onClick={() => setManagingAddrUser(u)}
                      >
                        <AtSign className="h-3.5 w-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="iconSm"
                        title={t('common.edit')}
                        aria-label={t('common.edit')}
                        onClick={() => openEdit(u)}
                      >
                        <Edit3 className="h-3.5 w-3.5" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="iconSm"
                        title={t('admin.users.resetPassword')}
                        aria-label={t('admin.users.resetPassword')}
                        onClick={() => {
                          setResetPwdUser(u)
                          setNewPassword('')
                        }}
                      >
                        <KeyRound className="h-3.5 w-3.5" />
                      </Button>
                      {canResetFactor(u) && (
                        <Button
                          variant="dangerGhost"
                          size="iconSm"
                          title={t('security.reset')}
                          aria-label={t('security.reset')}
                          onClick={() => setResetFactorUser(u)}
                        >
                          <ShieldOff className="h-3.5 w-3.5" />
                        </Button>
                      )}
                      {u.disabled === 1 ? (
                        <Button
                          variant="ghost"
                          size="iconSm"
                          title={t('admin.users.enable')}
                          aria-label={t('admin.users.enable')}
                          onClick={() =>
                            toggleDisableMutation.mutate({ user: u, disabled: 0 })
                          }
                        >
                          <CheckCircle2 className="h-3.5 w-3.5 text-[var(--success)]" />
                        </Button>
                      ) : (
                        <Button
                          variant="dangerGhost"
                          size="iconSm"
                          title={t('admin.users.disable')}
                          aria-label={t('admin.users.disable')}
                          onClick={() => setDisablingUser(u)}
                        >
                          <Ban className="h-3.5 w-3.5" />
                        </Button>
                      )}
                    </div>
                  </DataTableCell>
                </DataTableRow>
              ))}
            </DataTableBody>
          </DataTable>

          {/* Mobile Card Layout */}
          <div className="space-y-3 md:hidden">
            {users.map((u) => (
              <div
                key={u.id}
                className="rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-3.5 space-y-3"
              >
                <div className="flex items-start justify-between gap-2">
                  <div className="flex items-center gap-2.5 min-w-0">
                    <SenderAvatar name={u.display_name || u.name} email={u.account} size="sm" />
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-1.5">
                        <span className="font-medium text-[var(--ink-primary)] truncate">
                          {u.display_name || u.name}
                        </span>
                        {u.is_admin === 1 && (
                          <Badge variant="accent">{t('admin.users.roleAdmin')}</Badge>
                        )}
                        {renderFactorStatus(u)}
                      </div>
                      <p className="font-mono text-[11.5px] text-[var(--ink-muted)] truncate">
                        {u.account} · {t('admin.users.addressesCount', { count: u.addresses?.length ?? 0 })}
                      </p>
                    </div>
                  </div>
                  <Badge variant={u.disabled === 1 ? 'danger' : 'success'}>
                    {u.disabled === 1
                      ? t('admin.users.statusDisabled')
                      : t('admin.users.statusActive')}
                  </Badge>
                </div>

                {renderQuotaBar(u)}

                <div className="flex flex-wrap items-center gap-1.5 border-t border-[var(--border-hairline)] pt-2.5">
                  <Button variant="outline" size="xs" onClick={() => setManagingAddrUser(u)}>
                    <AtSign className="h-3 w-3" />
                    {t('admin.users.manageAddresses')}
                  </Button>
                  <Button variant="outline" size="xs" onClick={() => openEdit(u)}>
                    {t('common.edit')}
                  </Button>
                  <Button
                    variant="outline"
                    size="xs"
                    onClick={() => {
                      setResetPwdUser(u)
                      setNewPassword('')
                    }}
                  >
                    {t('admin.users.resetPassword')}
                  </Button>
                  {canResetFactor(u) && (
                    <Button variant="outline" size="xs" onClick={() => setResetFactorUser(u)}>{t('security.reset')}</Button>
                  )}
                  {u.disabled === 1 ? (
                    <Button
                      variant="outline"
                      size="xs"
                      onClick={() => toggleDisableMutation.mutate({ user: u, disabled: 0 })}
                    >
                      {t('admin.users.enable')}
                    </Button>
                  ) : (
                    <Button
                      variant="dangerGhost"
                      size="xs"
                      onClick={() => setDisablingUser(u)}
                    >
                      {t('admin.users.disable')}
                    </Button>
                  )}
                </div>
              </div>
            ))}
          </div>

          {/* Pagination */}
          {totalPages > 1 && (
            <div className="flex items-center justify-between pt-1">
              <span className="text-[12px] text-[var(--ink-muted)]">
                {t('common.pageInfo', { page, total: totalPages })}
              </span>
              <div className="flex items-center gap-1.5">
                <Button
                  variant="outline"
                  size="xs"
                  disabled={page <= 1}
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                >
                  <ChevronLeft className="h-3.5 w-3.5" />
                  {t('common.prevPage')}
                </Button>
                <Button
                  variant="outline"
                  size="xs"
                  disabled={page >= totalPages}
                  onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                >
                  {t('common.nextPage')}
                  <ChevronRight className="h-3.5 w-3.5" />
                </Button>
              </div>
            </div>
          )}
        </>
      )}

      {/* Create User Modal */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>{t('admin.users.modalAdd')}</DialogTitle>
          </DialogHeader>
          <form
            onSubmit={createForm.handleSubmit((vals) => createMutation.mutate(vals))}
            className="mt-3 space-y-3.5"
          >
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div className="space-y-1">
                <Label>{t('admin.users.account')}</Label>
                <Input {...createForm.register('account')} autoFocus />
                {createForm.formState.errors.account && (
                  <p className="text-[11.5px] text-[var(--danger)]">
                    {createForm.formState.errors.account.message}
                  </p>
                )}
              </div>
              <div className="space-y-1">
                <Label>{t('admin.users.displayName')}</Label>
                <Input {...createForm.register('display_name')} />
                {createForm.formState.errors.display_name && (
                  <p className="text-[11.5px] text-[var(--danger)]">
                    {createForm.formState.errors.display_name.message}
                  </p>
                )}
              </div>
            </div>

            <div className="space-y-1">
              <Label>{t('admin.users.passwordLabel')}</Label>
              <Input type="password" {...createForm.register('password')} />
              {createForm.formState.errors.password && (
                <p className="text-[11.5px] text-[var(--danger)]">
                  {createForm.formState.errors.password.message}
                </p>
              )}
            </div>

            <div className="space-y-1">
              <Label>{t('admin.users.quotaLabel')}</Label>
              <Input type="number" min={0} {...createForm.register('quota_mb')} />
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" size="sm" onClick={() => setCreateOpen(false)}>
                {t('common.cancel')}
              </Button>
              <Button type="submit" size="sm" disabled={createMutation.isPending}>
                {createMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                {t('common.create')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Edit User Modal */}
      <Dialog open={editingUser !== null} onOpenChange={(open) => !open && setEditingUser(null)}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{t('admin.users.modalEdit')}</DialogTitle>
            <DialogDescription>{editingUser?.account}</DialogDescription>
          </DialogHeader>
          <form
            onSubmit={editForm.handleSubmit((vals) => editMutation.mutate(vals))}
            className="mt-3 space-y-3.5"
          >
            <div className="space-y-1">
              <Label>{t('admin.users.displayName')}</Label>
              <Input {...editForm.register('display_name')} autoFocus />
              {editForm.formState.errors.display_name && (
                <p className="text-[11.5px] text-[var(--danger)]">
                  {editForm.formState.errors.display_name.message}
                </p>
              )}
            </div>
            <div className="space-y-1">
              <Label>{t('admin.users.quotaLabel')}</Label>
              <Input type="number" min={0} {...editForm.register('quota_mb')} />
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setEditingUser(null)}
              >
                {t('common.cancel')}
              </Button>
              <Button type="submit" size="sm" disabled={editMutation.isPending}>
                {editMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                {t('common.save')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Reset Two-Step Verification Confirm Dialog */}
      <Dialog
        open={resetFactorUser !== null}
        onOpenChange={(open) => {
          if (!open && !resetFactorMutation.isPending) setResetFactorUser(null)
        }}
      >
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <span className="flex h-7 w-7 items-center justify-center rounded-[var(--radius-sm)] bg-[var(--danger-soft)] text-[var(--danger)]">
                <ShieldOff className="h-4 w-4" />
              </span>
              {t('security.reset')}
            </DialogTitle>
            <DialogDescription>
              {t('security.resetConfirm', { account: resetFactorUser?.account })}
            </DialogDescription>
          </DialogHeader>
          <div className="mt-2 flex items-start gap-2 rounded-[var(--radius-sm)] border border-[var(--warning)]/30 bg-[var(--warning-soft)] px-3 py-2.5 text-[12px] text-[var(--warning)]">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
            <span>{t('security.resetWarningNote')}</span>
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={resetFactorMutation.isPending}
              onClick={() => setResetFactorUser(null)}
            >
              {t('common.cancel')}
            </Button>
            <Button
              type="button"
              variant="danger"
              size="sm"
              disabled={!resetFactorUser || resetFactorMutation.isPending}
              onClick={() => {
                if (resetFactorUser) resetFactorMutation.mutate(resetFactorUser.id)
              }}
            >
              {resetFactorMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
              {t('common.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Reset Password Modal */}
      <Dialog
        open={resetPwdUser !== null}
        onOpenChange={(open) => !open && setResetPwdUser(null)}
      >
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{t('admin.users.resetPassword')}</DialogTitle>
            <DialogDescription>{resetPwdUser?.account}</DialogDescription>
          </DialogHeader>
          <form
            onSubmit={(e) => {
              e.preventDefault()
              resetPwdMutation.mutate()
            }}
            className="mt-3 space-y-3.5"
          >
            <div className="space-y-1">
              <Label>{t('admin.users.newPasswordLabel')}</Label>
              <Input
                type="password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                autoFocus
              />
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setResetPwdUser(null)}
              >
                {t('common.cancel')}
              </Button>
              <Button type="submit" size="sm" disabled={resetPwdMutation.isPending}>
                {resetPwdMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                {t('common.confirm')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Disable Confirm */}
      <Dialog
        open={disablingUser !== null}
        onOpenChange={(open) => !open && setDisablingUser(null)}
      >
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{t('admin.users.disableTitle')}</DialogTitle>
            <DialogDescription>
              {disablingUser
                ? t('admin.users.disableConfirm', { account: disablingUser.account })
                : ''}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" size="sm" onClick={() => setDisablingUser(null)}>
              {t('common.cancel')}
            </Button>
            <Button
              variant="danger"
              size="sm"
              disabled={toggleDisableMutation.isPending}
              onClick={() =>
                disablingUser &&
                toggleDisableMutation.mutate({ user: disablingUser, disabled: 1 })
              }
            >
              {t('admin.users.disable')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Assign / Manage User Addresses Modal */}
      <Dialog
        open={managingAddrUser !== null}
        onOpenChange={(open) => !open && setManagingAddrUser(null)}
      >
        <DialogContent className="max-w-2xl max-h-[85vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>
              {managingAddrUser
                ? t('admin.users.addressesModalTitle', {
                    name: managingAddrUser.display_name || managingAddrUser.name,
                    account: managingAddrUser.account,
                  })
                : ''}
            </DialogTitle>
          </DialogHeader>
          {managingAddrUser && (
            <div className="mt-3">
              <AddressesSection userId={managingAddrUser.id} embedded />
            </div>
          )}
        </DialogContent>
      </Dialog>
    </SectionCard>
  )
}
