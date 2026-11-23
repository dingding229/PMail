import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Eye, EyeOff, HardDrive, Loader2, Lock, User } from 'lucide-react'
import { userApi } from '@/api'
import { getAppLanguage, setAppLanguage, type SupportedLanguage } from '@/i18n'
import { formatBytes } from '@/lib/format'
import { queryKeys, useUserMeQuery } from '@/hooks/useMailQueries'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import { AccountCredentials } from './AccountCredentials'

export function ProfileSecuritySection() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { data: user, isLoading } = useUserMeQuery(true)

  const [showOldPwd, setShowOldPwd] = useState(false)
  const [showNewPwd, setShowNewPwd] = useState(false)
  const [lang, setLang] = useState<SupportedLanguage>(getAppLanguage())

  const profileSchema = useMemo(
    () =>
      z.object({
        display_name: z
          .string()
          .trim()
          .min(1, t('settings.profile.validation.displayNameRequired'))
          .max(50, t('settings.profile.validation.displayNameMax')),
      }),
    [t],
  )

  const passwordSchema = useMemo(
    () =>
      z
        .object({
          old_password: z.string().min(1, t('settings.profile.validation.oldPasswordRequired')),
          new_password: z.string().min(8, t('settings.profile.validation.newPasswordMin')),
          confirm_password: z.string().min(1, t('settings.profile.validation.passwordMismatch')),
        })
        .refine((v) => v.new_password === v.confirm_password, {
          path: ['confirm_password'],
          message: t('settings.profile.validation.passwordMismatch'),
        }),
    [t],
  )

  const profileForm = useForm<z.infer<typeof profileSchema>>({
    resolver: zodResolver(profileSchema),
    defaultValues: { display_name: '' },
  })

  const passwordForm = useForm<z.infer<typeof passwordSchema>>({
    resolver: zodResolver(passwordSchema),
    defaultValues: { old_password: '', new_password: '', confirm_password: '' },
  })

  useEffect(() => {
    if (user) {
      profileForm.reset({ display_name: user.display_name || user.name || '' })
      if (user.lang) {
        const normalized: SupportedLanguage = user.lang.toLowerCase().startsWith('en') ? 'en' : 'zh-CN'
        setLang(normalized)
      }
    }
  }, [user, profileForm])

  const handleLanguageChange = async (nextLang: SupportedLanguage) => {
    setLang(nextLang)
    setAppLanguage(nextLang)
    try {
      await userApi.me({ lang: nextLang })
      await queryClient.invalidateQueries({ queryKey: queryKeys.userMe })
      toast.success(t('settings.profile.saved'))
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    }
  }

  const onProfileSubmit = async (values: z.infer<typeof profileSchema>) => {
    try {
      await userApi.me({ display_name: values.display_name, lang })
      await queryClient.invalidateQueries({ queryKey: queryKeys.userMe })
      toast.success(t('settings.profile.saved'))
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    }
  }

  const onPasswordSubmit = async (values: z.infer<typeof passwordSchema>) => {
    try {
      await userApi.changePassword({
        old_password: values.old_password,
        new_password: values.new_password,
      })
      toast.success(t('settings.profile.passwordUpdated'))
      queryClient.clear()
      navigate('/login', { replace: true })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('common.unknownError'))
    }
  }

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-48 w-full rounded-[var(--radius-lg)]" />
        <Skeleton className="h-28 w-full rounded-[var(--radius-lg)]" />
        <Skeleton className="h-56 w-full rounded-[var(--radius-lg)]" />
      </div>
    )
  }

  const usedBytes = user?.used_bytes ?? 0
  const quotaBytes = (user?.quota_mb ?? 0) * 1024 * 1024
  const quotaPct = quotaBytes > 0 ? Math.min(100, Math.round((usedBytes / quotaBytes) * 100)) : 0

  return (
    <div className="space-y-5">
      {/* Profile & Language Card */}
      <section className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] p-5 shadow-[var(--shadow-xs)]">
        <div className="mb-4 flex items-center gap-2.5">
          <div className="flex h-8 w-8 items-center justify-center rounded-[var(--radius-sm)] bg-[var(--accent-soft)] text-[var(--accent)]">
            <User className="h-4 w-4" />
          </div>
          <div>
            <h2 className="text-[15px] font-semibold text-[var(--ink-primary)]">
              {t('settings.profile.title')}
            </h2>
            <p className="text-[12.5px] text-[var(--ink-secondary)]">
              {t('settings.profile.desc')}
            </p>
          </div>
        </div>

        <form onSubmit={profileForm.handleSubmit(onProfileSubmit)} className="space-y-4 max-w-lg">
          <div className="space-y-1.5">
            <Label>{t('settings.profile.account')}</Label>
            <Input value={user?.account ?? ''} disabled />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="profile_display_name">{t('settings.profile.displayName')}</Label>
            <Input
              id="profile_display_name"
              {...profileForm.register('display_name')}
              placeholder={t('settings.profile.displayNamePlaceholder')}
            />
            {profileForm.formState.errors.display_name && (
              <p className="text-[12px] text-[var(--danger)]">
                {profileForm.formState.errors.display_name.message}
              </p>
            )}
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="profile_language">{t('settings.profile.language')}</Label>
            <Select
              value={lang}
              onValueChange={(v) => void handleLanguageChange(v as SupportedLanguage)}
            >
              <SelectTrigger id="profile_language" className="h-9">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="zh-CN">{t('language.zhCN')}</SelectItem>
                <SelectItem value="en">{t('language.en')}</SelectItem>
              </SelectContent>
            </Select>
            <p className="text-[12px] text-[var(--ink-muted)]">
              {t('settings.profile.languageHint')}
            </p>
          </div>

          <Button type="submit" size="sm" disabled={profileForm.formState.isSubmitting}>
            {profileForm.formState.isSubmitting && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
            {t('common.save')}
          </Button>
        </form>
      </section>

      {/* Storage Quota Card */}
      <section className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] p-5 shadow-[var(--shadow-xs)]">
        <div className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-[var(--radius-sm)] bg-[var(--surface-subtle)] text-[var(--ink-secondary)]">
              <HardDrive className="h-4 w-4" />
            </div>
            <div>
              <h2 className="text-[14.5px] font-semibold text-[var(--ink-primary)]">
                {t('settings.profile.storageTitle')}
              </h2>
              <p className="text-[12.5px] tabular-nums text-[var(--ink-secondary)]">
                {quotaBytes > 0
                  ? t('settings.profile.storageUsed', {
                      used: formatBytes(usedBytes),
                      total: formatBytes(quotaBytes),
                    })
                  : t('settings.profile.storageUnlimited', { used: formatBytes(usedBytes) })}
              </p>
            </div>
          </div>
          {quotaBytes > 0 && (
            <span className="text-[13px] font-semibold tabular-nums text-[var(--ink-primary)]">
              {quotaPct}%
            </span>
          )}
        </div>

        {quotaBytes > 0 && (
          <div className="mt-3.5 h-2 w-full overflow-hidden rounded-full bg-[var(--surface-subtle)]">
            <div
              style={{ width: `${quotaPct}%` }}
              className={`h-full rounded-full transition-all ${
                quotaPct >= 90
                  ? 'bg-[var(--danger)]'
                  : quotaPct >= 75
                    ? 'bg-[var(--warning)]'
                    : 'bg-[var(--accent)]'
              }`}
            />
          </div>
        )}
      </section>

      {/* Change Password Card */}
      <section className="rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] p-5 shadow-[var(--shadow-xs)]">
        <div className="mb-4 flex items-center gap-2.5">
          <div className="flex h-8 w-8 items-center justify-center rounded-[var(--radius-sm)] bg-[var(--warning-soft)] text-[var(--warning)]">
            <Lock className="h-4 w-4" />
          </div>
          <div>
            <h2 className="text-[15px] font-semibold text-[var(--ink-primary)]">
              {t('settings.profile.passwordTitle')}
            </h2>
            <p className="text-[12.5px] text-[var(--ink-secondary)]">
              {t('settings.profile.passwordDesc')}
            </p>
          </div>
        </div>

        <form onSubmit={passwordForm.handleSubmit(onPasswordSubmit)} className="space-y-4 max-w-lg">
          <div className="space-y-1.5">
            <Label htmlFor="old_password">{t('settings.profile.oldPassword')}</Label>
            <div className="relative">
              <Input
                id="old_password"
                type={showOldPwd ? 'text' : 'password'}
                autoComplete="current-password"
                className="pr-9"
                {...passwordForm.register('old_password')}
              />
              <button
                type="button"
                onClick={() => setShowOldPwd((v) => !v)}
                aria-label={showOldPwd ? t('auth.hidePassword') : t('auth.showPassword')}
                className="absolute right-2.5 top-1/2 -translate-y-1/2 text-[var(--ink-muted)] hover:text-[var(--ink-primary)]"
              >
                {showOldPwd ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              </button>
            </div>
            {passwordForm.formState.errors.old_password && (
              <p className="text-[12px] text-[var(--danger)]">
                {passwordForm.formState.errors.old_password.message}
              </p>
            )}
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="new_password">{t('settings.profile.newPassword')}</Label>
            <div className="relative">
              <Input
                id="new_password"
                type={showNewPwd ? 'text' : 'password'}
                autoComplete="new-password"
                className="pr-9"
                {...passwordForm.register('new_password')}
              />
              <button
                type="button"
                onClick={() => setShowNewPwd((v) => !v)}
                aria-label={showNewPwd ? t('auth.hidePassword') : t('auth.showPassword')}
                className="absolute right-2.5 top-1/2 -translate-y-1/2 text-[var(--ink-muted)] hover:text-[var(--ink-primary)]"
              >
                {showNewPwd ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              </button>
            </div>
            {passwordForm.formState.errors.new_password && (
              <p className="text-[12px] text-[var(--danger)]">
                {passwordForm.formState.errors.new_password.message}
              </p>
            )}
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="confirm_password">{t('settings.profile.confirmPassword')}</Label>
            <Input
              id="confirm_password"
              type={showNewPwd ? 'text' : 'password'}
              autoComplete="new-password"
              {...passwordForm.register('confirm_password')}
            />
            {passwordForm.formState.errors.confirm_password && (
              <p className="text-[12px] text-[var(--danger)]">
                {passwordForm.formState.errors.confirm_password.message}
              </p>
            )}
          </div>

          <Button type="submit" size="sm" disabled={passwordForm.formState.isSubmitting}>
            {passwordForm.formState.isSubmitting && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
            {t('settings.profile.passwordSubmit')}
          </Button>
        </form>
      </section>
      {user && <AccountCredentials key={user.id} userID={user.id} />}
    </div>
  )
}
