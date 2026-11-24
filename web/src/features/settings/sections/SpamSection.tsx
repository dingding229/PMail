import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  AlertTriangle,
  Loader2,
  Plus,
  Search,
  ShieldBan,
  ShieldCheck,
  Trash2,
} from 'lucide-react'
import {
  spamRulesApi,
  spamSettingsApi,
  type SpamRuleAction,
  type SpamRuleKind,
  type SpamSettings,
} from '@/api'
import { queryKeys } from '@/hooks/useMailQueries'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
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
  EmptyState,
  FormRow,
  SectionCard,
  SectionFooter,
  SectionHeader,
  SectionLoadingState,
} from '@/components/settings'

const MAX_SPAM_RULES = 500

export function SpamSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const { data: settings, isLoading: settingsLoading } = useQuery({
    queryKey: queryKeys.spamSettings,
    queryFn: () => spamSettingsApi.get(),
  })

  const { data: rules = [], isLoading: rulesLoading } = useQuery({
    queryKey: queryKeys.spamRules,
    queryFn: () => spamRulesApi.list(),
  })

  const [enabled, setEnabled] = useState(true)
  const [threshold, setThreshold] = useState(6)
  const [bayesEnabled, setBayesEnabled] = useState(true)

  useEffect(() => {
    if (!settings) return
    setEnabled(settings.enabled)
    setThreshold(settings.threshold)
    setBayesEnabled(settings.bayes_enabled)
  }, [settings])

  const [ruleAction, setRuleAction] = useState<SpamRuleAction>('block')
  const [ruleKind, setRuleKind] = useState<SpamRuleKind>('address')
  const [ruleValue, setRuleValue] = useState('')
  const [searchQuery, setSearchQuery] = useState('')

  const saveSettingsMutation = useMutation({
    mutationFn: (payload: SpamSettings) => spamSettingsApi.update(payload),
    onSuccess: (updated) => {
      queryClient.setQueryData(queryKeys.spamSettings, updated)
      toast.success(t('settings.spam.saved'))
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const createRuleMutation = useMutation({
    mutationFn: () => {
      const value = ruleValue.trim()
      if (!value) {
        throw new Error(t('settings.spam.errors.valueRequired'))
      }
      if (rules.length >= MAX_SPAM_RULES) {
        throw new Error(t('settings.spam.errors.limitReached'))
      }
      return spamRulesApi.create({ action: ruleAction, kind: ruleKind, value })
    },
    onSuccess: () => {
      setRuleValue('')
      toast.success(t('settings.spam.added'))
      void queryClient.invalidateQueries({ queryKey: queryKeys.spamRules })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const deleteRuleMutation = useMutation({
    mutationFn: (id: number) => spamRulesApi.remove(id),
    onSuccess: () => {
      toast.success(t('settings.spam.deleted'))
      void queryClient.invalidateQueries({ queryKey: queryKeys.spamRules })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const filteredRules = useMemo(() => {
    const q = searchQuery.trim().toLowerCase()
    if (!q) return rules
    return rules.filter(
      (r) =>
        r.value.toLowerCase().includes(q) ||
        r.action.toLowerCase().includes(q) ||
        r.kind.toLowerCase().includes(q),
    )
  }, [rules, searchQuery])

  const isAtLimit = rules.length >= MAX_SPAM_RULES

  const valuePlaceholder =
    ruleKind === 'address'
      ? t('settings.spam.valuePlaceholderAddress')
      : ruleKind === 'domain'
        ? t('settings.spam.valuePlaceholderDomain')
        : t('settings.spam.valuePlaceholderKeyword')

  const kindLabel = (kind: SpamRuleKind) =>
    kind === 'address'
      ? t('settings.spam.kindAddress')
      : kind === 'domain'
        ? t('settings.spam.kindDomain')
        : t('settings.spam.kindKeyword')

  return (
    <div className="space-y-5">
      {/* Core Spam Filter Settings */}
      <SectionCard>
        <SectionHeader
          icon={ShieldBan}
          title={t('settings.spam.title')}
          description={t('settings.spam.desc')}
        />

        {settingsLoading ? (
          <SectionLoadingState rows={3} rowHeight="h-14" />
        ) : (
          <div className="space-y-4">
            {/* Enable toggle */}
            <FormRow
              layout="horizontal"
              surface
              label={t('settings.spam.enabledLabel')}
              description={t('settings.spam.enabledDesc')}
            >
              <Switch checked={enabled} onCheckedChange={setEnabled} />
            </FormRow>

            {/* Threshold slider (1-20) */}
            <div className="rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] px-4 py-3.5 space-y-2.5">
              <div className="flex items-center justify-between gap-2">
                <Label htmlFor="spam_threshold" className="text-[13.5px] font-medium">
                  {t('settings.spam.thresholdLabel')}
                </Label>
                <span className="rounded-[var(--radius-sm)] bg-[var(--accent-soft)] px-2.5 py-0.5 font-mono text-[13px] font-semibold tabular-nums text-[var(--accent-ink)]">
                  {threshold}
                </span>
              </div>
              <input
                id="spam_threshold"
                type="range"
                min={1}
                max={20}
                step={0.5}
                value={threshold}
                onChange={(e) => setThreshold(Number(e.target.value))}
                disabled={!enabled}
                className="w-full accent-[var(--accent)] cursor-pointer disabled:opacity-50"
              />
              <div className="flex items-center justify-between text-[11px] text-[var(--ink-muted)]">
                <span>{t('settings.spam.thresholdStrict')}</span>
                <span>{t('settings.spam.thresholdBalanced')}</span>
                <span>{t('settings.spam.thresholdLoose')}</span>
              </div>
              <p className="text-[12px] text-[var(--ink-muted)]">
                {t('settings.spam.thresholdHint')}
              </p>
            </div>

            {/* Bayesian learning toggle */}
            <FormRow
              layout="horizontal"
              surface
              label={t('settings.spam.bayesLabel')}
              description={t('settings.spam.bayesDesc')}
            >
              <Switch
                checked={bayesEnabled}
                onCheckedChange={setBayesEnabled}
                disabled={!enabled}
              />
            </FormRow>

            <SectionFooter>
              <Button
                size="sm"
                disabled={saveSettingsMutation.isPending}
                onClick={() =>
                  saveSettingsMutation.mutate({
                    enabled,
                    threshold,
                    bayes_enabled: bayesEnabled,
                  })
                }
              >
                {saveSettingsMutation.isPending && (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                )}
                {t('common.save')}
              </Button>
            </SectionFooter>
          </div>
        )}
      </SectionCard>

      {/* Allow / Block Rules (max 500) */}
      <SectionCard>
        <SectionHeader
          icon={ShieldCheck}
          iconTone="neutral"
          title={t('settings.spam.rulesTitle')}
          description={t('settings.spam.rulesDesc')}
          badge={
            <Badge variant={isAtLimit ? 'danger' : 'default'}>
              {t('settings.spam.rulesCount', { count: rules.length })}
            </Badge>
          }
        />

        {isAtLimit && (
          <div className="flex items-start gap-2 rounded-[var(--radius-sm)] border border-[var(--danger)]/35 bg-[var(--danger-soft)] px-3 py-2 text-[12.5px] text-[var(--danger)]">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
            <span>{t('settings.spam.limitWarning')}</span>
          </div>
        )}

        {/* Add rule form */}
        <form
          onSubmit={(e) => {
            e.preventDefault()
            createRuleMutation.mutate()
          }}
          className="grid grid-cols-1 gap-2.5 sm:grid-cols-[150px_130px_1fr_auto]"
        >
          <Select value={ruleAction} onValueChange={(v) => setRuleAction(v as SpamRuleAction)}>
            <SelectTrigger className="h-9" aria-label={t('settings.spam.actionLabel')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="allow">{t('settings.spam.actionAllow')}</SelectItem>
              <SelectItem value="block">{t('settings.spam.actionBlock')}</SelectItem>
            </SelectContent>
          </Select>

          <Select value={ruleKind} onValueChange={(v) => setRuleKind(v as SpamRuleKind)}>
            <SelectTrigger className="h-9" aria-label={t('settings.spam.kindLabel')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="address">{t('settings.spam.kindAddress')}</SelectItem>
              <SelectItem value="domain">{t('settings.spam.kindDomain')}</SelectItem>
              <SelectItem value="keyword">{t('settings.spam.kindKeyword')}</SelectItem>
            </SelectContent>
          </Select>

          <Input
            value={ruleValue}
            onChange={(e) => setRuleValue(e.target.value)}
            placeholder={valuePlaceholder}
            maxLength={254}
            disabled={isAtLimit}
          />

          <Button
            type="submit"
            size="sm"
            className="h-9"
            disabled={isAtLimit || createRuleMutation.isPending}
          >
            {createRuleMutation.isPending ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <Plus className="h-3.5 w-3.5" />
            )}
            {t('settings.spam.addRule')}
          </Button>
        </form>

        {/* Search bar */}
        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[var(--ink-muted)]" />
          <Input
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder={t('settings.spam.searchPlaceholder')}
            className="pl-8"
          />
        </div>

        {/* Rule list */}
        {rulesLoading ? (
          <SectionLoadingState rows={2} rowHeight="h-11" />
        ) : filteredRules.length === 0 ? (
          <EmptyState
            icon={ShieldCheck}
            title={
              rules.length === 0
                ? t('settings.spam.emptyRules')
                : t('settings.spam.emptyFiltered')
            }
            compact
          />
        ) : (
          <div className="divide-y divide-[var(--border-hairline)] rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)]">
            {filteredRules.map((rule) => (
              <div
                key={rule.id}
                className="flex items-center justify-between gap-3 px-3.5 py-2.5"
              >
                <div className="flex min-w-0 flex-wrap items-center gap-2">
                  <Badge variant={rule.action === 'allow' ? 'success' : 'danger'}>
                    {rule.action === 'allow' ? (
                      <ShieldCheck className="h-3 w-3" />
                    ) : (
                      <ShieldBan className="h-3 w-3" />
                    )}
                    {rule.action === 'allow'
                      ? t('settings.spam.actionAllow')
                      : t('settings.spam.actionBlock')}
                  </Badge>
                  <Badge variant="default">{kindLabel(rule.kind)}</Badge>
                  <code className="truncate font-mono text-[12.5px] text-[var(--ink-primary)]">
                    {rule.value}
                  </code>
                </div>
                <Button
                  type="button"
                  variant="dangerGhost"
                  size="iconSm"
                  disabled={deleteRuleMutation.isPending}
                  onClick={() => deleteRuleMutation.mutate(rule.id)}
                  aria-label={t('common.delete')}
                  title={t('common.delete')}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </div>
            ))}
          </div>
        )}
      </SectionCard>
    </div>
  )
}
