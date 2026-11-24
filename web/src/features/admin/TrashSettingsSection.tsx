import { useEffect, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Info, Loader2, Trash2 } from 'lucide-react'
import { adminTrashApi } from '@/api'
import { queryKeys, useAdminTrashQuery } from '@/hooks/useMailQueries'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  FormRow,
  SectionCard,
  SectionErrorState,
  SectionFooter,
  SectionHeader,
  SectionLoadingState,
} from '@/components/settings'

export function TrashSettingsSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { data, isLoading, error, refetch } = useAdminTrashQuery(true)

  const [daysInput, setDaysInput] = useState('30')
  const [validationError, setValidationError] = useState<string | null>(null)

  useEffect(() => {
    if (data !== undefined) {
      setDaysInput(String(data.trash_retention_days))
      setValidationError(null)
    }
  }, [data])

  const saveMutation = useMutation({
    mutationFn: (days: number) => adminTrashApi.update({ trash_retention_days: days }),
    onSuccess: (updated) => {
      queryClient.setQueryData(queryKeys.adminTrash, updated)
      toast.success(t('admin.trash.saved'))
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const trimmed = daysInput.trim()
    const num = Number(trimmed)
    if (!trimmed || !/^\d+$/.test(trimmed) || !Number.isInteger(num) || num < 0 || num > 3650) {
      setValidationError(t('admin.trash.errors.retentionInvalid'))
      return
    }
    setValidationError(null)
    saveMutation.mutate(num)
  }

  return (
    <SectionCard>
      <SectionHeader
        icon={Trash2}
        iconTone="warning"
        title={t('admin.trash.title')}
        description={t('admin.trash.desc')}
      />

      {error ? (
        <SectionErrorState
          message={error instanceof Error ? error.message : t('common.unknownError')}
          retryLabel={t('app.retry')}
          onRetry={() => void refetch()}
        />
      ) : isLoading ? (
        <SectionLoadingState rows={2} rowHeight="h-14" />
      ) : (
        <form onSubmit={handleSubmit} className="space-y-4">
          <FormRow
            surface
            label={t('admin.trash.retentionLabel')}
            htmlFor="admin_trash_retention_days"
            description={t('admin.trash.retentionHint')}
            error={validationError ?? undefined}
          >
            <div className="flex items-center gap-2 sm:max-w-xs">
              <Input
                id="admin_trash_retention_days"
                type="number"
                min={0}
                max={3650}
                step={1}
                value={daysInput}
                onChange={(e) => {
                  setDaysInput(e.target.value)
                  setValidationError(null)
                }}
                className="font-mono tabular-nums"
              />
              <span className="shrink-0 text-[13px] text-[var(--ink-secondary)]">
                {t('admin.trash.daysUnit')}
              </span>
            </div>
          </FormRow>

          <div className="flex items-start gap-2.5 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] px-3.5 py-2.5 text-[12.5px] text-[var(--ink-secondary)]">
            <Info className="mt-0.5 h-4 w-4 shrink-0 text-[var(--accent)]" />
            <span>{t('admin.trash.zeroNote')}</span>
          </div>

          <SectionFooter>
            <Button type="submit" size="sm" disabled={saveMutation.isPending}>
              {saveMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
              {t('common.save')}
            </Button>
          </SectionFooter>
        </form>
      )}
    </SectionCard>
  )
}
