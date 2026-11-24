import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Forward, Loader2 } from 'lucide-react'
import { forwardApi } from '@/api'
import { isValidEmail } from '@/lib/format'
import { queryKeys } from '@/hooks/useMailQueries'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import {
  FormRow,
  SectionCard,
  SectionFooter,
  SectionHeader,
  SectionLoadingState,
} from '@/components/settings'
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
    return (
      <SectionCard>
        <SectionLoadingState rows={3} rowHeight="h-16" />
      </SectionCard>
    )
  }

  return (
    <SectionCard>
      <SectionHeader
        icon={Forward}
        title={t('settings.forward.title')}
        description={t('settings.forward.desc')}
      />

      <form onSubmit={handleSave} className="space-y-4">
        <FormRow
          layout="horizontal"
          surface
          label={t('settings.forward.enableLabel')}
          description={t('settings.forward.enableDesc')}
        >
          <Switch checked={enabled} onCheckedChange={setEnabled} />
        </FormRow>

        <FormRow
          surface
          label={t('settings.forward.targetsLabel')}
          htmlFor="forward_targets"
          description={t('settings.forward.targetsHint')}
          error={errorMsg ?? undefined}
        >
          <div className="rounded-[var(--radius-sm)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] px-2.5 py-0.5 focus-within:border-[var(--accent)] focus-within:ring-2 focus-within:ring-[var(--accent-ring)]">
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
        </FormRow>

        <FormRow
          layout="horizontal"
          surface
          label={t('settings.forward.keepCopyLabel')}
          description={t('settings.forward.keepCopyDesc')}
        >
          <Switch checked={keepCopy} onCheckedChange={setKeepCopy} />
        </FormRow>

        <SectionFooter>
          <Button type="submit" size="sm" disabled={saveMutation.isPending}>
            {saveMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
            {t('common.save')}
          </Button>
        </SectionFooter>
      </form>
    </SectionCard>
  )
}
