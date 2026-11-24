import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  Check,
  Clock,
  Copy,
  ExternalLink,
  Info,
  Loader2,
  RefreshCw,
  Send,
  Unlink,
} from 'lucide-react'
import {
  SYSTEM_GROUP_IDS,
  userTelegramApi,
  type TelegramBindCode,
  type TelegramFilters,
} from '@/api'
import { formatMailFullTime } from '@/lib/date'
import { queryKeys, useGroupOptionsQuery } from '@/hooks/useMailQueries'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
  SectionCard,
  SectionHeader,
  SectionLoadingState,
} from '@/components/settings'

function formatCountdown(seconds: number): string {
  const safe = Math.max(0, Math.floor(seconds))
  const m = Math.floor(safe / 60)
  const s = safe % 60
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}

export function TelegramSection() {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()

  const [bindInfo, setBindInfo] = useState<TelegramBindCode | null>(null)
  const [qrSvg, setQrSvg] = useState<string>('')
  const [secondsLeft, setSecondsLeft] = useState<number>(0)
  const [copiedCode, setCopiedCode] = useState(false)
  const [unbindOpen, setUnbindOpen] = useState(false)

  const isWaitingForBind = Boolean(bindInfo && secondsLeft > 0)

  const { data: status, isLoading } = useQuery({
    queryKey: queryKeys.userTelegram,
    queryFn: () => userTelegramApi.get(),
    refetchInterval: isWaitingForBind ? 3000 : false,
  })

  const { data: groups = [] } = useGroupOptionsQuery(Boolean(status?.bound))
  const customGroups = useMemo(
    () => groups.filter((g) => g.id < SYSTEM_GROUP_IDS.INBOX),
    [groups],
  )

  // Local filter form state
  const [allGroups, setAllGroups] = useState(true)
  const [selectedGroupIds, setSelectedGroupIds] = useState<number[]>([])
  const [onlyVerification, setOnlyVerification] = useState(false)
  const [includeSpam, setIncludeSpam] = useState(false)
  const [includePreview, setIncludePreview] = useState(true)
  const [quietEnabled, setQuietEnabled] = useState(false)
  const [quietStart, setQuietStart] = useState('23:00')
  const [quietEnd, setQuietEnd] = useState('08:00')
  const [quietTz, setQuietTz] = useState(
    () => Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Shanghai',
  )

  // Sync filters when status loads or updates
  useEffect(() => {
    if (!status) return
    if (status.bound && bindInfo) {
      setBindInfo(null)
      setQrSvg('')
      setSecondsLeft(0)
      toast.success(t('settings.telegram.boundSuccess'))
    }
    const f = status.filters
    if (f) {
      setAllGroups(f.group_ids === null)
      setSelectedGroupIds(f.group_ids ?? [])
      setOnlyVerification(f.only_verification)
      setIncludeSpam(f.include_spam)
      setIncludePreview(f.include_preview)
      if (f.quiet_hours) {
        setQuietEnabled(true)
        setQuietStart(f.quiet_hours.start || '23:00')
        setQuietEnd(f.quiet_hours.end || '08:00')
        setQuietTz(
          f.quiet_hours.tz ||
            Intl.DateTimeFormat().resolvedOptions().timeZone ||
            'Asia/Shanghai',
        )
      } else {
        setQuietEnabled(false)
      }
    }
  }, [status, bindInfo, t])

  // Countdown timer for bind code
  useEffect(() => {
    if (!bindInfo) return
    const computeRemaining = () => {
      const ms = new Date(bindInfo.expires_at).getTime() - Date.now()
      return Math.max(0, Math.ceil(ms / 1000))
    }
    setSecondsLeft(computeRemaining())
    const timer = window.setInterval(() => {
      setSecondsLeft(computeRemaining())
    }, 1000)
    return () => window.clearInterval(timer)
  }, [bindInfo])

  // Lazy-render QR SVG with uqr (zero runtime deps, only loaded when generating bind code)
  useEffect(() => {
    if (!bindInfo?.deep_link) {
      setQrSvg('')
      return
    }
    let cancelled = false
    void import('uqr').then(({ renderSVG }) => {
      if (cancelled) return
      const svg = renderSVG(bindInfo.deep_link, {
        border: 2,
        pixelSize: 6,
      })
      setQrSvg(svg)
    })
    return () => {
      cancelled = true
    }
  }, [bindInfo?.deep_link])

  const generateMutation = useMutation({
    mutationFn: () => userTelegramApi.bind(),
    onSuccess: (res) => {
      setBindInfo(res)
      setCopiedCode(false)
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const saveFiltersMutation = useMutation({
    mutationFn: (filters: TelegramFilters) => userTelegramApi.updateFilters(filters),
    onSuccess: (updated) => {
      queryClient.setQueryData(queryKeys.userTelegram, updated)
      toast.success(t('settings.telegram.filtersSaved'))
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const testPushMutation = useMutation({
    mutationFn: () => userTelegramApi.test(),
    onSuccess: () => {
      toast.success(t('settings.telegram.testPushSuccess'))
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const unbindMutation = useMutation({
    mutationFn: () => userTelegramApi.unbind(),
    onSuccess: () => {
      setUnbindOpen(false)
      toast.success(t('settings.telegram.unboundSuccess'))
      void queryClient.invalidateQueries({ queryKey: queryKeys.userTelegram })
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const handleCopyCode = async (code: string) => {
    try {
      await navigator.clipboard.writeText(code)
      setCopiedCode(true)
      toast.success(t('common.copied'))
      window.setTimeout(() => setCopiedCode(false), 1800)
    } catch {
      toast.error(t('common.copyFailed'))
    }
  }

  const toggleGroupId = (id: number) => {
    setSelectedGroupIds((prev) =>
      prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id],
    )
  }

  return (
    <SectionCard>
      <SectionHeader
        icon={Send}
        title={t('settings.telegram.title')}
        description={t('settings.telegram.desc')}
        badge={
          status?.available && status.bound ? (
            <Badge variant="success">{t('settings.telegram.boundBadge')}</Badge>
          ) : undefined
        }
      />

      {isLoading ? (
        <SectionLoadingState rows={2} rowHeight="h-24" />
      ) : !status?.available ? (
        /* Admin has not configured Telegram bot */
        <div className="flex items-start gap-3 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/70 p-4">
          <Info className="mt-0.5 h-4.5 w-4.5 shrink-0 text-[var(--ink-muted)]" />
          <div className="space-y-1">
            <p className="text-[13.5px] font-medium text-[var(--ink-primary)]">
              {t('settings.telegram.unavailableTitle')}
            </p>
            <p className="text-[12.5px] text-[var(--ink-muted)]">
              {t('settings.telegram.unavailableDesc')}
            </p>
          </div>
        </div>
      ) : !status.bound ? (
        /* Unbound flow: Generate bind code + deep link + QR + 10m countdown + 3s poll */
        <div className="rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-5 space-y-4">
          {!bindInfo ? (
            <div className="flex flex-col items-center py-6 text-center">
              <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-[var(--accent-soft)] text-[var(--accent)]">
                <Send className="h-5 w-5" />
              </div>
              <p className="mt-3 text-[14.5px] font-semibold text-[var(--ink-primary)]">
                {t('settings.telegram.unboundTitle')}
              </p>
              <p className="mt-1 max-w-md text-[12.5px] text-[var(--ink-muted)]">
                {t('settings.telegram.unboundDesc')}
              </p>
              <Button
                className="mt-4"
                size="sm"
                disabled={generateMutation.isPending}
                onClick={() => generateMutation.mutate()}
              >
                {generateMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                {generateMutation.isPending
                  ? t('settings.telegram.generating')
                  : t('settings.telegram.generateBindCode')}
              </Button>
            </div>
          ) : secondsLeft <= 0 ? (
            <div className="flex flex-col items-center py-6 text-center">
              <p className="text-[14px] font-semibold text-[var(--warning)]">
                {t('settings.telegram.expiredTitle')}
              </p>
              <Button
                className="mt-3"
                size="sm"
                disabled={generateMutation.isPending}
                onClick={() => generateMutation.mutate()}
              >
                <RefreshCw className="h-3.5 w-3.5" />
                {t('settings.telegram.regenerate')}
              </Button>
            </div>
          ) : (
            <div className="grid grid-cols-1 gap-5 sm:grid-cols-[1fr_auto] sm:items-center">
              <div className="space-y-3">
                <div className="space-y-1">
                  <span className="text-[12px] font-medium text-[var(--ink-muted)]">
                    {t('settings.telegram.bindCodeLabel')}
                  </span>
                  <div className="flex items-center gap-2">
                    <code className="rounded-[var(--radius-sm)] border border-[var(--border-subtle)] bg-[var(--surface-subtle)] px-3 py-1.5 font-mono text-[18px] font-bold tracking-[0.14em] text-[var(--ink-primary)] select-all">
                      {bindInfo.code}
                    </code>
                    <Button
                      type="button"
                      variant="secondary"
                      size="sm"
                      onClick={() => void handleCopyCode(bindInfo.code)}
                    >
                      {copiedCode ? (
                        <Check className="h-3.5 w-3.5 text-[var(--success)]" />
                      ) : (
                        <Copy className="h-3.5 w-3.5" />
                      )}
                      {t('common.copy')}
                    </Button>
                  </div>
                </div>

                <div className="flex flex-wrap items-center gap-2">
                  <Button asChild size="sm">
                    <a
                      href={bindInfo.deep_link}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      <ExternalLink className="h-3.5 w-3.5" />
                      {t('settings.telegram.openInTelegram')}
                    </a>
                  </Button>
                  <Badge variant="accent" className="font-mono">
                    <Clock className="h-3 w-3" />
                    {t('settings.telegram.countdownLabel', {
                      time: formatCountdown(secondsLeft),
                    })}
                  </Badge>
                </div>

                <p className="flex items-center gap-2 text-[12px] text-[var(--ink-muted)]">
                  <Loader2 className="h-3.5 w-3.5 animate-spin text-[var(--accent)]" />
                  <span>{t('settings.telegram.pollingHint')}</span>
                </p>
              </div>

              {qrSvg && (
                <div className="flex flex-col items-center gap-1.5 rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-white p-3">
                  <div
                    className="h-36 w-36 [&>svg]:h-full [&>svg]:w-full"
                    dangerouslySetInnerHTML={{ __html: qrSvg }}
                  />
                  <span className="max-w-[152px] text-center text-[11px] text-slate-600">
                    {t('settings.telegram.scanQrHint')}
                  </span>
                </div>
              )}
            </div>
          )}
        </div>
      ) : (
        /* Bound state + Filters */
        <div className="space-y-4">
          {/* Bound summary */}
          <div className="flex flex-wrap items-center justify-between gap-3 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-4">
            <div className="space-y-1">
              <div className="flex items-center gap-2 text-[13.5px] font-semibold text-[var(--ink-primary)]">
                <span>{t('settings.telegram.boundChat')}：</span>
                <span>{status.chat_title || 'Telegram'}</span>
              </div>
              {status.bound_at && (
                <p className="text-[12px] text-[var(--ink-muted)]">
                  {t('settings.telegram.boundAt')}：
                  {formatMailFullTime(status.bound_at, i18n.language)}
                </p>
              )}
            </div>

            <div className="flex flex-wrap items-center gap-2">
              <Button
                type="button"
                variant="secondary"
                size="sm"
                disabled={testPushMutation.isPending}
                onClick={() => testPushMutation.mutate()}
              >
                {testPushMutation.isPending ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <Send className="h-3.5 w-3.5" />
                )}
                {t('settings.telegram.testPush')}
              </Button>
              <Button
                type="button"
                variant="dangerGhost"
                size="sm"
                onClick={() => setUnbindOpen(true)}
              >
                <Unlink className="h-3.5 w-3.5" />
                {t('settings.telegram.unbind')}
              </Button>
            </div>
          </div>

          {/* Filters */}
          <div className="rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-4 space-y-4">
            <h3 className="text-[13.5px] font-semibold text-[var(--ink-primary)]">
              {t('settings.telegram.filtersTitle')}
            </h3>

            {/* Group multi-select */}
            <div className="space-y-2">
              <Label>{t('settings.telegram.groupsLabel')}</Label>
              <div className="flex flex-wrap gap-2">
                <label className="inline-flex cursor-pointer items-center gap-2 rounded-[var(--radius-sm)] border border-[var(--border-subtle)] bg-[var(--surface-subtle)]/70 px-3 py-1.5 text-[12.5px]">
                  <Checkbox
                    checked={allGroups}
                    onCheckedChange={(checked) => {
                      const isAll = Boolean(checked)
                      setAllGroups(isAll)
                      if (isAll) setSelectedGroupIds([])
                    }}
                  />
                  <span>{t('settings.telegram.groupsAll')}</span>
                </label>

                {!allGroups &&
                  customGroups.map((g) => (
                    <label
                      key={g.id}
                      className="inline-flex cursor-pointer items-center gap-2 rounded-[var(--radius-sm)] border border-[var(--border-subtle)] bg-[var(--surface-canvas)] px-3 py-1.5 text-[12.5px]"
                    >
                      <Checkbox
                        checked={selectedGroupIds.includes(g.id)}
                        onCheckedChange={() => toggleGroupId(g.id)}
                      />
                      <span>{g.name}</span>
                    </label>
                  ))}
              </div>
              <p className="text-[11.5px] text-[var(--ink-muted)]">
                {t('settings.telegram.groupsCustomHint')}
              </p>
            </div>

            {/* Boolean filter switches */}
            <div className="divide-y divide-[var(--border-hairline)] rounded-[var(--radius-sm)] border border-[var(--border-hairline)]">
              <div className="flex items-center justify-between gap-4 px-3.5 py-2.5">
                <div>
                  <p className="text-[13px] font-medium text-[var(--ink-primary)]">
                    {t('settings.telegram.onlyVerificationLabel')}
                  </p>
                  <p className="text-[11.5px] text-[var(--ink-muted)]">
                    {t('settings.telegram.onlyVerificationDesc')}
                  </p>
                </div>
                <Switch
                  checked={onlyVerification}
                  onCheckedChange={setOnlyVerification}
                />
              </div>

              <div className="flex items-center justify-between gap-4 px-3.5 py-2.5">
                <div>
                  <p className="text-[13px] font-medium text-[var(--ink-primary)]">
                    {t('settings.telegram.includeSpamLabel')}
                  </p>
                  <p className="text-[11.5px] text-[var(--ink-muted)]">
                    {t('settings.telegram.includeSpamDesc')}
                  </p>
                </div>
                <Switch checked={includeSpam} onCheckedChange={setIncludeSpam} />
              </div>

              <div className="flex items-center justify-between gap-4 px-3.5 py-2.5">
                <div>
                  <p className="text-[13px] font-medium text-[var(--ink-primary)]">
                    {t('settings.telegram.includePreviewLabel')}
                  </p>
                  <p className="text-[11.5px] text-[var(--ink-muted)]">
                    {t('settings.telegram.includePreviewDesc')}
                  </p>
                </div>
                <Switch checked={includePreview} onCheckedChange={setIncludePreview} />
              </div>

              <div className="space-y-3 px-3.5 py-2.5">
                <div className="flex items-center justify-between gap-4">
                  <div>
                    <p className="text-[13px] font-medium text-[var(--ink-primary)]">
                      {t('settings.telegram.quietHoursLabel')}
                    </p>
                    <p className="text-[11.5px] text-[var(--ink-muted)]">
                      {t('settings.telegram.quietHoursDesc')}
                    </p>
                  </div>
                  <Switch checked={quietEnabled} onCheckedChange={setQuietEnabled} />
                </div>

                {quietEnabled && (
                  <div className="grid grid-cols-1 gap-2.5 sm:grid-cols-3">
                    <div className="space-y-1">
                      <Label htmlFor="tg_quiet_start">{t('settings.telegram.quietStart')}</Label>
                      <Input
                        id="tg_quiet_start"
                        type="time"
                        value={quietStart}
                        onChange={(e) => setQuietStart(e.target.value)}
                      />
                    </div>
                    <div className="space-y-1">
                      <Label htmlFor="tg_quiet_end">{t('settings.telegram.quietEnd')}</Label>
                      <Input
                        id="tg_quiet_end"
                        type="time"
                        value={quietEnd}
                        onChange={(e) => setQuietEnd(e.target.value)}
                      />
                    </div>
                    <div className="space-y-1">
                      <Label htmlFor="tg_quiet_tz">{t('settings.telegram.quietTz')}</Label>
                      <Input
                        id="tg_quiet_tz"
                        value={quietTz}
                        onChange={(e) => setQuietTz(e.target.value)}
                      />
                    </div>
                  </div>
                )}
              </div>
            </div>

            <div className="flex justify-end">
              <Button
                size="sm"
                disabled={saveFiltersMutation.isPending}
                onClick={() =>
                  saveFiltersMutation.mutate({
                    group_ids: allGroups ? null : selectedGroupIds,
                    only_verification: onlyVerification,
                    include_spam: includeSpam,
                    include_preview: includePreview,
                    quiet_hours: quietEnabled
                      ? { start: quietStart, end: quietEnd, tz: quietTz.trim() || 'UTC' }
                      : null,
                  })
                }
              >
                {saveFiltersMutation.isPending && (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                )}
                {t('common.save')}
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* Unbind confirmation dialog */}
      <Dialog open={unbindOpen} onOpenChange={setUnbindOpen}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{t('settings.telegram.unbindTitle')}</DialogTitle>
            <DialogDescription>{t('settings.telegram.unbindConfirm')}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setUnbindOpen(false)}
            >
              {t('common.cancel')}
            </Button>
            <Button
              type="button"
              variant="danger"
              size="sm"
              disabled={unbindMutation.isPending}
              onClick={() => unbindMutation.mutate()}
            >
              {unbindMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
              {t('settings.telegram.unbind')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SectionCard>
  )
}
