import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  CheckCircle2,
  KeyRound,
  Loader2,
  PlugZap,
  Send,
  Trash2,
} from 'lucide-react'
import { adminTelegramApi, type AdminTelegramTestResult } from '@/api'
import { queryKeys } from '@/hooks/useMailQueries'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
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
  SectionErrorState,
  SectionFooter,
  SectionHeader,
  SectionLoadingState,
} from '@/components/settings'

export function AdminTelegramSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const { data: status, isLoading, error, refetch } = useQuery({
    queryKey: queryKeys.adminTelegram,
    queryFn: () => adminTelegramApi.get(),
  })

  const [replacingToken, setReplacingToken] = useState(false)
  const [botToken, setBotToken] = useState('')
  const [proxy, setProxy] = useState('')
  const [proxyTouched, setProxyTouched] = useState(false)
  const [clearConfirmOpen, setClearConfirmOpen] = useState(false)
  const [testResult, setTestResult] = useState<AdminTelegramTestResult | null>(null)

  const saveMutation = useMutation({
    mutationFn: () => {
      const payload: { bot_token?: string; proxy?: string | null } = {}
      if (!status?.configured || replacingToken) {
        if (botToken.trim()) {
          payload.bot_token = botToken.trim()
        }
      }
      if (proxyTouched) {
        payload.proxy = proxy.trim() ? proxy.trim() : null
      }
      return adminTelegramApi.update(payload)
    },
    onSuccess: (updated) => {
      queryClient.setQueryData(queryKeys.adminTelegram, updated)
      setBotToken('')
      setReplacingToken(false)
      setProxyTouched(false)
      setTestResult(null)
      toast.success(t('admin.telegram.saved'))
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const clearMutation = useMutation({
    mutationFn: () => adminTelegramApi.update({ bot_token: '', proxy: null }),
    onSuccess: (updated) => {
      queryClient.setQueryData(queryKeys.adminTelegram, updated)
      setBotToken('')
      setProxy('')
      setReplacingToken(false)
      setProxyTouched(false)
      setClearConfirmOpen(false)
      setTestResult(null)
      toast.success(t('admin.telegram.cleared'))
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const testMutation = useMutation({
    mutationFn: () => adminTelegramApi.test(),
    onSuccess: (res) => {
      setTestResult(res)
      toast.success(
        t('admin.telegram.testSuccess', {
          username: res.bot_username,
          latency: res.latency_ms,
        }),
      )
    },
    onError: (err) => {
      setTestResult(null)
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const showTokenInput = !status?.configured || replacingToken

  return (
    <SectionCard>
      <SectionHeader
        icon={Send}
        title={t('admin.telegram.title')}
        description={t('admin.telegram.desc')}
        action={
          !isLoading && status ? (
            <>
              <Badge variant={status.configured ? 'success' : 'default'}>
                {status.configured && status.bot_username
                  ? t('admin.telegram.configuredBadge', { username: status.bot_username })
                  : t('admin.telegram.notConfiguredBadge')}
              </Badge>
              <Badge variant="default">{t('admin.telegram.webhookModeLabel')}</Badge>
            </>
          ) : undefined
        }
      />

      {error ? (
        <SectionErrorState
          message={error instanceof Error ? error.message : t('common.unknownError')}
          retryLabel={t('app.retry')}
          onRetry={() => void refetch()}
        />
      ) : isLoading ? (
        <SectionLoadingState rows={2} rowHeight="h-16" />
      ) : (
        <form
          onSubmit={(e) => {
            e.preventDefault()
            saveMutation.mutate()
          }}
          className="space-y-4"
        >
          {/* Bot Token (write-only) */}
          <div className="rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-4 space-y-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <Label htmlFor="tg_bot_token">{t('admin.telegram.tokenLabel')}</Label>
              {status?.configured && !replacingToken && (
                <div className="flex items-center gap-1.5">
                  <Button
                    type="button"
                    variant="secondary"
                    size="xs"
                    onClick={() => setReplacingToken(true)}
                  >
                    <KeyRound className="h-3.5 w-3.5" />
                    {t('admin.telegram.replaceToken')}
                  </Button>
                  <Button
                    type="button"
                    variant="dangerGhost"
                    size="xs"
                    onClick={() => setClearConfirmOpen(true)}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                    {t('admin.telegram.clearToken')}
                  </Button>
                </div>
              )}
            </div>

            {showTokenInput ? (
              <div className="space-y-2">
                <Input
                  id="tg_bot_token"
                  type="password"
                  value={botToken}
                  onChange={(e) => setBotToken(e.target.value)}
                  placeholder={t('admin.telegram.tokenPlaceholder')}
                  autoComplete="new-password"
                />
                {replacingToken && (
                  <Button
                    type="button"
                    variant="ghost"
                    size="xs"
                    onClick={() => {
                      setReplacingToken(false)
                      setBotToken('')
                    }}
                  >
                    {t('common.cancel')}
                  </Button>
                )}
              </div>
            ) : (
              <p className="text-[12.5px] text-[var(--ink-muted)]">
                {t('admin.telegram.tokenKeepHint')}
              </p>
            )}
          </div>

          {/* Proxy URL */}
          <div className="rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-4 space-y-2">
            <Label htmlFor="tg_proxy">{t('admin.telegram.proxyLabel')}</Label>
            <Input
              id="tg_proxy"
              value={proxy}
              onChange={(e) => {
                setProxy(e.target.value)
                setProxyTouched(true)
              }}
              placeholder={t('admin.telegram.proxyPlaceholder')}
            />
            {status?.proxy_set && !proxyTouched && (
              <p className="text-[12px] text-[var(--ink-muted)]">
                {t('admin.telegram.proxySetHint')}
              </p>
            )}
          </div>

          {/* Test Result Banner */}
          {testResult && (
            <div className="flex items-center gap-2 rounded-[var(--radius-sm)] border border-[var(--success)]/35 bg-[var(--success-soft)] px-3 py-2 text-[12.5px] text-[var(--success)]">
              <CheckCircle2 className="h-4 w-4 shrink-0" />
              <span>
                {t('admin.telegram.testSuccess', {
                  username: testResult.bot_username,
                  latency: testResult.latency_ms,
                })}
              </span>
            </div>
          )}

          <SectionFooter align="between">
            <Button
              type="button"
              variant="secondary"
              size="sm"
              disabled={!status?.configured || testMutation.isPending}
              onClick={() => testMutation.mutate()}
            >
              {testMutation.isPending ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <PlugZap className="h-3.5 w-3.5" />
              )}
              {testMutation.isPending
                ? t('admin.telegram.testing')
                : t('admin.telegram.testButton')}
            </Button>

            <Button
              type="submit"
              size="sm"
              disabled={
                saveMutation.isPending ||
                (!botToken.trim() && !proxyTouched)
              }
            >
              {saveMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
              {t('common.save')}
            </Button>
          </SectionFooter>
        </form>
      )}

      {/* Clear confirm dialog */}
      <Dialog open={clearConfirmOpen} onOpenChange={setClearConfirmOpen}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{t('admin.telegram.clearConfirmTitle')}</DialogTitle>
            <DialogDescription>{t('admin.telegram.clearConfirmDesc')}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setClearConfirmOpen(false)}
            >
              {t('common.cancel')}
            </Button>
            <Button
              type="button"
              variant="danger"
              size="sm"
              disabled={clearMutation.isPending}
              onClick={() => clearMutation.mutate()}
            >
              {clearMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
              {t('admin.telegram.clearToken')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </SectionCard>
  )
}
