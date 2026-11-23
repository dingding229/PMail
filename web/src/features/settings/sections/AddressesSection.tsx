import { useMemo, useState } from 'react'
import { useForm, Controller } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { AtSign, Loader2, Plus, Star, Trash2 } from 'lucide-react'
import { domainApi, userApi, type MailboxAddress } from '@/api'
import { queryKeys } from '@/hooks/useMailQueries'
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

interface AddressesSectionProps {
  /** Omit for current user; pass userId when used inside Admin Console */
  userId?: number
  embedded?: boolean
}

export function AddressesSection({ userId, embedded = false }: AddressesSectionProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const [addOpen, setAddOpen] = useState(false)
  const [deletingAddr, setDeletingAddr] = useState<MailboxAddress | null>(null)

  const addressQueryKey = userId ? queryKeys.userAddresses(userId) : queryKeys.addresses

  const { data: addresses = [], isLoading } = useQuery({
    queryKey: addressQueryKey,
    queryFn: () => userApi.listAddresses(userId),
  })

  // Fetch domains for alias creation dropdown; gracefully derive from existing addresses if non-admin
  const { data: adminDomains } = useQuery({
    queryKey: queryKeys.domains,
    queryFn: () => domainApi.list(),
    retry: false,
  })

  const domainOptions = useMemo(() => {
    if (adminDomains && adminDomains.length > 0) {
      return adminDomains.map((d) => ({ id: d.id, name: d.name }))
    }
    const map = new Map<number, string>()
    for (const a of addresses) {
      map.set(a.domain_id, a.domain)
    }
    return Array.from(map.entries()).map(([id, name]) => ({ id, name }))
  }, [adminDomains, addresses])

  const schema = useMemo(
    () =>
      z.object({
        local_part: z
          .string()
          .trim()
          .min(1, t('settings.addresses.validation.localRequired'))
          .regex(/^[a-zA-Z0-9._+-]+$/, t('settings.addresses.validation.localFormat')),
        domain_id: z.number().min(1, t('settings.addresses.validation.domainRequired')),
        can_send: z.boolean(),
      }),
    [t],
  )

  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: {
      local_part: '',
      domain_id: domainOptions[0]?.id ?? 0,
      can_send: true,
    },
  })

  const invalidateAll = () => {
    void queryClient.invalidateQueries({ queryKey: addressQueryKey })
    void queryClient.invalidateQueries({ queryKey: queryKeys.addresses })
    void queryClient.invalidateQueries({ queryKey: queryKeys.sidebarGroups })
    void queryClient.invalidateQueries({ queryKey: queryKeys.emailListsRoot })
    void queryClient.invalidateQueries({ queryKey: ['emails', 'detail'] })
    void queryClient.invalidateQueries({ queryKey: queryKeys.userMe })
    void queryClient.invalidateQueries({ queryKey: queryKeys.usersRoot })
  }

  const createMutation = useMutation({
    mutationFn: (values: z.infer<typeof schema>) =>
      userApi.createAddress({
        user_id: userId,
        local_part: values.local_part.toLowerCase(),
        domain_id: values.domain_id,
        can_send: values.can_send,
      }),
    onSuccess: (created) => {
      toast.success(t('settings.addresses.created', { address: created.address }))
      setAddOpen(false)
      form.reset({
        local_part: '',
        domain_id: domainOptions[0]?.id ?? 0,
        can_send: true,
      })
      invalidateAll()
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const toggleSendMutation = useMutation({
    mutationFn: ({ id, can_send }: { id: number; can_send: boolean }) =>
      userApi.updateAddress({ id, can_send }),
    onSuccess: () => {
      toast.success(t('settings.addresses.updated'))
      invalidateAll()
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const setPrimaryMutation = useMutation({
    mutationFn: (id: number) => userApi.setPrimaryAddress(id),
    onSuccess: () => {
      toast.success(t('settings.addresses.primaryUpdated'))
      invalidateAll()
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (addr: MailboxAddress) => userApi.deleteAddress(addr.id),
    onSuccess: (_res, addr) => {
      toast.success(t('settings.addresses.deleted', { address: addr.address }))
      setDeletingAddr(null)
      invalidateAll()
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const openAddModal = () => {
    form.reset({
      local_part: '',
      domain_id: domainOptions[0]?.id ?? 0,
      can_send: true,
    })
    setAddOpen(true)
  }

  return (
    <section
      className={
        embedded
          ? 'space-y-4'
          : 'rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] p-5 shadow-[var(--shadow-xs)] space-y-4'
      }
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        {!embedded && (
          <div>
            <h2 className="text-[15px] font-semibold text-[var(--ink-primary)]">
              {t('settings.addresses.title')}
            </h2>
            <p className="mt-0.5 text-[12.5px] text-[var(--ink-secondary)]">
              {t('settings.addresses.desc')}
            </p>
          </div>
        )}
        <Button size="sm" onClick={openAddModal} disabled={domainOptions.length === 0}>
          <Plus className="h-3.5 w-3.5" />
          {t('settings.addresses.addButton')}
        </Button>
      </div>

      {isLoading ? (
        <div className="space-y-2.5">
          <Skeleton className="h-16 w-full rounded-[var(--radius-md)]" />
          <Skeleton className="h-16 w-full rounded-[var(--radius-md)]" />
        </div>
      ) : addresses.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-[var(--radius-md)] border border-dashed border-[var(--border-subtle)] py-10 text-center">
          <AtSign className="h-6 w-6 text-[var(--ink-muted)]" />
          <p className="mt-2 text-[13.5px] text-[var(--ink-secondary)]">
            {t('settings.addresses.empty')}
          </p>
        </div>
      ) : (
        <div className="divide-y divide-[var(--border-hairline)] rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)]">
          {addresses.map((addr) => (
            <div
              key={addr.id}
              className="flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
            >
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="break-all font-mono text-[13.5px] font-medium text-[var(--ink-primary)]">
                    {addr.address}
                  </span>
                  {addr.is_primary && (
                    <Badge variant="accent">{t('settings.addresses.primaryBadge')}</Badge>
                  )}
                  {!addr.can_send && (
                    <Badge variant="default">{t('settings.addresses.receiveOnly')}</Badge>
                  )}
                </div>
              </div>

              <div className="flex flex-wrap items-center gap-3">
                <label className="inline-flex items-center gap-2 text-[12.5px] text-[var(--ink-secondary)]">
                  <span>{t('settings.addresses.canSendLabel')}</span>
                  <Switch
                    checked={addr.can_send}
                    disabled={addr.is_primary || toggleSendMutation.isPending}
                    onCheckedChange={(checked) =>
                      toggleSendMutation.mutate({ id: addr.id, can_send: checked })
                    }
                  />
                </label>

                {!addr.is_primary && (
                  <Button
                    variant="outline"
                    size="xs"
                    disabled={setPrimaryMutation.isPending}
                    onClick={() => setPrimaryMutation.mutate(addr.id)}
                  >
                    <Star className="h-3 w-3" />
                    {t('settings.addresses.setPrimary')}
                  </Button>
                )}

                {!addr.is_primary && (
                  <Button
                    variant="dangerGhost"
                    size="iconSm"
                    aria-label={t('common.delete')}
                    onClick={() => setDeletingAddr(addr)}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Add Alias Modal */}
      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>{t('settings.addresses.modalTitle')}</DialogTitle>
          </DialogHeader>
          <form
            onSubmit={form.handleSubmit((vals) => createMutation.mutate(vals))}
            className="mt-3 space-y-4"
          >
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="alias_local">{t('settings.addresses.localPart')}</Label>
                <Input
                  id="alias_local"
                  {...form.register('local_part')}
                  placeholder={t('settings.addresses.localPartPlaceholder')}
                  autoFocus
                />
                {form.formState.errors.local_part && (
                  <p className="text-[12px] text-[var(--danger)]">
                    {form.formState.errors.local_part.message}
                  </p>
                )}
              </div>

              <div className="space-y-1.5">
                <Label htmlFor="alias_domain">{t('settings.addresses.domain')}</Label>
                <Controller
                  control={form.control}
                  name="domain_id"
                  render={({ field }) => (
                    <Select
                      value={field.value ? String(field.value) : ''}
                      onValueChange={(v) => field.onChange(Number(v))}
                    >
                      <SelectTrigger id="alias_domain" className="h-9">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {domainOptions.map((d) => (
                          <SelectItem key={d.id} value={String(d.id)}>
                            @{d.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  )}
                />
                {form.formState.errors.domain_id && (
                  <p className="text-[12px] text-[var(--danger)]">
                    {form.formState.errors.domain_id.message}
                  </p>
                )}
              </div>
            </div>

            <div className="flex items-center justify-between rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/60 px-3 py-2.5">
              <span className="text-[13px] text-[var(--ink-primary)]">
                {t('settings.addresses.canSendLabel')}
              </span>
              <Controller
                control={form.control}
                name="can_send"
                render={({ field }) => (
                  <Switch checked={field.value} onCheckedChange={field.onChange} />
                )}
              />
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" size="sm" onClick={() => setAddOpen(false)}>
                {t('common.cancel')}
              </Button>
              <Button type="submit" size="sm" disabled={createMutation.isPending}>
                {createMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                {t('common.add')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Delete Confirm Dialog */}
      <Dialog
        open={deletingAddr !== null}
        onOpenChange={(open) => !open && setDeletingAddr(null)}
      >
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{t('settings.addresses.deleteTitle')}</DialogTitle>
            <DialogDescription>
              {deletingAddr
                ? t('settings.addresses.deleteConfirm', { address: deletingAddr.address })
                : ''}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" size="sm" onClick={() => setDeletingAddr(null)}>
              {t('common.cancel')}
            </Button>
            <Button
              variant="danger"
              size="sm"
              disabled={deleteMutation.isPending}
              onClick={() => deletingAddr && deleteMutation.mutate(deletingAddr)}
            >
              {t('common.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  )
}
