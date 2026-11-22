import { useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  AlertTriangle,
  CheckCircle2,
  Globe,
  Loader2,
  Plus,
  Settings2,
  Star,
  Trash2,
} from 'lucide-react'
import { domainApi, type MailDomain } from '@/api'
import { queryKeys } from '@/hooks/useMailQueries'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { DnsWizardModal } from './DnsWizardModal'

export function DomainsSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const { data: domains = [], isLoading } = useQuery({
    queryKey: queryKeys.domains,
    queryFn: () => domainApi.list(),
  })

  // Fetch cached DNS status for each domain to show live status badges
  const dnsQueries = useQueries({
    queries: domains.map((d) => ({
      queryKey: queryKeys.domainDns(d.id),
      queryFn: () => domainApi.dns(d.id, false),
      staleTime: 60_000,
      retry: false,
    })),
  })

  const [addOpen, setAddOpen] = useState(false)
  const [wizardDomain, setWizardDomain] = useState<{ domain: MailDomain; step: 1 | 2 } | null>(null)
  const [deleting, setDeleting] = useState<MailDomain | null>(null)

  const schema = useMemo(
    () =>
      z.object({
        name: z
          .string()
          .trim()
          .min(1, t('admin.domains.validation.nameRequired'))
          .regex(
            /^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$/,
            t('admin.domains.validation.nameFormat'),
          ),
      }),
    [t],
  )

  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { name: '' },
  })

  const createMutation = useMutation({
    mutationFn: (name: string) => domainApi.create(name.toLowerCase()),
    onSuccess: (created) => {
      toast.success(t('admin.domains.created', { name: created.name }))
      setAddOpen(false)
      form.reset({ name: '' })
      void queryClient.invalidateQueries({ queryKey: queryKeys.domains })
      // Immediately enter step-by-step DNS setup wizard!
      setWizardDomain({ domain: created, step: 1 })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const setDefaultMutation = useMutation({
    mutationFn: (d: MailDomain) => domainApi.setDefault(d.id),
    onSuccess: (_res, d) => {
      toast.success(t('admin.domains.defaultUpdated', { name: d.name }))
      void queryClient.invalidateQueries({ queryKey: queryKeys.domains })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (d: MailDomain) => domainApi.remove(d.id),
    onSuccess: (_res, d) => {
      toast.success(t('admin.domains.deleted', { name: d.name }))
      setDeleting(null)
      void queryClient.invalidateQueries({ queryKey: queryKeys.domains })
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
            {t('admin.domains.title')}
          </h2>
          <p className="mt-0.5 text-[12.5px] text-[var(--ink-secondary)]">
            {t('admin.domains.desc')}
          </p>
        </div>
        <Button size="sm" onClick={() => setAddOpen(true)}>
          <Plus className="h-3.5 w-3.5" />
          {t('admin.domains.addButton')}
        </Button>
      </div>

      {isLoading ? (
        <div className="space-y-3">
          <Skeleton className="h-24 w-full rounded-[var(--radius-md)]" />
          <Skeleton className="h-24 w-full rounded-[var(--radius-md)]" />
        </div>
      ) : domains.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-[var(--radius-md)] border border-dashed border-[var(--border-subtle)] py-14 text-center">
          <Globe className="h-7 w-7 text-[var(--ink-muted)]" />
          <p className="mt-2.5 text-[14px] font-medium text-[var(--ink-primary)]">
            {t('admin.domains.empty')}
          </p>
          <p className="mt-1 text-[12.5px] text-[var(--ink-muted)]">
            {t('admin.domains.emptyDesc')}
          </p>
        </div>
      ) : (
        <div className="space-y-3">
          {domains.map((dom, idx) => {
            const dnsQuery = dnsQueries[idx]
            const dnsData = dnsQuery?.data
            const pendingCount = dnsData
              ? dnsData.records.filter((r) => r.required && r.status !== 'ok').length
              : null

            return (
              <div
                key={dom.id}
                className="flex flex-col gap-3 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-4 sm:flex-row sm:items-center sm:justify-between"
              >
                <div className="min-w-0 space-y-1.5">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-mono text-[15px] font-semibold text-[var(--ink-primary)]">
                      {dom.name}
                    </span>
                    {dom.is_default && (
                      <Badge variant="accent">{t('admin.domains.defaultBadge')}</Badge>
                    )}

                    {/* DNS Health Status Badge */}
                    {dnsQuery?.isLoading ? (
                      <Badge variant="default">
                        <Loader2 className="h-3 w-3 animate-spin" />
                        {t('admin.domains.statusChecking')}
                      </Badge>
                    ) : dnsData?.all_required_ok ? (
                      <Badge variant="success">
                        <CheckCircle2 className="h-3 w-3" />
                        {t('admin.domains.statusAllOk')}
                      </Badge>
                    ) : pendingCount !== null && pendingCount > 0 ? (
                      <Badge variant="warning">
                        <AlertTriangle className="h-3 w-3" />
                        {t('admin.domains.statusPending', { count: pendingCount })}
                      </Badge>
                    ) : (
                      <Badge variant="default">{t('admin.domains.statusUnknown')}</Badge>
                    )}
                  </div>

                  <p className="text-[12px] text-[var(--ink-muted)]">
                    {t('admin.domains.selectorLabel')}：
                    <code className="font-mono">{dom.dkim_selector || 'default'}</code>
                  </p>
                </div>

                <div className="flex flex-wrap items-center gap-2 self-end sm:self-center">
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => setWizardDomain({ domain: dom, step: 2 })}
                  >
                    <Settings2 className="h-3.5 w-3.5" />
                    {t('admin.domains.openWizard')}
                  </Button>

                  {!dom.is_default && (
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={setDefaultMutation.isPending}
                      onClick={() => setDefaultMutation.mutate(dom)}
                    >
                      <Star className="h-3.5 w-3.5" />
                      {t('admin.domains.setDefault')}
                    </Button>
                  )}

                  {!dom.is_default && (
                    <Button
                      variant="dangerGhost"
                      size="iconSm"
                      aria-label={t('common.delete')}
                      onClick={() => setDeleting(dom)}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  )}
                </div>
              </div>
            )
          })}
        </div>
      )}

      {/* Add Domain Modal */}
      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>{t('admin.domains.addModalTitle')}</DialogTitle>
            <DialogDescription>{t('admin.domains.addModalDesc')}</DialogDescription>
          </DialogHeader>
          <form
            onSubmit={form.handleSubmit((vals) => createMutation.mutate(vals.name))}
            className="mt-3 space-y-4"
          >
            <div className="space-y-1.5">
              <Label htmlFor="new_domain_name">{t('settings.addresses.domain')}</Label>
              <Input
                id="new_domain_name"
                {...form.register('name')}
                placeholder={t('admin.domains.domainPlaceholder')}
                autoFocus
              />
              {form.formState.errors.name && (
                <p className="text-[12px] text-[var(--danger)]">
                  {form.formState.errors.name.message}
                </p>
              )}
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

      {/* Delete Domain Confirm Dialog */}
      <Dialog open={deleting !== null} onOpenChange={(open) => !open && setDeleting(null)}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>
              {deleting ? t('admin.domains.deleteTitle', { name: deleting.name }) : ''}
            </DialogTitle>
            <DialogDescription>{t('admin.domains.deleteWarning')}</DialogDescription>
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

      {/* Step-by-Step DNS Setup Wizard */}
      {wizardDomain && (
        <DnsWizardModal
          domain={wizardDomain.domain}
          initialStep={wizardDomain.step}
          onClose={() => setWizardDomain(null)}
        />
      )}
    </section>
  )
}
