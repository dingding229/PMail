import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Forward, Loader2 } from 'lucide-react'
import { forwardApi } from '@/api'
import { isValidEmail } from '@/lib/format'
import { queryKeys } from '@/hooks/useMailQueries'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Skeleton } from '@/components/ui/skeleton'
import { RecipientChipInput } from '@/features/mail/RecipientChipInput'

export function ForwardSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const { data, isLoading } = useQuery({
    queryKey: queryKeys.forward,
    queryFn: () => forwardApi.get(),
  })

  const [enabled, setEnabled] = useState(false)
  const [targets, setTargets] = useState<string[]>([])
  const [keepCopy, setKeepCopy] = useState(true)
  const [errorMsg, setErrorMsg] = useState<string | null>(null)

  useEffect(() => {
    if (data) {
      setEnabled(Boolean(data.setting?.enabled))
      setTargets(data.targets ?? [])
      setKeepCopy(data.setting?.keep_copy ?? true)
    }
  }, [data])

  const saveMutation = useMutation({
    mutationFn: () => forwardApi.set({ enabled, targets, keep_copy: keepCopy }),
    onSuccess: () => {
      toast.success(t('settings.forward.saved'))
      void queryClient.invalidateQueries({ queryKey: queryKeys.forward })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const handleSave = (e: React.FormEvent) => {
    e.preventDefault()
    setErrorMsg(null)
    if (enabled && targets.length === 0) {
      setErrorMsg(t('settings.forward.validation.targetsRequired'))
      return
    }
    const invalid = targets.find((addr) => !isValidEmail(addr))
    if (invalid) {
      setErrorMsg(t('settings.forward.validation.invalidEmail', { email: invalid }))
      return
    }
    saveMutation.mutate()
  }

  if (isLoading) {
    return <Skeleton className="h-64 w-full rounded-[var(--radius-lg)]" />
  }

  return (
    <section className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] p-5 shadow-[var(--shadow-xs)]">
      <div className="mb-5 flex items-center gap-2.5">
        <div className="flex h-8 w-8 items-center justify-center rounded-[var(--radius-sm)] bg-[var(--accent-soft)] text-[var(--accent)]">
          <Forward className="h-4 w-4" />
        </div>
        <div>
          <h2 className="text-[15px] font-semibold text-[var(--ink-primary)]">
            {t('settings.forward.title')}
          </h2>
          <p className="text-[12.5px] text-[var(--ink-secondary)]">
            {t('settings.forward.desc')}
          </p>
        </div>
      </div>

      <form onSubmit={handleSave} className="space-y-5 max-w-xl">
        <div className="flex items-center justify-between gap-4 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/50 px-4 py-3">
          <div>
            <p className="text-[13.5px] font-medium text-[var(--ink-primary)]">
              {t('settings.forward.enableLabel')}
            </p>
            <p className="text-[12px] text-[var(--ink-muted)]">
              {t('settings.forward.enableDesc')}
            </p>
          </div>
          <Switch checked={enabled} onCheckedChange={setEnabled} />
        </div>

        <div className="space-y-1.5">
          <Label htmlFor="forward_targets">{t('settings.forward.targetsLabel')}</Label>
          <div className="rounded-[var(--radius-sm)] border border-[var(--border-subtle)] bg-[var(--surface-canvas)] px-2.5 py-0.5 focus-within:border-[var(--accent)] focus-within:ring-2 focus-within:ring-[var(--accent-ring)]">
            <RecipientChipInput
              id="forward_targets"
              values={targets}
              onChange={(next) => {
                setTargets(next)
                setErrorMsg(null)
              }}
              suggestions={[]}
              placeholder={t('settings.forward.targetsPlaceholder')}
            />
          </div>
          <p className="text-[12px] text-[var(--ink-muted)]">
            {t('settings.forward.targetsHint')}
          </p>
          {errorMsg && <p className="text-[12px] text-[var(--danger)]">{errorMsg}</p>}
        </div>

        <div className="flex items-center justify-between gap-4 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/50 px-4 py-3">
          <div>
            <p className="text-[13.5px] font-medium text-[var(--ink-primary)]">
              {t('settings.forward.keepCopyLabel')}
            </p>
            <p className="text-[12px] text-[var(--ink-muted)]">
              {t('settings.forward.keepCopyDesc')}
            </p>
          </div>
          <Switch checked={keepCopy} onCheckedChange={setKeepCopy} />
        </div>

        <Button type="submit" size="sm" disabled={saveMutation.isPending}>
          {saveMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
          {t('common.save')}
        </Button>
      </form>
    </section>
  )
}
