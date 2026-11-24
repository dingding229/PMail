import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  AlertTriangle,
  CheckCircle2,
  Info,
  KeyRound,
  Languages,
  Loader2,
  PlugZap,
  Sparkles,
} from 'lucide-react'
import {
  adminTranslationApi,
  type AdminTranslationPolicyPatch,
  type AdminTranslationProxyMode,
  type AdminTranslationTestResult,
  type DeepLTier,
  type TranslationEngine,
  type WebTranslationProvider,
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
  buildProxyInput,
  createEmptyProxyDraft,
  FormRow,
  isProxyDraftEmpty,
  ProxyFields,
  proxyViewToDraft,
  SectionCard,
  SectionErrorState,
  SectionFooter,
  SectionHeader,
  SectionLoadingState,
  validateProxyDraft,
  type ProxyDraft,
  type ProxyValidationErrors,
} from '@/components/settings'

export function AdminTranslationSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const { data: policy, isLoading, error, refetch } = useQuery({
    queryKey: queryKeys.adminTranslation,
    queryFn: () => adminTranslationApi.get(),
  })

  const [webEnabled, setWebEnabled] = useState(false)
  const [provider, setProvider] = useState<WebTranslationProvider>('google_web')
  const [url, setUrl] = useState('')
  const [deeplTier, setDeeplTier] = useState<DeepLTier>('auto')
  const [apiKey, setApiKey] = useState('')
  const [replacingKey, setReplacingKey] = useState(false)

  const [aiEnabled, setAiEnabled] = useState(false)
  const [defaultEngine, setDefaultEngine] = useState<TranslationEngine>('web')

  const [proxyMode, setProxyMode] = useState<AdminTranslationProxyMode>('none')
  const [proxyDraft, setProxyDraft] = useState<ProxyDraft>(createEmptyProxyDraft)
  const [proxyTouched, setProxyTouched] = useState(false)
  const [proxyCleared, setProxyCleared] = useState(false)
  const [proxyErrors, setProxyErrors] = useState<ProxyValidationErrors>({})

  const [maxChars, setMaxChars] = useState('8000')
  const [dailyLimit, setDailyLimit] = useState('200')
  const [timeoutSec, setTimeoutSec] = useState('30')

  const [aiUserConfigEnabled, setAiUserConfigEnabled] = useState(true)
  const [aiAllowPrivateNetwork, setAiAllowPrivateNetwork] = useState(false)

  const [testResult, setTestResult] = useState<AdminTranslationTestResult | null>(null)

  useEffect(() => {
    if (!policy) return
    setWebEnabled(policy.web_enabled)
    setProvider(policy.provider)
    setUrl(policy.url ?? '')
    setDeeplTier(policy.deepl_tier ?? 'auto')
    setApiKey('')
    setReplacingKey(false)
    setAiEnabled(policy.ai_enabled)
    setDefaultEngine(policy.default_engine)
    setProxyMode(policy.proxy_mode)
    setProxyDraft(proxyViewToDraft(policy.proxy))
    setProxyTouched(false)
    setProxyCleared(false)
    setProxyErrors({})
    setMaxChars(String(policy.max_chars))
    setDailyLimit(String(policy.daily_limit_per_user))
    setTimeoutSec(String(policy.timeout))
    setAiUserConfigEnabled(policy.ai_user_config_enabled)
    setAiAllowPrivateNetwork(policy.ai_allow_private_network)
  }, [policy])

  const saveMutation = useMutation({
    mutationFn: (payload: AdminTranslationPolicyPatch) => adminTranslationApi.update(payload),
    onSuccess: (updated) => {
      queryClient.setQueryData(queryKeys.adminTranslation, updated)
      void queryClient.invalidateQueries({ queryKey: queryKeys.userAI })
      setApiKey('')
      setReplacingKey(false)
      setProxyTouched(false)
      setProxyCleared(false)
      setProxyErrors({})
      toast.success(t('admin.translation.saved'))
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const testMutation = useMutation({
    mutationFn: () => adminTranslationApi.test(),
    onSuccess: (res) => {
      setTestResult(res)
      toast.success(t('admin.translation.testSuccessToast', { latency: res.latency_ms }))
    },
    onError: (err) => {
      setTestResult(null)
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const handleSave = (e: React.FormEvent) => {
    e.preventDefault()
    const parsedMaxChars = Math.max(500, Math.min(50000, Number(maxChars) || 8000))
    const parsedDaily = Math.max(0, Number(dailyLimit) || 0)
    const parsedTimeout = Math.max(1, Math.min(120, Number(timeoutSec) || 30))

    const payload: AdminTranslationPolicyPatch = {
      web_enabled: webEnabled,
      provider,
      ...(provider === 'libretranslate' ? { url: url.trim() } : {}),
      ...(provider === 'deepl' ? { deepl_tier: deeplTier } : {}),
      ai_enabled: aiEnabled,
      default_engine: defaultEngine,
      proxy_mode: proxyMode,
      max_chars: parsedMaxChars,
      daily_limit_per_user: parsedDaily,
      timeout: parsedTimeout,
      ai_user_config_enabled: aiUserConfigEnabled,
      ai_allow_private_network: aiAllowPrivateNetwork,
    }

    if ((provider === 'deepl' || provider === 'libretranslate') && (!policy?.api_key_set || replacingKey)) {
      if (apiKey.trim()) {
        payload.api_key = apiKey.trim()
      }
    }

    if (proxyMode === 'custom') {
      if (proxyCleared || isProxyDraftEmpty(proxyDraft)) {
        payload.clear_proxy = true
      } else if (proxyTouched || !policy?.proxy) {
        const errs = validateProxyDraft(proxyDraft)
        if (errs.host || errs.port) {
          setProxyErrors(errs)
          return
        }
        payload.proxy = buildProxyInput(proxyDraft)
      }
    } else if (proxyCleared) {
      payload.clear_proxy = true
    }

    setProxyErrors({})
    saveMutation.mutate(payload)
  }

  const needsKey = provider === 'deepl' || provider === 'libretranslate'
  const isUnofficialWeb = provider === 'google_web' || provider === 'edge_web'
  const showKeyInput = needsKey && (!policy?.api_key_set || replacingKey)

  return (
    <form onSubmit={handleSave} className="space-y-5">
      {/* Web & AI Translation Engines Card */}
      <SectionCard>
        <SectionHeader
          icon={Languages}
          title={t('admin.translation.title')}
          description={t('admin.translation.desc')}
        />

        {error ? (
          <SectionErrorState
            message={error instanceof Error ? error.message : t('common.unknownError')}
            retryLabel={t('app.retry')}
            onRetry={() => void refetch()}
          />
        ) : isLoading ? (
          <SectionLoadingState rows={3} rowHeight="h-16" />
        ) : (
          <div className="space-y-4">
            {/* Web translation toggle & provider */}
            <div className="space-y-3 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-4">
              <FormRow
                layout="horizontal"
                label={t('admin.translation.webEnabledLabel')}
                description={t('admin.translation.webEnabledDesc')}
              >
                <Switch checked={webEnabled} onCheckedChange={setWebEnabled} />
              </FormRow>

              <div className="grid grid-cols-1 gap-3 border-t border-[var(--border-hairline)] pt-3 sm:grid-cols-2">
                <FormRow label={t('admin.translation.providerLabel')} htmlFor="admin_tr_provider">
                  <Select
                    value={provider}
                    onValueChange={(v) => setProvider(v as WebTranslationProvider)}
                  >
                    <SelectTrigger id="admin_tr_provider" className="h-9">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="google_web">
                        {t('translation.providers.google_web')}
                      </SelectItem>
                      <SelectItem value="edge_web">
                        {t('translation.providers.edge_web')}
                      </SelectItem>
                      <SelectItem value="deepl">{t('translation.providers.deepl')}</SelectItem>
                      <SelectItem value="libretranslate">
                        {t('translation.providers.libretranslate')}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                </FormRow>

                {provider === 'deepl' && (
                  <FormRow label={t('admin.translation.deeplTierLabel')} htmlFor="admin_tr_deepl_tier">
                    <Select value={deeplTier} onValueChange={(v) => setDeeplTier(v as DeepLTier)}>
                      <SelectTrigger id="admin_tr_deepl_tier" className="h-9">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="auto">{t('admin.translation.deeplTierAuto')}</SelectItem>
                        <SelectItem value="free">{t('admin.translation.deeplTierFree')}</SelectItem>
                        <SelectItem value="pro">{t('admin.translation.deeplTierPro')}</SelectItem>
                      </SelectContent>
                    </Select>
                  </FormRow>
                )}

                {provider === 'libretranslate' && (
                  <FormRow label={t('admin.translation.libreUrlLabel')} htmlFor="admin_tr_libre_url">
                    <Input
                      id="admin_tr_libre_url"
                      value={url}
                      onChange={(e) => setUrl(e.target.value)}
                      placeholder={t('admin.translation.libreUrlPlaceholder')}
                      className="font-mono text-[13px]"
                    />
                  </FormRow>
                )}
              </div>

              {isUnofficialWeb && (
                <div className="flex items-start gap-2 rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-subtle)]/70 px-3 py-2 text-[12px] text-[var(--ink-secondary)]">
                  <Info className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[var(--accent)]" />
                  <span>{t('admin.translation.unofficialNotice')}</span>
                </div>
              )}

              {needsKey && (
                <div className="space-y-2 border-t border-[var(--border-hairline)] pt-3">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="flex items-center gap-2">
                      <Label htmlFor="admin_tr_api_key">{t('admin.translation.apiKeyLabel')}</Label>
                      {policy?.api_key_set && !replacingKey && (
                        <Badge variant="success">{t('proxy.passwordSet')}</Badge>
                      )}
                    </div>
                    {policy?.api_key_set && !replacingKey && (
                      <Button
                        type="button"
                        variant="secondary"
                        size="xs"
                        onClick={() => setReplacingKey(true)}
                      >
                        <KeyRound className="h-3.5 w-3.5" />
                        {t('admin.translation.replaceKey')}
                      </Button>
                    )}
                  </div>
                  {showKeyInput ? (
                    <div className="flex items-center gap-2">
                      <Input
                        id="admin_tr_api_key"
                        type="password"
                        value={apiKey}
                        onChange={(e) => setApiKey(e.target.value)}
                        placeholder={t('admin.translation.apiKeyPlaceholder')}
                        autoComplete="new-password"
                      />
                      {replacingKey && (
                        <Button
                          type="button"
                          variant="ghost"
                          size="xs"
                          onClick={() => {
                            setReplacingKey(false)
                            setApiKey('')
                          }}
                        >
                          {t('common.cancel')}
                        </Button>
                      )}
                    </div>
                  ) : (
                    <p className="text-[12px] text-[var(--ink-muted)]">
                      {t('admin.translation.apiKeyKeepHint')}
                    </p>
                  )}
                </div>
              )}
            </div>

            {/* AI Translation Toggle */}
            <FormRow
              layout="horizontal"
              surface
              label={t('admin.translation.aiEnabledLabel')}
              description={t('admin.translation.aiEnabledDesc')}
            >
              <Switch checked={aiEnabled} onCheckedChange={setAiEnabled} />
            </FormRow>

            {/* Default Engine & Limits */}
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
              <FormRow
                surface
                label={t('admin.translation.defaultEngineLabel')}
                htmlFor="admin_tr_default_engine"
              >
                <Select
                  value={defaultEngine}
                  onValueChange={(v) => setDefaultEngine(v as TranslationEngine)}
                >
                  <SelectTrigger id="admin_tr_default_engine" className="h-9">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="web">{t('translation.engines.web')}</SelectItem>
                    <SelectItem value="ai">{t('translation.engines.ai')}</SelectItem>
                  </SelectContent>
                </Select>
              </FormRow>

              <FormRow
                surface
                label={t('admin.translation.maxCharsLabel')}
                htmlFor="admin_tr_max_chars"
              >
                <Input
                  id="admin_tr_max_chars"
                  type="number"
                  min={500}
                  max={50000}
                  value={maxChars}
                  onChange={(e) => setMaxChars(e.target.value)}
                  className="font-mono tabular-nums"
                />
              </FormRow>

              <FormRow
                surface
                label={t('admin.translation.dailyLimitLabel')}
                htmlFor="admin_tr_daily_limit"
                description={t('admin.translation.dailyLimitHint')}
              >
                <Input
                  id="admin_tr_daily_limit"
                  type="number"
                  min={0}
                  value={dailyLimit}
                  onChange={(e) => setDailyLimit(e.target.value)}
                  className="font-mono tabular-nums"
                />
              </FormRow>

              <FormRow
                surface
                label={t('admin.translation.timeoutLabel')}
                htmlFor="admin_tr_timeout"
              >
                <Input
                  id="admin_tr_timeout"
                  type="number"
                  min={1}
                  max={120}
                  value={timeoutSec}
                  onChange={(e) => setTimeoutSec(e.target.value)}
                  className="font-mono tabular-nums"
                />
              </FormRow>
            </div>

            {/* Proxy Mode */}
            <div className="space-y-3">
              <FormRow
                surface
                label={t('admin.translation.proxyModeLabel')}
                htmlFor="admin_tr_proxy_mode"
                description={t('admin.translation.proxyModeDesc')}
              >
                <Select
                  value={proxyMode}
                  onValueChange={(v) => {
                    setProxyMode(v as AdminTranslationProxyMode)
                    setProxyErrors({})
                  }}
                >
                  <SelectTrigger id="admin_tr_proxy_mode" className="h-9 sm:max-w-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">{t('proxy.modeNone')}</SelectItem>
                    <SelectItem value="telegram">{t('admin.translation.proxyModeTelegram')}</SelectItem>
                    <SelectItem value="custom">{t('proxy.modeCustom')}</SelectItem>
                  </SelectContent>
                </Select>
              </FormRow>

              {proxyMode === 'custom' && (
                <ProxyFields
                  idPrefix="tr_proxy"
                  value={proxyDraft}
                  onChange={(next) => {
                    setProxyDraft(next)
                    setProxyTouched(true)
                    setProxyCleared(false)
                    setProxyErrors({})
                  }}
                  errors={proxyErrors}
                  canClearProxy={Boolean(policy?.proxy) || !isProxyDraftEmpty(proxyDraft)}
                  onClearProxy={() => {
                    setProxyDraft(createEmptyProxyDraft())
                    setProxyTouched(true)
                    setProxyCleared(true)
                    setProxyErrors({})
                  }}
                />
              )}
            </div>

            {/* Test Result */}
            {testResult && (
              <div className="space-y-1.5 rounded-[var(--radius-md)] border border-[var(--success)]/35 bg-[var(--success-soft)]/70 p-3.5 text-[12.5px] text-[var(--ink-primary)]">
                <div className="flex flex-wrap items-center gap-2 text-[var(--success)]">
                  <CheckCircle2 className="h-4 w-4 shrink-0" />
                  <span className="font-medium">
                    {t('admin.translation.testResultHeader', {
                      provider: t(`translation.providers.${testResult.provider}`),
                      latency: testResult.latency_ms,
                    })}
                  </span>
                </div>
                <p className="whitespace-pre-wrap text-[12.5px] text-[var(--ink-secondary)]">
                  {testResult.text}
                </p>
              </div>
            )}
          </div>
        )}
      </SectionCard>

      {/* User AI Access Policy Card */}
      <SectionCard>
        <SectionHeader
          icon={Sparkles}
          iconTone="neutral"
          title={t('admin.translation.aiPolicyTitle')}
          description={t('admin.translation.aiPolicyDesc')}
        />

        {!isLoading && !error && (
          <div className="space-y-4">
            <FormRow
              layout="horizontal"
              surface
              label={t('admin.translation.allowUserAILabel')}
              description={t('admin.translation.allowUserAIDesc')}
            >
              <Switch
                checked={aiUserConfigEnabled}
                onCheckedChange={setAiUserConfigEnabled}
              />
            </FormRow>

            <FormRow
              layout="horizontal"
              surface
              label={t('admin.translation.allowPrivateNetLabel')}
              description={t('admin.translation.allowPrivateNetDesc')}
            >
              <Switch
                checked={aiAllowPrivateNetwork}
                onCheckedChange={setAiAllowPrivateNetwork}
              />
            </FormRow>

            {aiAllowPrivateNetwork && (
              <div
                role="alert"
                className="flex items-start gap-2.5 rounded-[var(--radius-md)] border border-[var(--warning)]/40 bg-[var(--warning-soft)] px-3.5 py-2.5 text-[12.5px] text-[var(--warning)]"
              >
                <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
                <span>{t('admin.translation.privateNetWarning')}</span>
              </div>
            )}

            <SectionFooter align="between">
              <Button
                type="button"
                variant="secondary"
                size="sm"
                disabled={testMutation.isPending}
                onClick={() => testMutation.mutate()}
              >
                {testMutation.isPending ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <PlugZap className="h-3.5 w-3.5" />
                )}
                {testMutation.isPending
                  ? t('admin.translation.testing')
                  : t('admin.translation.testButton')}
              </Button>

              <Button type="submit" size="sm" disabled={saveMutation.isPending}>
                {saveMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                {t('common.save')}
              </Button>
            </SectionFooter>
          </div>
        )}
      </SectionCard>
    </form>
  )
}
