import { useMemo } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { FolderSync, Inbox, Info, Loader2 } from 'lucide-react'
import { reportMailApi, SYSTEM_GROUP_IDS, type ReportMailPreference } from '@/api'
import { queryKeys, useGroupOptionsQuery } from '@/hooks/useMailQueries'
import { Button } from '@/components/ui/button'
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
  SectionHeader,
  SectionLoadingState,
} from '@/components/settings'

const DEFAULT_PREF: ReportMailPreference = {
  enabled: true,
  group_id: 0,
  mark_read: true,
  forward_reports: false,
}

export function ReportMailSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const { data: pref = DEFAULT_PREF, isLoading } = useQuery({
    queryKey: queryKeys.reportMailPreference,
    queryFn: () => reportMailApi.getPreference(),
  })

  const { data: groupOptions = [] } = useGroupOptionsQuery(true)

  const customFolders = useMemo(
    () => groupOptions.filter((g) => g.id > 0 && g.id < SYSTEM_GROUP_IDS.INBOX),
    [groupOptions],
  )

  const saveMutation = useMutation({
    mutationFn: (next: ReportMailPreference) => reportMailApi.savePreference(next),
    onSuccess: (saved) => {
      queryClient.setQueryData(queryKeys.reportMailPreference, saved)
      toast.success(t('settings.reportMail.savedToast'))
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.saveFailed'))
    },
  })

  const applyMutation = useMutation({
    mutationFn: () => reportMailApi.apply(),
    onSuccess: (res) => {
      toast.success(t('settings.reportMail.appliedToast', { count: res.moved }))
      queryClient.invalidateQueries({ queryKey: queryKeys.sidebarGroups })
      queryClient.invalidateQueries({ queryKey: queryKeys.groupOptions })
      queryClient.invalidateQueries({ queryKey: queryKeys.emailListsRoot })
      queryClient.invalidateQueries({ queryKey: queryKeys.reportMailPreference })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.operationFailed'))
    },
  })

  const updatePref = (patch: Partial<ReportMailPreference>) => {
    saveMutation.mutate({ ...pref, ...patch })
  }

  const selectedFolderValue = useMemo(() => {
    if (pref.group_id === 0) return '0'
    if (customFolders.some((g) => g.id === pref.group_id)) {
      return String(pref.group_id)
    }
    return '0'
  }, [pref.group_id, customFolders])

  return (
    <SectionCard>
      <SectionHeader
        icon={Inbox}
        title={t('settings.reportMail.title')}
        description={t('settings.reportMail.desc')}
      />

      {isLoading ? (
        <SectionLoadingState />
      ) : (
        <div className="space-y-4">
          <div className="flex items-center justify-between gap-4 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/50 px-4 py-3">
            <div className="min-w-0">
              <Label
                htmlFor="report_mail_enabled"
                className="text-[13px] font-medium text-[var(--ink-primary)]"
              >
                {t('settings.reportMail.enabledLabel')}
              </Label>
              <p className="mt-0.5 text-[12px] text-[var(--ink-secondary)]">
                {t('settings.reportMail.enabledDesc')}
              </p>
            </div>
            <Switch
              id="report_mail_enabled"
              checked={pref.enabled}
              disabled={saveMutation.isPending}
              onCheckedChange={(checked) => updatePref({ enabled: checked })}
            />
          </div>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="report_mail_folder">
                {t('settings.reportMail.targetFolderLabel')}
              </Label>
              <Select
                value={selectedFolderValue}
                disabled={!pref.enabled || saveMutation.isPending}
                onValueChange={(val) => updatePref({ group_id: Number(val) || 0 })}
              >
                <SelectTrigger id="report_mail_folder">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="0">
                    {t('settings.reportMail.autoCreateFolder')}
                  </SelectItem>
                  {customFolders.map((g) => (
                    <SelectItem key={g.id} value={String(g.id)}>
                      {g.full_path || g.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="flex items-center justify-between gap-4 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/50 px-4 py-2.5">
              <div className="min-w-0">
                <Label
                  htmlFor="report_mail_mark_read"
                  className="text-[13px] font-medium text-[var(--ink-primary)]"
                >
                  {t('settings.reportMail.markReadLabel')}
                </Label>
                <p className="mt-0.5 text-[12px] text-[var(--ink-secondary)]">
                  {t('settings.reportMail.markReadDesc')}
                </p>
              </div>
              <Switch
                id="report_mail_mark_read"
                checked={pref.mark_read}
                disabled={!pref.enabled || saveMutation.isPending}
                onCheckedChange={(checked) => updatePref({ mark_read: checked })}
              />
            </div>

            <div className="flex items-center justify-between gap-4 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/50 px-4 py-2.5">
              <div className="min-w-0">
                <Label
                  htmlFor="report_mail_forward"
                  className="text-[13px] font-medium text-[var(--ink-primary)]"
                >
                  {t('settings.reportMail.forwardLabel')}
                </Label>
                <p className="mt-0.5 text-[12px] text-[var(--ink-secondary)]">
                  {t('settings.reportMail.forwardDesc')}
                </p>
              </div>
              <Switch
                id="report_mail_forward"
                aria-label={t('settings.reportMail.forwardLabel')}
                checked={pref.forward_reports ?? false}
                disabled={!pref.enabled || saveMutation.isPending}
                onCheckedChange={(checked) => updatePref({ forward_reports: checked })}
              />
            </div>
          </div>

          <div className="flex items-start gap-2 rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/60 px-3.5 py-2.5 text-[12px] text-[var(--ink-secondary)]">
            <Info className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[var(--ink-muted)]" />
            <span>{t('settings.reportMail.notice')}</span>
          </div>

          <div className="flex items-center justify-end border-t border-[var(--border-hairline)] pt-3">
            <Button
              type="button"
              variant="secondary"
              size="sm"
              disabled={applyMutation.isPending}
              onClick={() => applyMutation.mutate()}
            >
              {applyMutation.isPending ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <FolderSync className="h-3.5 w-3.5" />
              )}
              {t('settings.reportMail.applyExisting')}
            </Button>
          </div>
        </div>
      )}
    </SectionCard>
  )
}
