import { useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { securityApi } from '@/api/security'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { SecondFactorField } from '@/features/auth/SecondFactorField'
import { queryKeys } from '@/hooks/useMailQueries'
import { formatMailFullTime } from '@/lib/date'

const card = 'space-y-4 rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] p-5 shadow-[var(--shadow-xs)]'
const heading = 'text-[15px] font-semibold text-[var(--ink-primary)]'
const hint = 'text-sm text-[var(--ink-secondary)]'
const errorStyle = 'text-sm text-[var(--danger)]'

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

  const message = (err: unknown) => err instanceof Error ? err.message : t('common.unknownError')
  const choose = (next: FactorForm | null) => {
    setForm(next)
    setCode('')
    setPassword('')
    setRecovery(false)
    setFactorError('')
    setSetup(null)
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
      const result = form === 'enable' ? await securityApi.enable(confirm.code)
        : form === 'disable' ? await securityApi.disable(confirm)
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

  return (
    <>
      <section className={card} aria-labelledby="factor-title">
        <h2 className={heading} id="factor-title">{t('security.title')}</h2>
        {factor.isPending && <p className={hint}>{t('common.loading')}</p>}
        {factor.error && <p className={errorStyle} role="alert">{factor.error.message}</p>}
        {factorError && <p className={errorStyle} role="alert">{factorError}</p>}
        {factor.data && <p className={hint} role="status">{factor.data.enabled ? t('security.enabled') : t('security.disabled')}</p>}
        {factor.data && !form && !factor.data.enabled && (
          <Button type="button" disabled={factorBusy} onClick={() => void startSetup()}>{t('security.startSetup')}</Button>
        )}
        {factor.data?.enabled && !form && (
          <div className="flex flex-wrap gap-2">
            <Button type="button" variant="outline" onClick={() => choose('regenerate')}>{t('security.regenerate')}</Button>
            <Button type="button" variant="outline" onClick={() => choose('disable')}>{t('security.disable')}</Button>
          </div>
        )}
        {form && (
          <form onSubmit={submitFactor} className="max-w-lg space-y-3">
            {form === 'enable' && setup && (
              <div className="space-y-2">
                <p className={hint}>{t('security.scanHint')}</p>
                {setup.qr && <img className="rounded bg-white" src={setup.qr} width={192} height={192} alt={t('security.qrAlt')} />}
                <Label htmlFor="totp-manual-key">{t('security.manualKey')}</Label>
                <Input id="totp-manual-key" value={setup.secret} readOnly className="font-mono" autoComplete="off" />
              </div>
            )}
            {form !== 'enable' && (
              <div className="space-y-2">
                <p className={hint}>{form === 'disable' ? t('security.disableHint') : t('security.regenerateHint')}</p>
                <Label htmlFor="factor-current-password">{t('settings.profile.oldPassword')}</Label>
                <Input id="factor-current-password" value={password} onChange={(e) => setPassword(e.target.value)} type="password" autoComplete="current-password" required disabled={factorBusy} />
              </div>
            )}
            <SecondFactorField code={code} recovery={recovery} onCode={setCode}
              onRecovery={form !== 'enable' ? setRecovery : undefined} disabled={factorBusy} />
            <div className="flex flex-wrap gap-2">
              <Button type="submit" disabled={factorBusy}>{form === 'enable' ? t('security.enable') : form === 'disable' ? t('security.disable') : t('security.regenerate')}</Button>
              <Button type="button" variant="ghost" disabled={factorBusy} onClick={() => choose(null)}>{t('common.cancel')}</Button>
            </div>
          </form>
        )}
        {recoveryCodes.length > 0 && (
          <div className="space-y-2">
            <h3 className={heading}>{t('security.recoveryCodes')}</h3>
            <p className={hint}>{t('security.recoveryWarning')}</p>
            <ul className="grid gap-1 font-mono text-sm sm:grid-cols-2">
              {recoveryCodes.map((item) => <li key={item}>{item}</li>)}
            </ul>
            <div className="flex flex-wrap gap-2">
              <Button type="button" variant="outline" onClick={downloadCodes}>{t('security.downloadCodes')}</Button>
              <Button type="button" variant="ghost" onClick={() => setRecoveryCodes([])}>{t('security.done')}</Button>
            </div>
          </div>
        )}
      </section>
      <section className={card} aria-labelledby="apps-title">
        <h2 id="apps-title" className={heading}>{t('security.appPasswords')}</h2>
        <p className={hint}>{t('security.clientHint')}</p>
        {apps.isPending && <p className={hint}>{t('common.loading')}</p>}
        {apps.error && <p className={errorStyle} role="alert">{apps.error.message}</p>}
        {appError && <p className={errorStyle} role="alert">{appError}</p>}
        {apps.data?.length === 0 && <p className={hint}>{t('security.noApps')}</p>}
        {!!apps.data?.length && (
          <ul className="space-y-2">
            {apps.data.map((app) => (
              <li key={app.id} className="flex flex-wrap items-center justify-between gap-2 rounded border border-[var(--border-subtle)] p-2.5">
                <div className="min-w-0 break-words">
                  <p className="text-sm font-medium">{app.name}</p>
                  <p className={hint}>{t('security.createdAt', { date: formatMailFullTime(app.created_at, i18n.language) })}</p>
                  <p className={hint}>{app.last_used_at ? t('security.lastUsed', { date: formatMailFullTime(app.last_used_at, i18n.language) }) : t('security.neverUsed')}</p>
                </div>
                <Button type="button" variant="outline" size="sm" disabled={appBusy} onClick={() => void removeApp(app.id, app.name)}>{t('security.removeApp')}</Button>
              </li>
            ))}
          </ul>
        )}
        <form onSubmit={createApp} className="max-w-lg space-y-2">
          <Label htmlFor="app-password-name">{t('security.appName')}</Label>
          <Input id="app-password-name" value={appName} onChange={(e) => setAppName(e.target.value)} maxLength={80} required disabled={appBusy} />
          <Button type="submit" disabled={appBusy || !appName.trim()}>{t('security.createApp')}</Button>
        </form>
        {newApp && (
          <div className="max-w-lg space-y-2">
            <Label htmlFor="app-password-new">{t('security.newAppWarning')}</Label>
            <Input id="app-password-new" value={newApp} readOnly autoComplete="off" className="font-mono" />
            <Button type="button" variant="ghost" onClick={() => setNewApp('')}>{t('security.done')}</Button>
          </div>
        )}
      </section>
    </>
  )
}
