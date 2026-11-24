import { useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  AlertTriangle,
  Check,
  Copy,
  Download,
  Info,
  KeyRound,
  Loader2,
  Plus,
  RefreshCw,
  ShieldCheck,
  ShieldOff,
  Trash2,
} from 'lucide-react'
import { securityApi } from '@/api/security'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  EmptyState,
  FormRow,
  SectionCard,
  SectionErrorState,
  SectionHeader,
  SectionLoadingState,
} from '@/components/settings'
import { SecondFactorField } from '@/features/auth/SecondFactorField'
import { queryKeys } from '@/hooks/useMailQueries'
import { formatMailFullTime } from '@/lib/date'

type FactorForm = 'enable' | 'disable' | 'regenerate'

export function AccountCredentials({ userID }: { userID: number }) {
  const { t, i18n } = useTranslation()
  const client = useQueryClient()
  const factorKey = ['second-factor', userID]
  const appKey = ['app-passwords', userID]
  const factor = useQuery({ queryKey: factorKey, queryFn: securityApi.status })
  const apps = useQuery({ queryKey: appKey, queryFn: securityApi.listAppPasswords })

  const [form, setForm] = useState<FactorForm | null>(null)
  const [setup, setSetup] = useState<{ secret: string; qr: string | null } | null>(null)
  const [code, setCode] = useState('')
  const [password, setPassword] = useState('')
  const [recovery, setRecovery] = useState(false)
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([])
  const [factorBusy, setFactorBusy] = useState(false)
  const [factorError, setFactorError] = useState('')

  const [appName, setAppName] = useState('')
  const [newApp, setNewApp] = useState('')
  const [appBusy, setAppBusy] = useState(false)
  const [appError, setAppError] = useState('')

  const [copiedKey, setCopiedKey] = useState(false)
  const [copiedCodes, setCopiedCodes] = useState(false)
  const [copiedApp, setCopiedApp] = useState(false)

  const message = (err: unknown) => err instanceof Error ? err.message : t('common.unknownError')
  const choose = (next: FactorForm | null) => {
    setForm(next)
    setCode('')
    setPassword('')
    setRecovery(false)
    setFactorError('')
    setSetup(null)
  }

  const copyText = async (
    text: string,
    onCopied: (v: boolean) => void,
    toastMsg: string,
  ) => {
    try {
      await navigator.clipboard.writeText(text)
      onCopied(true)
      toast.success(toastMsg)
      window.setTimeout(() => onCopied(false), 1800)
    } catch {
      toast.error(t('common.copyFailed'))
    }
  }

  const startSetup = async () => {
    if (factorBusy) return
    choose(null)
    setFactorBusy(true)
    try {
      const { secret, otpauth_uri } = await securityApi.setup()
      // Render locally; the URI (and its secret) never leaves the browser.
      const qr = await import('uqr').then(({ renderSVG }) =>
        `data:image/svg+xml,${encodeURIComponent(renderSVG(otpauth_uri, { border: 4 }))}`,
      ).catch(() => null)
      setSetup({ secret, qr })
      setForm('enable')
    } catch (err) {
      setFactorError(message(err))
    } finally {
      setFactorBusy(false)
    }
  }

  const submitFactor = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!form || factorBusy) return
    setFactorBusy(true)
    setFactorError('')
    try {
      const confirm = { password, code: code.trim(), recovery }
      const result =
        form === 'enable'
          ? await securityApi.enable(confirm.code)
          : form === 'disable'
            ? await securityApi.disable(confirm)
            : await securityApi.regenerate(confirm)
      await client.cancelQueries({ queryKey: factorKey })
      client.setQueryData(factorKey, { enabled: result.enabled })
      void client.invalidateQueries({ queryKey: queryKeys.userMe })
      setRecoveryCodes('recovery_codes' in result ? result.recovery_codes : [])
      choose(null)
    } catch (err) {
      setCode('')
      setPassword('')
      setFactorError(message(err))
    } finally {
      setFactorBusy(false)
    }
  }

  const downloadCodes = () => {
    const blob = new Blob([recoveryCodes.join('\n') + '\n'], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = 'pmail-recovery-codes.txt'
    link.click()
    window.setTimeout(() => URL.revokeObjectURL(url), 1000)
  }

  const createApp = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (appBusy || !appName.trim()) return
    setAppBusy(true)
    setAppError('')
    setNewApp('')
    try {
      const result = await securityApi.createAppPassword(appName.trim())
      setNewApp(result.password)
      setAppName('')
      void client.invalidateQueries({ queryKey: appKey })
    } catch (err) {
      setAppError(message(err))
    } finally {
      setAppBusy(false)
    }
  }

  const removeApp = async (id: number, name: string) => {
    if (appBusy || !window.confirm(t('security.removeAppConfirm', { name }))) return
    setAppBusy(true)
    setAppError('')
    setNewApp('')
    try {
      await securityApi.deleteAppPassword(id)
      void client.invalidateQueries({ queryKey: appKey })
    } catch (err) {
      setAppError(message(err))
    } finally {
      setAppBusy(false)
    }
  }

  const isEnabled = Boolean(factor.data?.enabled)

  return (
    <>
      {/* Two-Step Verification Card */}
      <SectionCard aria-labelledby="factor-title">
        <SectionHeader
          icon={isEnabled ? ShieldCheck : ShieldOff}
          iconTone={isEnabled ? 'success' : 'accent'}
          title={t('security.title')}
          titleId="factor-title"
          description={t('security.desc')}
          badge={
            factor.data ? (
              <Badge variant={isEnabled ? 'success' : 'default'} role="status">
                {isEnabled ? t('security.enabled') : t('security.disabled')}
              </Badge>
            ) : undefined
          }
          action={
            factor.data && !form ? (
              isEnabled ? (
                <>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => choose('regenerate')}
                  >
                    <RefreshCw className="h-3.5 w-3.5" />
                    {t('security.regenerate')}
                  </Button>
                  <Button
                    type="button"
                    variant="dangerGhost"
                    size="sm"
                    onClick={() => choose('disable')}
                  >
                    {t('security.disable')}
                  </Button>
                </>
              ) : (
                <Button
                  type="button"
                  size="sm"
                  disabled={factorBusy}
                  onClick={() => void startSetup()}
                >
                  {factorBusy && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                  {t('security.startSetup')}
                </Button>
              )
            ) : undefined
          }
        />

        {/* IMAP/SMTP Client Notice */}
        <div className="flex items-start gap-2.5 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] px-3.5 py-2.5 text-[12.5px] text-[var(--ink-secondary)]">
          <Info className="mt-0.5 h-4 w-4 shrink-0 text-[var(--accent)]" />
          <span>{t('security.imapSmtpNote')}</span>
        </div>

        {factor.isPending && <SectionLoadingState rows={1} rowHeight="h-16" />}
        {factor.error && <SectionErrorState message={factor.error.message} />}
        {factorError && <SectionErrorState message={factorError} />}

        {/* Setup / Disable / Regenerate Form */}
        {form && (
          <form
            onSubmit={submitFactor}
            className="space-y-4 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-4"
          >
            {form === 'enable' && setup && (
              <div className="space-y-4">
                <p className="text-[13px] leading-relaxed text-[var(--ink-secondary)]">
                  {t('security.scanHint')}
                </p>
                <div className="flex flex-col gap-5 sm:flex-row sm:items-start">
                  {setup.qr && (
                    <div className="flex shrink-0 flex-col items-center justify-center rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-white p-3 shadow-[var(--shadow-xs)] self-start">
                      <img
                        className="block rounded"
                        src={setup.qr}
                        width={176}
                        height={176}
                        alt={t('security.qrAlt')}
                      />
                    </div>
                  )}
                  <div className="min-w-0 flex-1 space-y-3">
                    <div className="space-y-1.5">
                      <Label htmlFor="totp-manual-key">{t('security.manualKey')}</Label>
                      <div className="flex items-center gap-2">
                        <Input
                          id="totp-manual-key"
                          value={setup.secret}
                          readOnly
                          className="font-mono text-[13px] tracking-wider"
                          autoComplete="off"
                        />
                        <Button
                          type="button"
                          variant="secondary"
                          size="sm"
                          className="h-9 shrink-0"
                          onClick={() =>
                            void copyText(setup.secret, setCopiedKey, t('common.copied'))
                          }
                        >
                          {copiedKey ? (
                            <Check className="h-3.5 w-3.5 text-[var(--success)]" />
                          ) : (
                            <Copy className="h-3.5 w-3.5" />
                          )}
                          {t('common.copy')}
                        </Button>
                      </div>
                    </div>

                    <SecondFactorField
                      autoFocus
                      code={code}
                      recovery={recovery}
                      onCode={setCode}
                      disabled={factorBusy}
                    />
                  </div>
                </div>
              </div>
            )}

            {form !== 'enable' && (
              <div className="max-w-md space-y-4">
                <p className="text-[13px] leading-relaxed text-[var(--ink-secondary)]">
                  {form === 'disable' ? t('security.disableHint') : t('security.regenerateHint')}
                </p>
                <FormRow
                  label={t('settings.profile.oldPassword')}
                  htmlFor="factor-current-password"
                >
                  <Input
                    id="factor-current-password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    type="password"
                    autoComplete="current-password"
                    required
                    disabled={factorBusy}
                  />
                </FormRow>
                <SecondFactorField
                  code={code}
                  recovery={recovery}
                  onCode={setCode}
                  onRecovery={setRecovery}
                  disabled={factorBusy}
                />
              </div>
            )}

            <div className="flex flex-wrap items-center gap-2 border-t border-[var(--border-hairline)] pt-3">
              <Button
                type="submit"
                size="sm"
                variant={form === 'disable' ? 'danger' : 'primary'}
                disabled={factorBusy}
              >
                {factorBusy && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
                {form === 'enable'
                  ? t('security.enable')
                  : form === 'disable'
                    ? t('security.disable')
                    : t('security.regenerate')}
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                disabled={factorBusy}
                onClick={() => choose(null)}
              >
                {t('common.cancel')}
              </Button>
            </div>
          </form>
        )}

        {/* Recovery Codes Grid (Shown Once) */}
        {recoveryCodes.length > 0 && (
          <div className="space-y-4 rounded-[var(--radius-md)] border border-[var(--warning)]/40 bg-[var(--warning-soft)]/55 p-4">
            <div className="flex items-start gap-2.5">
              <AlertTriangle className="mt-0.5 h-4.5 w-4.5 shrink-0 text-[var(--warning)]" />
              <div className="min-w-0 space-y-0.5">
                <h3 className="text-[14px] font-semibold text-[var(--ink-primary)]">
                  {t('security.recoveryCodes')}
                </h3>
                <p className="text-[12.5px] leading-relaxed text-[var(--ink-secondary)]">
                  {t('security.recoveryWarning')}
                </p>
              </div>
            </div>

            <ul className="grid grid-cols-1 gap-2 sm:grid-cols-2">
              {recoveryCodes.map((item, idx) => (
                <li
                  key={item}
                  className="flex items-center justify-between gap-2 rounded-[var(--radius-sm)] border border-[var(--border-subtle)] bg-[var(--surface-canvas)] px-3 py-2 font-mono text-[13px] tracking-wider text-[var(--ink-primary)]"
                >
                  <span className="text-[11px] tabular-nums text-[var(--ink-muted)] select-none">
                    {String(idx + 1).padStart(2, '0')}
                  </span>
                  <span className="select-all font-semibold">{item}</span>
                </li>
              ))}
            </ul>

            <div className="flex flex-wrap items-center gap-2 pt-1">
              <Button
                type="button"
                variant="secondary"
                size="sm"
                onClick={() =>
                  void copyText(
                    recoveryCodes.join('\n'),
                    setCopiedCodes,
                    t('security.allCodesCopied'),
                  )
                }
              >
                {copiedCodes ? (
                  <Check className="h-3.5 w-3.5 text-[var(--success)]" />
                ) : (
                  <Copy className="h-3.5 w-3.5" />
                )}
                {t('security.copyAllCodes')}
              </Button>
              <Button type="button" variant="outline" size="sm" onClick={downloadCodes}>
                <Download className="h-3.5 w-3.5" />
                {t('security.downloadCodes')}
              </Button>
              <Button
                type="button"
                size="sm"
                className="ml-auto"
                onClick={() => setRecoveryCodes([])}
              >
                {t('security.done')}
              </Button>
            </div>
          </div>
        )}
      </SectionCard>

      {/* App Passwords Card */}
      <SectionCard aria-labelledby="apps-title">
        <SectionHeader
          icon={KeyRound}
          iconTone="neutral"
          title={t('security.appPasswords')}
          titleId="apps-title"
          description={t('security.clientHint')}
        />

        {apps.error && <SectionErrorState message={apps.error.message} />}
        {appError && <SectionErrorState message={appError} />}

        {/* One-time reveal box for newly generated app password */}
        {newApp && (
          <div className="space-y-3 rounded-[var(--radius-md)] border border-[var(--warning)]/45 bg-[var(--warning-soft)]/60 p-4">
            <div className="flex items-start gap-2.5">
              <AlertTriangle className="mt-0.5 h-4.5 w-4.5 shrink-0 text-[var(--warning)]" />
              <div className="min-w-0 space-y-0.5">
                <p className="text-[13.5px] font-semibold text-[var(--ink-primary)]">
                  {t('security.oneTimeTitle')}
                </p>
                <Label
                  htmlFor="app-password-new"
                  className="text-[12.5px] font-normal text-[var(--ink-secondary)]"
                >
                  {t('security.newAppWarning')}
                </Label>
              </div>
            </div>

            <div className="flex flex-wrap items-center gap-2">
              <Input
                id="app-password-new"
                value={newApp}
                readOnly
                autoComplete="off"
                className="min-w-0 flex-1 bg-[var(--surface-canvas)] font-mono text-[14px] font-semibold tracking-wider"
              />
              <Button
                type="button"
                variant="secondary"
                size="sm"
                className="h-9 shrink-0"
                onClick={() => void copyText(newApp, setCopiedApp, t('common.copied'))}
              >
                {copiedApp ? (
                  <Check className="h-3.5 w-3.5 text-[var(--success)]" />
                ) : (
                  <Copy className="h-3.5 w-3.5" />
                )}
                {t('common.copy')}
              </Button>
              <Button
                type="button"
                size="sm"
                className="h-9 shrink-0"
                onClick={() => setNewApp('')}
              >
                {t('security.done')}
              </Button>
            </div>
          </div>
        )}

        {/* Create new app password */}
        <form
          onSubmit={createApp}
          className="flex flex-col gap-2.5 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-3.5 sm:flex-row sm:items-end"
        >
          <div className="min-w-0 flex-1 space-y-1.5">
            <Label htmlFor="app-password-name">{t('security.appName')}</Label>
            <Input
              id="app-password-name"
              value={appName}
              onChange={(e) => setAppName(e.target.value)}
              placeholder={t('security.appNamePlaceholder')}
              maxLength={80}
              required
              disabled={appBusy}
            />
          </div>
          <Button
            type="submit"
            size="sm"
            className="h-9 shrink-0"
            disabled={appBusy || !appName.trim()}
          >
            {appBusy ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <Plus className="h-3.5 w-3.5" />
            )}
            {t('security.createApp')}
          </Button>
        </form>

        {/* Existing app passwords list */}
        {apps.isPending ? (
          <SectionLoadingState rows={2} rowHeight="h-14" />
        ) : apps.data?.length === 0 ? (
          <EmptyState icon={KeyRound} title={t('security.noApps')} compact />
        ) : apps.data && apps.data.length > 0 ? (
          <ul className="divide-y divide-[var(--border-hairline)] rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)]">
            {apps.data.map((app) => (
              <li
                key={app.id}
                className="flex flex-wrap items-center justify-between gap-3 px-4 py-3"
              >
                <div className="min-w-0 space-y-0.5">
                  <p className="truncate text-[13.5px] font-medium text-[var(--ink-primary)]">
                    {app.name}
                  </p>
                  <p className="flex flex-wrap gap-x-3 gap-y-0.5 text-[12px] text-[var(--ink-muted)]">
                    <span>
                      {t('security.createdAt', {
                        date: formatMailFullTime(app.created_at, i18n.language),
                      })}
                    </span>
                    <span>
                      {app.last_used_at
                        ? t('security.lastUsed', {
                            date: formatMailFullTime(app.last_used_at, i18n.language),
                          })
                        : t('security.neverUsed')}
                    </span>
                  </p>
                </div>
                <Button
                  type="button"
                  variant="dangerGhost"
                  size="sm"
                  disabled={appBusy}
                  onClick={() => void removeApp(app.id, app.name)}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                  {t('security.removeApp')}
                </Button>
              </li>
            ))}
          </ul>
        ) : null}
      </SectionCard>
    </>
  )
}
