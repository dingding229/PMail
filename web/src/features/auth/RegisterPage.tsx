import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router'
import { useForm, Controller } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Check, Copy, Eye, EyeOff, Globe, KeyRound, Loader2, Terminal } from 'lucide-react'
import { authApi } from '@/api'
import { queryKeys, useBootstrapQuery } from '@/hooks/useMailQueries'
import { formatRegistrationKey } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { AuthLayout } from './AuthLayout'
import { DOMAIN_PATTERN, guessMailDomain } from './mailDomain'

const sleep = (ms: number) => new Promise((resolve) => window.setTimeout(resolve, ms))

function scorePassword(pwd: string): 0 | 1 | 2 | 3 | 4 {
  if (!pwd) return 0
  let score = 0
  if (pwd.length >= 8) score++
  if (pwd.length >= 12) score++
  if (/[a-z]/.test(pwd) && /[A-Z]/.test(pwd)) score++
  if (/\d/.test(pwd) && /[^A-Za-z0-9]/.test(pwd)) score++
  return Math.min(4, Math.max(1, score)) as 1 | 2 | 3 | 4
}

export function RegisterPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { data: bootstrap } = useBootstrapQuery()
  const needDomain = bootstrap?.need_domain === true
  const [applyingDomain, setApplyingDomain] = useState(false)

  const [showPwd, setShowPwd] = useState(false)
  const [showConfirm, setShowConfirm] = useState(false)
  const [copied, setCopied] = useState(false)
  const [serverError, setServerError] = useState<string | null>(null)

  const schema = useMemo(
    () =>
      z
        .object({
          mail_domain: z.string().trim().toLowerCase(),
          registration_key: z
            .string()
            .trim()
            .min(1, t('auth.validation.keyRequired'))
            .regex(/^[A-HJ-NP-Z2-9]{4}(-[A-HJ-NP-Z2-9]{4}){3}$/, t('auth.validation.keyFormat')),
          account: z
            .string()
            .trim()
            .min(1, t('auth.validation.accountRequired'))
            .regex(/^[a-zA-Z0-9][a-zA-Z0-9._-]{1,19}$/, t('auth.validation.accountFormat')),
          display_name: z
            .string()
            .trim()
            .min(1, t('auth.validation.displayNameRequired'))
            .max(50, t('auth.validation.displayNameMax')),
          password: z
            .string()
            .min(1, t('auth.validation.passwordRequired'))
            .min(8, t('auth.validation.passwordMin')),
          confirm_password: z.string().min(1, t('auth.validation.confirmPasswordRequired')),
        })
        .refine((vals) => vals.password === vals.confirm_password, {
          path: ['confirm_password'],
          message: t('auth.validation.passwordMismatch'),
        })
        .refine((vals) => !needDomain || vals.mail_domain.length > 0, {
          path: ['mail_domain'],
          message: t('auth.validation.domainRequired'),
        })
        .refine((vals) => !needDomain || vals.mail_domain.length === 0 || DOMAIN_PATTERN.test(vals.mail_domain), {
          path: ['mail_domain'],
          message: t('auth.validation.domainFormat'),
        }),
    [t, needDomain],
  )

  type RegisterFormValues = z.infer<typeof schema>

  const {
    control,
    register,
    watch,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<RegisterFormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      mail_domain: guessMailDomain(typeof window === 'undefined' ? '' : window.location.hostname),
      registration_key: '',
      account: 'admin',
      display_name: '',
      password: '',
      confirm_password: '',
    },
  })

  const pwdValue = watch('password')
  const pwdScore = scorePassword(pwdValue)
  const typedDomain = watch('mail_domain')
  const primaryDomain = needDomain ? typedDomain?.trim().toLowerCase() : bootstrap?.primary_domain

  const commandText = t('auth.registerKeyCommand')

  const handleCopyCommand = async () => {
    try {
      await navigator.clipboard.writeText(commandText)
      setCopied(true)
      toast.success(t('common.copied'))
      window.setTimeout(() => setCopied(false), 1800)
    } catch {
      toast.error(t('common.copyFailed'))
    }
  }

  const onSubmit = async (values: RegisterFormValues) => {
    setServerError(null)
    try {
      if (needDomain) {
        await authApi.setupDomain({
          registration_key: values.registration_key,
          domain: values.mail_domain,
        })
        // PMail now finishes starting with the new domain; wait until it is back.
        setApplyingDomain(true)
        let ready = false
        for (let i = 0; i < 60 && !ready; i++) {
          await sleep(1000)
          try {
            const next = await authApi.bootstrap()
            ready = next.need_domain !== true
            if (ready) queryClient.setQueryData(queryKeys.bootstrap, next)
          } catch {
            // The server is restarting its listeners.
          }
        }
        setApplyingDomain(false)
        if (!ready) throw new Error(t('auth.errors.domainSetupTimeout'))
      }
      await authApi.register({
        registration_key: values.registration_key,
        account: values.account.toLowerCase(),
        display_name: values.display_name,
        password: values.password,
      })
      toast.success(t('auth.registerSuccess'))
      await queryClient.invalidateQueries({ queryKey: queryKeys.bootstrap })
      await queryClient.invalidateQueries({ queryKey: queryKeys.userMe })
      navigate('/mail/inbox', { replace: true })
    } catch (err) {
      setApplyingDomain(false)
      const msg = err instanceof Error ? err.message : t('common.unknownError')
      setServerError(msg)
      toast.error(msg)
    }
  }

  const strengthLabels = [
    '',
    t('auth.passwordStrength.weak'),
    t('auth.passwordStrength.fair'),
    t('auth.passwordStrength.good'),
    t('auth.passwordStrength.strong'),
  ]
  const strengthColors = [
    'bg-[var(--border-subtle)]',
    'bg-red-500',
    'bg-amber-500',
    'bg-blue-500',
    'bg-emerald-500',
  ]

  return (
    <AuthLayout
      title={t('auth.registerTitle')}
      subtitle={t('auth.registerSubtitle')}
      wide
    >
      {/* Registration key Docker log hint box */}
      <div className="mb-5 rounded-[var(--radius-md)] border border-[var(--border-subtle)] bg-[var(--surface-subtle)] p-3.5">
        <div className="flex items-center gap-2 text-[12.5px] font-medium text-[var(--ink-primary)]">
          <Terminal className="h-3.5 w-3.5 text-[var(--accent)] shrink-0" />
          <span>{t('auth.registerKeyHintTitle')}</span>
        </div>
        <p className="mt-1 text-[12px] leading-relaxed text-[var(--ink-secondary)]">
          {t('auth.registerKeyHintDesc')}
        </p>
        <div className="mt-2 flex items-center justify-between gap-2 rounded-[var(--radius-sm)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] px-2.5 py-1.5">
          <code className="overflow-x-auto font-mono text-[12px] text-[var(--ink-primary)] whitespace-nowrap">
            {commandText}
          </code>
          <Button
            type="button"
            variant="ghost"
            size="iconSm"
            onClick={handleCopyCommand}
            aria-label={t('common.copy')}
            title={t('common.copy')}
          >
            {copied ? (
              <Check className="h-3.5 w-3.5 text-[var(--success)]" />
            ) : (
              <Copy className="h-3.5 w-3.5" />
            )}
          </Button>
        </div>
      </div>

      {serverError && (
        <div
          role="alert"
          className="mb-4 rounded-[var(--radius-sm)] border border-[var(--danger)]/30 bg-[var(--danger-soft)] px-3 py-2 text-[13px] text-[var(--danger)]"
        >
          {serverError}
        </div>
      )}

      <form onSubmit={handleSubmit(onSubmit)} className="space-y-4" noValidate>
        {needDomain && (
          <div className="space-y-1.5">
            <Label htmlFor="mail_domain">{t('auth.mailDomainLabel')}</Label>
            <div className="relative">
              <Globe className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--ink-muted)]" />
              <Input
                id="mail_domain"
                {...register('mail_domain')}
                placeholder="example.com"
                autoComplete="off"
                spellCheck={false}
                className="pl-9"
              />
            </div>
            {errors.mail_domain ? (
              <p className="text-[12px] text-[var(--danger)]">{errors.mail_domain.message}</p>
            ) : (
              <p className="text-[11.5px] leading-relaxed text-[var(--ink-muted)]">{t('auth.mailDomainHint')}</p>
            )}
          </div>
        )}

        {/* Registration Key */}
        <div className="space-y-1.5">
          <Label htmlFor="registration_key">{t('auth.registerKeyLabel')}</Label>
          <Controller
            name="registration_key"
            control={control}
            render={({ field }) => (
              <div className="relative">
                <KeyRound className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--ink-muted)]" />
                <Input
                  id="registration_key"
                  value={field.value}
                  onChange={(e) => field.onChange(formatRegistrationKey(e.target.value))}
                  onBlur={field.onBlur}
                  placeholder={t('auth.registerKeyPlaceholder')}
                  autoComplete="off"
                  spellCheck={false}
                  maxLength={19}
                  className="pl-9 font-mono tracking-[0.12em] uppercase"
                />
              </div>
            )}
          />
          {errors.registration_key && (
            <p className="text-[12px] text-[var(--danger)]">{errors.registration_key.message}</p>
          )}
        </div>

        {/* Admin Account & Display Name */}
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="space-y-1.5">
            <Label htmlFor="reg_account">{t('auth.adminAccountLabel')}</Label>
            <div className="relative flex items-center">
              <Input
                id="reg_account"
                {...register('account')}
                placeholder={t('auth.adminAccountPlaceholder')}
                autoComplete="username"
              />
            </div>
            {primaryDomain && !errors.account && (
              <p className="truncate text-[11.5px] text-[var(--ink-muted)]">
                @{primaryDomain}
              </p>
            )}
            {errors.account && (
              <p className="text-[12px] text-[var(--danger)]">{errors.account.message}</p>
            )}
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="reg_display_name">{t('auth.displayNameLabel')}</Label>
            <Input
              id="reg_display_name"
              {...register('display_name')}
              placeholder={t('auth.displayNamePlaceholder')}
              autoComplete="name"
            />
            {errors.display_name && (
              <p className="text-[12px] text-[var(--danger)]">{errors.display_name.message}</p>
            )}
          </div>
        </div>

        {/* Password */}
        <div className="space-y-1.5">
          <Label htmlFor="reg_password">{t('auth.passwordLabel')}</Label>
          <div className="relative">
            <Input
              id="reg_password"
              type={showPwd ? 'text' : 'password'}
              {...register('password')}
              placeholder={t('auth.passwordPlaceholder')}
              autoComplete="new-password"
              className="pr-9"
            />
            <button
              type="button"
              onClick={() => setShowPwd((v) => !v)}
              aria-label={showPwd ? t('auth.hidePassword') : t('auth.showPassword')}
              className="absolute right-2.5 top-1/2 -translate-y-1/2 text-[var(--ink-muted)] hover:text-[var(--ink-primary)]"
            >
              {showPwd ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
            </button>
          </div>
          {pwdValue && (
            <div className="flex items-center gap-2 pt-0.5">
              <div className="flex flex-1 gap-1">
                {[1, 2, 3, 4].map((step) => (
                  <div
                    key={step}
                    className={`h-1 flex-1 rounded-full transition-colors ${
                      step <= pwdScore ? strengthColors[pwdScore] : 'bg-[var(--border-subtle)]'
                    }`}
                  />
                ))}
              </div>
              <span className="text-[11px] text-[var(--ink-muted)]">
                {strengthLabels[pwdScore]}
              </span>
            </div>
          )}
          {errors.password && (
            <p className="text-[12px] text-[var(--danger)]">{errors.password.message}</p>
          )}
        </div>

        {/* Confirm Password */}
        <div className="space-y-1.5">
          <Label htmlFor="reg_confirm_password">{t('auth.confirmPasswordLabel')}</Label>
          <div className="relative">
            <Input
              id="reg_confirm_password"
              type={showConfirm ? 'text' : 'password'}
              {...register('confirm_password')}
              placeholder={t('auth.confirmPasswordPlaceholder')}
              autoComplete="new-password"
              className="pr-9"
            />
            <button
              type="button"
              onClick={() => setShowConfirm((v) => !v)}
              aria-label={showConfirm ? t('auth.hidePassword') : t('auth.showPassword')}
              className="absolute right-2.5 top-1/2 -translate-y-1/2 text-[var(--ink-muted)] hover:text-[var(--ink-primary)]"
            >
              {showConfirm ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
            </button>
          </div>
          {errors.confirm_password && (
            <p className="text-[12px] text-[var(--danger)]">{errors.confirm_password.message}</p>
          )}
        </div>

        <Button type="submit" size="lg" className="mt-2 w-full" disabled={isSubmitting}>
          {isSubmitting && <Loader2 className="h-4 w-4 animate-spin" />}
          {applyingDomain
            ? t('auth.applyingDomain')
            : isSubmitting
              ? t('auth.registering')
              : t('auth.registerSubmit')}
        </Button>
      </form>
    </AuthLayout>
  )
}
