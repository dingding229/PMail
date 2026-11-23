import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Eye, EyeOff, Loader2 } from 'lucide-react'
import { authApi, type AuthSessionUser } from '@/api'
import { ApiError } from '@/api/client'
import { queryKeys } from '@/hooks/useMailQueries'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { AuthLayout } from './AuthLayout'
import { SecondFactorField } from './SecondFactorField'

export function LoginPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const queryClient = useQueryClient()

  const [showPwd, setShowPwd] = useState(false)
  const [serverError, setServerError] = useState<string | null>(null)

  const [pending, setPending] = useState<{ token: string; deadline: number } | null>(null)
  const [code, setCode] = useState('')
  const [recovery, setRecovery] = useState(false)
  const [verifying, setVerifying] = useState(false)

  useEffect(() => {
    if (!pending || verifying) return
    const timer = window.setTimeout(() => {
      setPending(null)
      setCode('')
      setRecovery(false)
      setServerError(t('security.challengeExpired'))
    }, Math.max(0, pending.deadline - Date.now()))
    return () => window.clearTimeout(timer)
  }, [pending, t, verifying])

  const schema = useMemo(
    () =>
      z.object({
        account: z.string().trim().min(1, t('auth.validation.accountRequired')),
        password: z.string().min(1, t('auth.validation.passwordRequired')),
      }),
    [t],
  )

  type LoginFormValues = z.infer<typeof schema>

  const {
    register,
    handleSubmit,
    resetField,
    formState: { errors, isSubmitting },
  } = useForm<LoginFormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      account: '',
      password: '',
    },
  })

  const completeLogin = async (user: AuthSessionUser) => {
    toast.success(t('auth.loginSuccess', { name: user.name || user.account }))
    await queryClient.invalidateQueries({ queryKey: queryKeys.bootstrap })
    await queryClient.invalidateQueries({ queryKey: queryKeys.userMe })
    const redirect = searchParams.get('redirect')
    const safeTarget = redirect && redirect.startsWith('/') && !redirect.startsWith('//')
      ? redirect
      : '/mail/inbox'
    navigate(safeTarget, { replace: true })
  }

  const showError = (err: unknown) => {
    const msg = err instanceof Error ? err.message : t('common.unknownError')
    setServerError(msg)
    toast.error(msg)
  }

  const onSubmit = async (values: LoginFormValues) => {
    setServerError(null)
    try {
      const result = await authApi.login({ account: values.account.trim(), password: values.password })
      if ('two_factor_required' in result) {
        resetField('password')
        setShowPwd(false)
        setPending({ token: result.challenge, deadline: Date.now() + result.expires_in * 1000 })
        return
      }
      await completeLogin(result)
    } catch (err) {
      showError(err)
    }
  }

  const submitFactor = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!pending || verifying) return
    setVerifying(true)
    setServerError(null)
    try {
      const user = await authApi.loginSecondFactor({ challenge: pending.token, code: code.trim(), recovery })
      setCode('')
      await completeLogin(user)
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setPending(null)
        setRecovery(false)
      }
      setCode('')
      showError(err)
    } finally {
      setVerifying(false)
    }
  }

  return (
    <AuthLayout
      title={t('auth.loginTitle')}
      subtitle={t('auth.loginSubtitle')}
    >
      {serverError && (
        <div
          role="alert"
          className="mb-4 rounded-[var(--radius-sm)] border border-[var(--danger)]/30 bg-[var(--danger-soft)] px-3 py-2 text-[13px] text-[var(--danger)]"
        >
          {serverError}
        </div>
      )}

      {pending ? (
        <form key="factor" onSubmit={submitFactor} className="space-y-4">
          <p className="text-sm text-[var(--ink-secondary)]">{t('security.loginPrompt')}</p>
          <SecondFactorField autoFocus code={code} recovery={recovery} onCode={setCode} onRecovery={setRecovery} disabled={verifying} />
          <Button type="submit" size="lg" className="w-full" disabled={verifying}>
            {verifying && <Loader2 className="h-4 w-4 animate-spin" />}
            {t('security.verify')}
          </Button>
          <Button type="button" variant="ghost" className="w-full" disabled={verifying} onClick={() => {
            setPending(null)
            setRecovery(false)
            setCode('')
            setServerError(null)
          }}>{t('security.backToPassword')}</Button>
        </form>
      ) : (
        <form key="password" onSubmit={handleSubmit(onSubmit)} className="space-y-4" noValidate>
          <div className="space-y-1.5">
            <Label htmlFor="login_account">{t('auth.accountLabel')}</Label>
            <Input
              id="login_account"
              {...register('account')}
              placeholder={t('auth.accountPlaceholder')}
              autoComplete="username"
              autoFocus
            />
            {errors.account && (
              <p className="text-[12px] text-[var(--danger)]">{errors.account.message}</p>
            )}
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="login_password">{t('auth.passwordLabel')}</Label>
            <div className="relative">
              <Input
                id="login_password"
                type={showPwd ? 'text' : 'password'}
                {...register('password')}
                placeholder={t('auth.passwordPlaceholder')}
                autoComplete="current-password"
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
            {errors.password && (
              <p className="text-[12px] text-[var(--danger)]">{errors.password.message}</p>
            )}
          </div>

          <Button type="submit" size="lg" className="mt-2 w-full" disabled={isSubmitting}>
            {isSubmitting && <Loader2 className="h-4 w-4 animate-spin" />}
            {isSubmitting ? t('auth.loggingIn') : t('auth.loginSubmit')}
          </Button>
        </form>
      )}
    </AuthLayout>
  )
}
