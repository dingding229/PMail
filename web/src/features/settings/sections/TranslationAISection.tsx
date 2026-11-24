import { useEffect, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  CheckCircle2,
  Info,
  KeyRound,
  Languages,
  Loader2,
  Lock,
  PlugZap,
  Sparkles,
  Trash2,
} from 'lucide-react'
import {
  userAIApi,
  userTranslationApi,
  type TranslationEngine,
  type TranslationLanguage,
  type UserAIConfigInput,
  type UserAIProxyMode,
  type UserAITestResult,
  type UserTranslationPreferences,
} from '@/api'
import {
  queryKeys,
  useUserAIQuery,
  useUserTranslationPrefsQuery,
} from '@/hooks/useMailQueries'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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

const LANGUAGE_OPTIONS: TranslationLanguage[] = [
  'zh-CN',
  'zh-TW',
  'en',
  'ja',
  'ko',
  'fr',
  'de',
  'es',
  'ru',
]

export function TranslationAISection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const prefsQuery = useUserTranslationPrefsQuery(true)
  const aiQuery = useUserAIQuery(true)

  // Preferences form state
  const [targetLang, setTargetLang] = useState<TranslationLanguage | ''>('')
  const [translateEngine, setTranslateEngine] = useState<TranslationEngine | ''>('')
  const [autoTranslate, setAutoTranslate] = useState(false)
  const [aiAutoSummary, setAiAutoSummary] = useState(false)

  // Personal AI config state
  const [baseUrl, setBaseUrl] = useState('')
  const [apiKey, setApiKey] = useState('')
  const [replacingKey, setReplacingKey] = useState(false)
  const [model, setModel] = useState('')
  const [timeoutSec, setTimeoutSec] = useState('30')
  const [proxyMode, setProxyMode] = useState<UserAIProxyMode>('none')
  const [proxyDraft, setProxyDraft] = useState<ProxyDraft>(createEmptyProxyDraft)
  const [proxyTouched, setProxyTouched] = useState(false)
  const [proxyCleared, setProxyCleared] = useState(false)
  const [proxyErrors, setProxyErrors] = useState<ProxyValidationErrors>({})
  const [aiFormError, setAiFormError] = useState<string | null>(null)
  const [testResult, setTestResult] = useState<UserAITestResult | null>(null)
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false)

  useEffect(() => {
    if (!prefsQuery.data) return
    setTargetLang(prefsQuery.data.target_lang ?? '')
    setTranslateEngine(prefsQuery.data.translate_engine ?? '')
    setAutoTranslate(Boolean(prefsQuery.data.auto_translate))
    setAiAutoSummary(Boolean(prefsQuery.data.ai_auto_summary))
  }, [prefsQuery.data])

  useEffect(() => {
    if (!aiQuery.data) return
    setBaseUrl(aiQuery.data.base_url ?? '')
    setModel(aiQuery.data.model ?? '')
    setTimeoutSec(String(aiQuery.data.timeout ?? 30))
    setProxyMode(aiQuery.data.proxy_mode ?? 'none')
    setProxyDraft(proxyViewToDraft(aiQuery.data.proxy))
    setApiKey('')
    setReplacingKey(false)
    setProxyTouched(false)
    setProxyCleared(false)
    setProxyErrors({})
    setAiFormError(null)
  }, [aiQuery.data])

  const savePrefsMutation = useMutation({
    mutationFn: (payload: UserTranslationPreferences) => userTranslationApi.update(payload),
    onSuccess: (updated) => {
      queryClient.setQueryData(queryKeys.userTranslation, updated)
      toast.success(t('settings.translation.prefsSaved'))
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const saveAIMutation = useMutation({
    mutationFn: (payload: UserAIConfigInput) => userAIApi.update(payload),
    onSuccess: (updated) => {
      queryClient.setQueryData(queryKeys.userAI, updated)
      void queryClient.invalidateQueries({ queryKey: ['emails', 'detail'] })
      setApiKey('')
      setReplacingKey(false)
      setProxyTouched(false)
      setProxyCleared(false)
      setProxyErrors({})
      setAiFormError(null)
      toast.success(t('settings.translation.aiSaved'))
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const deleteAIMutation = useMutation({
    mutationFn: () => userAIApi.remove(),
    onSuccess: () => {
      setDeleteConfirmOpen(false)
      setTestResult(null)
      void queryClient.invalidateQueries({ queryKey: queryKeys.userAI })
      void queryClient.invalidateQueries({ queryKey: ['emails', 'detail'] })
      toast.success(t('settings.translation.aiDeleted'))
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const testAIMutation = useMutation({
    mutationFn: () => userAIApi.test(),
    onSuccess: (res) => {
      setTestResult(res)
      toast.success(
        t('settings.translation.aiTestSuccess', {
          model: res.model,
          latency: res.latency_ms,
        }),
      )
    },
    onError: (err) => {
      setTestResult(null)
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    },
  })

  const handleSavePrefs = (e: React.FormEvent) => {
    e.preventDefault()
    savePrefsMutation.mutate({
      target_lang: targetLang,
      translate_engine: translateEngine,
      auto_translate: autoTranslate,
      ai_auto_summary: aiAutoSummary,
    })
  }

  const handleSaveAI = (e: React.FormEvent) => {
    e.preventDefault()
    if (!baseUrl.trim() || !model.trim()) {
      setAiFormError(t('settings.translation.aiRequiredFields'))
      return
    }
    const payload: UserAIConfigInput = {
      base_url: baseUrl.trim(),
      model: model.trim(),
      timeout: Math.max(1, Math.min(120, Number(timeoutSec) || 30)),
      proxy_mode: proxyMode,
    }
    if (!aiQuery.data?.api_key_set || replacingKey) {
      if (apiKey.trim()) {
        payload.api_key = apiKey.trim()
      }
    }
    if (proxyMode === 'custom') {
      if (proxyCleared || isProxyDraftEmpty(proxyDraft)) {
        payload.clear_proxy = true
      } else if (proxyTouched || !aiQuery.data?.proxy) {
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
    setAiFormError(null)
    setProxyErrors({})
    saveAIMutation.mutate(payload)
  }

  const aiDisabledByAdmin = aiQuery.data ? !aiQuery.data.ai_user_config_enabled : false
  const showApiKeyInput = !aiQuery.data?.api_key_set || replacingKey

  return (
    <div className="space-y-5">
      {/* Card 1: Translation Preferences */}
      <SectionCard>
        <SectionHeader
          icon={Languages}
          title={t('settings.translation.prefsTitle')}
          description={t('settings.translation.prefsDesc')}
        />

        {prefsQuery.error ? (
          <SectionErrorState
            message={
              prefsQuery.error instanceof Error
                ? prefsQuery.error.message
                : t('common.unknownError')
            }
            retryLabel={t('app.retry')}
            onRetry={() => void prefsQuery.refetch()}
          />
        ) : prefsQuery.isLoading ? (
          <SectionLoadingState rows={3} rowHeight="h-14" />
        ) : (
          <form onSubmit={handleSavePrefs} className="space-y-4">
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <FormRow
                surface
                label={t('settings.translation.targetLangLabel')}
                htmlFor="user_tr_target_lang"
              >
                <Select
                  value={targetLang || '__ui__'}
                  onValueChange={(v) =>
                    setTargetLang(v === '__ui__' ? '' : (v as TranslationLanguage))
                  }
                >
                  <SelectTrigger id="user_tr_target_lang" className="h-9">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__ui__">
                      {t('settings.translation.followUiLang')}
                    </SelectItem>
                    {LANGUAGE_OPTIONS.map((lang) => (
                      <SelectItem key={lang} value={lang}>
                        {t(`translation.languages.${lang}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </FormRow>

              <FormRow
                surface
                label={t('settings.translation.defaultEngineLabel')}
                htmlFor="user_tr_engine"
              >
                <Select
                  value={translateEngine || '__default__'}
                  onValueChange={(v) =>
                    setTranslateEngine(v === '__default__' ? '' : (v as TranslationEngine))
                  }
                >
                  <SelectTrigger id="user_tr_engine" className="h-9">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__default__">
                      {t('settings.translation.followSystemDefault')}
                    </SelectItem>
                    <SelectItem value="web">{t('translation.engines.web')}</SelectItem>
                    <SelectItem value="ai">{t('translation.engines.ai')}</SelectItem>
                  </SelectContent>
                </Select>
              </FormRow>
            </div>

            <FormRow
              layout="horizontal"
              surface
              label={t('settings.translation.autoTranslateLabel')}
              description={t('settings.translation.autoTranslateDesc')}
            >
              <Switch checked={autoTranslate} onCheckedChange={setAutoTranslate} />
            </FormRow>

            <FormRow
              layout="horizontal"
              surface
              label={t('settings.translation.aiAutoSummaryLabel')}
              description={t('settings.translation.aiAutoSummaryDesc')}
            >
              <Switch checked={aiAutoSummary} onCheckedChange={setAiAutoSummary} />
            </FormRow>

            <SectionFooter>
              <Button type="submit" size="sm" disabled={savePrefsMutation.isPending}>
                {savePrefsMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                {t('common.save')}
              </Button>
            </SectionFooter>
          </form>
        )}
      </SectionCard>

      {/* Card 2: Personal AI Interface */}
      <SectionCard>
        <SectionHeader
          icon={Sparkles}
          iconTone={aiDisabledByAdmin ? 'neutral' : 'accent'}
          title={t('settings.translation.aiTitle')}
          description={t('settings.translation.aiDesc')}
          badge={
            aiQuery.data ? (
              aiDisabledByAdmin ? (
                <Badge variant="default">
                  <Lock className="h-3 w-3" />
                  {t('settings.translation.aiDisabledBadge')}
                </Badge>
              ) : aiQuery.data.configured ? (
                <Badge variant="success">{t('settings.translation.aiConfiguredBadge')}</Badge>
              ) : (
                <Badge variant="default">{t('settings.translation.aiNotConfiguredBadge')}</Badge>
              )
            ) : undefined
          }
          action={
            aiQuery.data?.configured && !aiDisabledByAdmin ? (
              <Button
                type="button"
                variant="dangerGhost"
                size="sm"
                onClick={() => setDeleteConfirmOpen(true)}
              >
                <Trash2 className="h-3.5 w-3.5" />
                {t('settings.translation.aiDeleteButton')}
              </Button>
            ) : undefined
          }
        />

        {aiQuery.error ? (
          <SectionErrorState
            message={
              aiQuery.error instanceof Error ? aiQuery.error.message : t('common.unknownError')
            }
            retryLabel={t('app.retry')}
            onRetry={() => void aiQuery.refetch()}
          />
        ) : aiQuery.isLoading ? (
          <SectionLoadingState rows={3} rowHeight="h-14" />
        ) : (
          <form onSubmit={handleSaveAI} className="space-y-4">
            {aiDisabledByAdmin && (
              <div className="flex items-start gap-2.5 rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-subtle)] px-3.5 py-3 text-[12.5px] text-[var(--ink-secondary)]">
                <Lock className="mt-0.5 h-4 w-4 shrink-0 text-[var(--ink-muted)]" />
                <span>{t('settings.translation.aiDisabledByAdminNotice')}</span>
              </div>
            )}

            <div className="flex items-start gap-2.5 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] px-3.5 py-2.5 text-[12px] text-[var(--ink-secondary)]">
              <Info className="mt-0.5 h-4 w-4 shrink-0 text-[var(--accent)]" />
              <span>{t('settings.translation.aiPrivacyHint')}</span>
            </div>

            {aiFormError && <SectionErrorState message={aiFormError} />}

            <fieldset disabled={aiDisabledByAdmin} className="space-y-4 disabled:opacity-60">
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <FormRow
                  surface
                  label={t('settings.translation.aiBaseUrlLabel')}
                  htmlFor="user_ai_base_url"
                >
                  <Input
                    id="user_ai_base_url"
                    value={baseUrl}
                    onChange={(e) => setBaseUrl(e.target.value)}
                    placeholder={t('settings.translation.aiBaseUrlPlaceholder')}
                    className="font-mono text-[13px]"
                  />
                </FormRow>

                <FormRow
                  surface
                  label={t('settings.translation.aiModelLabel')}
                  htmlFor="user_ai_model"
                >
                  <Input
                    id="user_ai_model"
                    value={model}
                    onChange={(e) => setModel(e.target.value)}
                    placeholder={t('settings.translation.aiModelPlaceholder')}
                    className="font-mono text-[13px]"
                  />
                </FormRow>
              </div>

              <div className="grid grid-cols-1 gap-4 sm:grid-cols-12">
                {/* API Key */}
                <div className="space-y-2 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-4 sm:col-span-8">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="flex items-center gap-2">
                      <Label htmlFor="user_ai_api_key">
                        {t('settings.translation.aiApiKeyLabel')}
                      </Label>
                      {aiQuery.data?.api_key_set && !replacingKey && (
                        <Badge variant="success">{t('proxy.passwordSet')}</Badge>
                      )}
                    </div>
                    {aiQuery.data?.api_key_set && !replacingKey && !aiDisabledByAdmin && (
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
                  {showApiKeyInput ? (
                    <div className="flex items-center gap-2">
                      <Input
                        id="user_ai_api_key"
                        type="password"
                        value={apiKey}
                        onChange={(e) => setApiKey(e.target.value)}
                        placeholder={t('settings.translation.aiApiKeyPlaceholder')}
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
                      {t('settings.translation.aiApiKeyKeepHint')}
                    </p>
                  )}
                </div>

                {/* Timeout */}
                <FormRow
                  surface
                  label={t('admin.translation.timeoutLabel')}
                  htmlFor="user_ai_timeout"
                  className="sm:col-span-4"
                >
                  <Input
                    id="user_ai_timeout"
                    type="number"
                    min={1}
                    max={120}
                    value={timeoutSec}
                    onChange={(e) => setTimeoutSec(e.target.value)}
                    className="font-mono tabular-nums"
                  />
                </FormRow>
              </div>

              {/* Proxy Mode 3-way */}
              <FormRow
                surface
                label={t('settings.translation.aiProxyModeLabel')}
                htmlFor="user_ai_proxy_mode"
                description={t('settings.translation.aiProxyModeDesc')}
              >
                <Select
                  value={proxyMode}
                  disabled={aiDisabledByAdmin}
                  onValueChange={(v) => {
                    setProxyMode(v as UserAIProxyMode)
                    setProxyErrors({})
                  }}
                >
                  <SelectTrigger id="user_ai_proxy_mode" className="h-9 sm:max-w-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">{t('proxy.modeNone')}</SelectItem>
                    <SelectItem value="system">{t('proxy.modeSystem')}</SelectItem>
                    <SelectItem value="custom">{t('proxy.modeCustom')}</SelectItem>
                  </SelectContent>
                </Select>
              </FormRow>

              {proxyMode === 'custom' && (
                <ProxyFields
                  idPrefix="ai_proxy"
                  value={proxyDraft}
                  onChange={(next) => {
                    setProxyDraft(next)
                    setProxyTouched(true)
                    setProxyCleared(false)
                    setProxyErrors({})
                  }}
                  errors={proxyErrors}
                  disabled={aiDisabledByAdmin}
                  canClearProxy={Boolean(aiQuery.data?.proxy) || !isProxyDraftEmpty(proxyDraft)}
                  onClearProxy={() => {
                    setProxyDraft(createEmptyProxyDraft())
                    setProxyTouched(true)
                    setProxyCleared(true)
                    setProxyErrors({})
                  }}
                />
              )}
            </fieldset>

            {testResult && (
              <div className="flex items-center gap-2 rounded-[var(--radius-sm)] border border-[var(--success)]/35 bg-[var(--success-soft)] px-3 py-2 text-[12.5px] text-[var(--success)]">
                <CheckCircle2 className="h-4 w-4 shrink-0" />
                <span>
                  {t('settings.translation.aiTestSuccess', {
                    model: testResult.model,
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
                disabled={
                  aiDisabledByAdmin || !aiQuery.data?.configured || testAIMutation.isPending
                }
                onClick={() => testAIMutation.mutate()}
              >
                {testAIMutation.isPending ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <PlugZap className="h-3.5 w-3.5" />
                )}
                {testAIMutation.isPending
                  ? t('settings.translation.aiTesting')
                  : t('settings.translation.aiTestButton')}
              </Button>

              <Button
                type="submit"
                size="sm"
                disabled={aiDisabledByAdmin || saveAIMutation.isPending}
              >
                {saveAIMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                {t('common.save')}
              </Button>
            </SectionFooter>
          </form>
        )}
      </SectionCard>

      {/* Delete AI Config Confirm Dialog */}
      <Dialog open={deleteConfirmOpen} onOpenChange={setDeleteConfirmOpen}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>{t('settings.translation.aiDeleteConfirmTitle')}</DialogTitle>
            <DialogDescription>
              {t('settings.translation.aiDeleteConfirmDesc')}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setDeleteConfirmOpen(false)}
            >
              {t('common.cancel')}
            </Button>
            <Button
              type="button"
              variant="danger"
              size="sm"
              disabled={deleteAIMutation.isPending}
              onClick={() => deleteAIMutation.mutate()}
            >
              {deleteAIMutation.isPending && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
              {t('settings.translation.aiDeleteButton')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
