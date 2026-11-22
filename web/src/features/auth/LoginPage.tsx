import { useMemo, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Eye, EyeOff, Loader2 } from 'lucide-react'
import { authApi } from '@/api'
import { queryKeys } from '@/hooks/useMailQueries'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { AuthLayout } from './AuthLayout'

export function LoginPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const queryClient = useQueryClient()

  const [showPwd, setShowPwd] = useState(false)
  const [serverError, setServerError] = useState<string | null>(null)

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
    formState: { errors, isSubmitting },
  } = useForm<LoginFormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      account: '',
      password: '',
    },
  })

  const onSubmit = async (values: LoginFormValues) => {
    setServerError(null)
    try {
      const user = await authApi.login({
        account: values.account.trim(),
        password: values.password,
      })
      toast.success(t('auth.loginSuccess', { name: user.name || user.account }))
      await queryClient.invalidateQueries({ queryKey: queryKeys.bootstrap })
      await queryClient.invalidateQueries({ queryKey: queryKeys.userMe })
      const redirect = searchParams.get('redirect')
      const safeTarget = redirect && redirect.startsWith('/') && !redirect.startsWith('//')
        ? redirect
        : '/mail/inbox'
      navigate(safeTarget, { replace: true })
    } catch (err) {
      const msg = err instanceof Error ? err.message : t('common.unknownError')
      setServerError(msg)
      toast.error(msg)
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

      <form onSubmit={handleSubmit(onSubmit)} className="space-y-4" noValidate>
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
    </AuthLayout>
  )
}
