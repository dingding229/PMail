import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Loader2, Plus, ShieldCheck, X } from 'lucide-react'
import { adminInboundApi, type InboundSettings } from '@/api'
import { queryKeys } from '@/hooks/useMailQueries'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Skeleton } from '@/components/ui/skeleton'

export function InboundSecuritySection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const { data, isLoading } = useQuery({
    queryKey: queryKeys.adminInbound,
    queryFn: () => adminInboundApi.get(),
  })

  const [dnsblEnabled, setDnsblEnabled] = useState(false)
  const [dnsblZones, setDnsblZones] = useState<string[]>([])
  const [newZone, setNewZone] = useState('')
  const [defaultThreshold, setDefaultThreshold] = useState(6)
  const [clamdEnabled, setClamdEnabled] = useState(false)
  const [clamdAddr, setClamdAddr] = useState('')
  const [blockDangerous, setBlockDangerous] = useState(true)

  useEffect(() => {
    if (!data) return
    setDnsblEnabled(data.dnsbl_enabled)
    setDnsblZones(data.dnsbl_zones ?? [])
    setDefaultThreshold(data.default_spam_threshold)
    setClamdEnabled(data.clamd_enabled)
    setClamdAddr(data.clamd_addr ?? '')
    setBlockDangerous(data.risk_block_dangerous_attachments)
  }, [data])

  const saveMutation = useMutation({
    mutationFn: (payload: InboundSettings) => {
      if (payload.clamd_enabled && !payload.clamd_addr.trim()) {
        throw new Error(t('admin.inbound.clamdAddrRequired'))
      }
      return adminInboundApi.update(payload)
    },
    onSuccess: (updated) => {
      queryClient.setQueryData(queryKeys.adminInbound, updated)
      toast.success(t('admin.inbound.saved'))
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const handleAddZone = () => {
    const trimmed = newZone.trim().toLowerCase()
    if (!trimmed) return
    if (dnsblZones.some((z) => z.toLowerCase() === trimmed)) {
      toast.error(t('admin.inbound.zoneExists'))
      return
    }
    setDnsblZones((prev) => [...prev, trimmed])
    setNewZone('')
  }

  const handleRemoveZone = (zone: string) => {
    setDnsblZones((prev) => prev.filter((z) => z !== zone))
  }

  return (
    <section className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] p-5 shadow-[var(--shadow-xs)] space-y-5">
      <div className="flex items-start gap-3">
        <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-[var(--radius-md)] bg-[var(--accent-soft)] text-[var(--accent)]">
          <ShieldCheck className="h-4.5 w-4.5" />
        </div>
        <div>
          <h2 className="text-[15px] font-semibold text-[var(--ink-primary)]">
            {t('admin.inbound.title')}
          </h2>
          <p className="mt-0.5 text-[12.5px] text-[var(--ink-secondary)]">
            {t('admin.inbound.desc')}
          </p>
        </div>
      </div>

      {isLoading ? (
        <div className="space-y-3">
          <Skeleton className="h-24 w-full rounded-[var(--radius-md)]" />
          <Skeleton className="h-20 w-full rounded-[var(--radius-md)]" />
          <Skeleton className="h-24 w-full rounded-[var(--radius-md)]" />
        </div>
      ) : (
        <div className="space-y-4">
          {/* DNSBL */}
          <div className="rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-4 space-y-3">
            <div className="flex items-center justify-between gap-4">
              <div className="min-w-0">
                <p className="text-[13.5px] font-medium text-[var(--ink-primary)]">
                  {t('admin.inbound.dnsblLabel')}
                </p>
                <p className="mt-0.5 text-[12px] text-[var(--ink-muted)]">
                  {t('admin.inbound.dnsblDesc')}
                </p>
              </div>
              <Switch checked={dnsblEnabled} onCheckedChange={setDnsblEnabled} />
            </div>

            <div className="space-y-2 border-t border-[var(--border-hairline)] pt-3">
              <Label>{t('admin.inbound.dnsblZonesLabel')}</Label>
              <div className="flex gap-2">
                <Input
                  value={newZone}
                  onChange={(e) => setNewZone(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      e.preventDefault()
                      handleAddZone()
                    }
                  }}
                  placeholder={t('admin.inbound.dnsblZonePlaceholder')}
                  disabled={!dnsblEnabled}
                />
                <Button
                  type="button"
                  variant="secondary"
                  size="sm"
                  className="h-9 shrink-0"
                  disabled={!dnsblEnabled || !newZone.trim()}
                  onClick={handleAddZone}
                >
                  <Plus className="h-3.5 w-3.5" />
                  {t('admin.inbound.dnsblAddZone')}
                </Button>
              </div>

              {dnsblZones.length === 0 ? (
                <p className="text-[12px] text-[var(--ink-muted)]">
                  {t('admin.inbound.dnsblEmptyZones')}
                </p>
              ) : (
                <div className="flex flex-wrap gap-1.5 pt-1">
                  {dnsblZones.map((zone) => (
                    <span
                      key={zone}
                      className="inline-flex items-center gap-1.5 rounded-full border border-[var(--border-subtle)] bg-[var(--surface-subtle)] px-2.5 py-1 font-mono text-[12px] text-[var(--ink-primary)]"
                    >
                      {zone}
                      <button
                        type="button"
                        onClick={() => handleRemoveZone(zone)}
                        aria-label={t('common.delete')}
                        className="rounded-full p-0.5 text-[var(--ink-muted)] hover:bg-[var(--surface-hover)] hover:text-[var(--danger)]"
                      >
                        <X className="h-3 w-3" />
                      </button>
                    </span>
                  ))}
                </div>
              )}
            </div>
          </div>

          {/* Default Spam Threshold */}
          <div className="rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-4 space-y-2.5">
            <div className="flex items-center justify-between gap-2">
              <Label htmlFor="admin_spam_threshold" className="text-[13.5px] font-medium">
                {t('admin.inbound.defaultThresholdLabel')}
              </Label>
              <span className="rounded-[var(--radius-sm)] bg-[var(--accent-soft)] px-2.5 py-0.5 font-mono text-[13px] font-semibold tabular-nums text-[var(--accent-ink)]">
                {defaultThreshold}
              </span>
            </div>
            <input
              id="admin_spam_threshold"
              type="range"
              min={1}
              max={20}
              step={0.5}
              value={defaultThreshold}
              onChange={(e) => setDefaultThreshold(Number(e.target.value))}
              className="w-full accent-[var(--accent)] cursor-pointer"
            />
            <div className="flex items-center justify-between text-[11px] text-[var(--ink-muted)]">
              <span>{t('settings.spam.thresholdStrict')}</span>
              <span>{t('settings.spam.thresholdBalanced')}</span>
              <span>{t('settings.spam.thresholdLoose')}</span>
            </div>
            <p className="text-[12px] text-[var(--ink-muted)]">
              {t('admin.inbound.defaultThresholdHint')}
            </p>
          </div>

          {/* ClamAV */}
          <div className="rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-4 space-y-3">
            <div className="flex items-center justify-between gap-4">
              <div className="min-w-0">
                <p className="text-[13.5px] font-medium text-[var(--ink-primary)]">
                  {t('admin.inbound.clamdLabel')}
                </p>
                <p className="mt-0.5 text-[12px] text-[var(--ink-muted)]">
                  {t('admin.inbound.clamdDesc')}
                </p>
              </div>
              <Switch checked={clamdEnabled} onCheckedChange={setClamdEnabled} />
            </div>

            <div className="space-y-1.5 border-t border-[var(--border-hairline)] pt-3">
              <Label htmlFor="clamd_addr">{t('admin.inbound.clamdAddrLabel')}</Label>
              <Input
                id="clamd_addr"
                value={clamdAddr}
                onChange={(e) => setClamdAddr(e.target.value)}
                placeholder={t('admin.inbound.clamdAddrPlaceholder')}
                disabled={!clamdEnabled}
              />
              <p className="text-[11.5px] text-[var(--ink-muted)]">
                {t('admin.inbound.clamdAddrHint')}
              </p>
            </div>
          </div>

          {/* Dangerous Attachments Blocking */}
          <div className="flex items-center justify-between gap-4 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] px-4 py-3">
            <div className="min-w-0">
              <p className="text-[13.5px] font-medium text-[var(--ink-primary)]">
                {t('admin.inbound.blockDangerousLabel')}
              </p>
              <p className="mt-0.5 text-[12px] text-[var(--ink-muted)]">
                {t('admin.inbound.blockDangerousDesc')}
              </p>
            </div>
            <Switch checked={blockDangerous} onCheckedChange={setBlockDangerous} />
          </div>

          <div className="flex justify-end">
            <Button
              size="sm"
              disabled={saveMutation.isPending}
              onClick={() =>
                saveMutation.mutate({
                  dnsbl_enabled: dnsblEnabled,
                  dnsbl_zones: dnsblZones,
                  default_spam_threshold: defaultThreshold,
                  clamd_enabled: clamdEnabled,
                  clamd_addr: clamdAddr.trim(),
                  risk_block_dangerous_attachments: blockDangerous,
                })
              }
            >
              {saveMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
              {t('common.save')}
            </Button>
          </div>
        </div>
      )}
    </section>
  )
}
