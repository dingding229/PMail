import { useTranslation } from 'react-i18next'
import { Lock, Trash2, X } from 'lucide-react'
import type { ProxyProtocol } from '@/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  isProxyDraftEmpty,
  type ProxyDraft,
  type ProxyValidationErrors,
} from './proxyHelpers'

export interface ProxyFieldsProps {
  value: ProxyDraft
  onChange: (next: ProxyDraft) => void
  errors?: ProxyValidationErrors
  disabled?: boolean
  disabledReason?: string
  onClearProxy?: () => void
  canClearProxy?: boolean
  idPrefix?: string
}

export function ProxyFields({
  value,
  onChange,
  errors,
  disabled = false,
  disabledReason,
  onClearProxy,
  canClearProxy = false,
  idPrefix = 'proxy',
}: ProxyFieldsProps) {
  const { t } = useTranslation()

  const patch = (partial: Partial<ProxyDraft>) => {
    onChange({ ...value, ...partial })
  }

  return (
    <div className="space-y-3 rounded-[var(--radius-md)] border border-[var(--border-hairline)] bg-[var(--surface-canvas)] p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <span className="text-[13px] font-medium text-[var(--ink-primary)]">
            {t('proxy.title')}
          </span>
          {disabled && disabledReason && (
            <Badge variant="default">
              <Lock className="h-3 w-3" />
              {t('admin.inbound.envControlledBadge')}
            </Badge>
          )}
        </div>
        {onClearProxy && !disabled && (
          <Button
            type="button"
            variant="dangerGhost"
            size="xs"
            disabled={!canClearProxy && isProxyDraftEmpty(value)}
            onClick={onClearProxy}
          >
            <Trash2 className="h-3.5 w-3.5" />
            {t('proxy.clearProxy')}
          </Button>
        )}
      </div>

      {disabled && disabledReason && (
        <div className="flex items-start gap-2 rounded-[var(--radius-sm)] border border-[var(--border-subtle)] bg-[var(--surface-subtle)] px-3 py-2 text-[12px] text-[var(--ink-secondary)]">
          <Lock className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[var(--ink-muted)]" />
          <span>{disabledReason}</span>
        </div>
      )}

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-12">
        {/* Protocol */}
        <div className="space-y-1.5 sm:col-span-3">
          <Label htmlFor={`${idPrefix}_type`}>{t('proxy.protocol')}</Label>
          <Select
            value={value.type}
            disabled={disabled}
            onValueChange={(next) => patch({ type: next as ProxyProtocol })}
          >
            <SelectTrigger id={`${idPrefix}_type`} className="h-9">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="http">HTTP</SelectItem>
              <SelectItem value="https">HTTPS</SelectItem>
              <SelectItem value="socks5">SOCKS5</SelectItem>
            </SelectContent>
          </Select>
        </div>

        {/* Host */}
        <div className="space-y-1.5 sm:col-span-6">
          <Label htmlFor={`${idPrefix}_host`}>{t('proxy.host')}</Label>
          <Input
            id={`${idPrefix}_host`}
            value={value.host}
            disabled={disabled}
            onChange={(e) => patch({ host: e.target.value })}
            placeholder={t('proxy.hostPlaceholder')}
            className="font-mono text-[13px]"
          />
          {errors?.host && (
            <p className="text-[11.5px] text-[var(--danger)]">
              {errors.host === 'invalidUrl'
                ? t('proxy.validation.hostNoUrl')
                : t('proxy.validation.hostRequired')}
            </p>
          )}
        </div>

        {/* Port */}
        <div className="space-y-1.5 sm:col-span-3">
          <Label htmlFor={`${idPrefix}_port`}>{t('proxy.port')}</Label>
          <Input
            id={`${idPrefix}_port`}
            type="number"
            min={1}
            max={65535}
            inputMode="numeric"
            value={value.port}
            disabled={disabled}
            onChange={(e) => patch({ port: e.target.value })}
            placeholder="1080"
            className="font-mono text-[13px] tabular-nums"
          />
          {errors?.port && (
            <p className="text-[11.5px] text-[var(--danger)]">
              {t('proxy.validation.portInvalid')}
            </p>
          )}
        </div>
      </div>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        {/* Username */}
        <div className="space-y-1.5">
          <Label htmlFor={`${idPrefix}_username`}>{t('proxy.username')}</Label>
          <Input
            id={`${idPrefix}_username`}
            value={value.username}
            disabled={disabled}
            onChange={(e) => patch({ username: e.target.value })}
            placeholder={t('proxy.usernamePlaceholder')}
            autoComplete="off"
          />
        </div>

        {/* Password */}
        <div className="space-y-1.5">
          <div className="flex items-center justify-between gap-2">
            <div className="flex items-center gap-1.5">
              <Label htmlFor={`${idPrefix}_password`}>{t('proxy.password')}</Label>
              {value.passwordSet && !value.clearPassword && (
                <Badge variant="success">{t('proxy.passwordSet')}</Badge>
              )}
              {value.clearPassword && (
                <Badge variant="warning">{t('proxy.passwordWillClear')}</Badge>
              )}
            </div>
            {value.passwordSet && !disabled && (
              <button
                type="button"
                onClick={() =>
                  patch({
                    clearPassword: !value.clearPassword,
                    password: '',
                  })
                }
                className="inline-flex items-center gap-0.5 text-[11.5px] text-[var(--ink-muted)] transition-colors hover:text-[var(--danger)]"
              >
                <X className="h-3 w-3" />
                {value.clearPassword ? t('common.cancel') : t('proxy.clearPassword')}
              </button>
            )}
          </div>
          <Input
            id={`${idPrefix}_password`}
            type="password"
            value={value.password}
            disabled={disabled || value.clearPassword}
            onChange={(e) => patch({ password: e.target.value, clearPassword: false })}
            placeholder={
              value.passwordSet
                ? t('proxy.passwordKeepPlaceholder')
                : t('proxy.passwordPlaceholder')
            }
            autoComplete="new-password"
          />
        </div>
      </div>
    </div>
  )
}
