import { useEffect, useId, useRef, type ClipboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { KeyRound, Smartphone } from 'lucide-react'
import { cn } from '@/lib/cn'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

export function SecondFactorField({
  code,
  recovery,
  onCode,
  onRecovery,
  disabled,
  autoFocus,
}: {
  code: string
  recovery: boolean
  onCode: (code: string) => void
  onRecovery?: (recovery: boolean) => void
  disabled: boolean
  autoFocus?: boolean
}) {
  const { t } = useTranslation()
  const id = useId()
  const inputRef = useRef<HTMLInputElement | null>(null)

  useEffect(() => {
    if (autoFocus) {
      inputRef.current?.focus()
    }
  }, [autoFocus, recovery])

  const sanitizeTotp = (raw: string) => raw.replace(/\D/g, '').slice(0, 6)

  const handleChange = (raw: string) => {
    onCode(recovery ? raw : sanitizeTotp(raw))
  }

  const handlePaste = (event: ClipboardEvent<HTMLInputElement>) => {
    if (recovery) return
    event.preventDefault()
    const pasted = event.clipboardData.getData('text')
    onCode(sanitizeTotp(pasted))
  }

  return (
    <div className="space-y-2.5">
      <div className="flex items-center justify-between gap-2">
        <Label htmlFor={id}>
          {recovery ? t('security.recoveryCode') : t('security.authenticatorCode')}
        </Label>
        {onRecovery && (
          <button
            type="button"
            disabled={disabled}
            onClick={() => {
              onCode('')
              onRecovery(!recovery)
            }}
            className="inline-flex items-center gap-1 text-[12px] font-medium text-[var(--accent-ink)] transition-colors hover:underline disabled:opacity-50"
          >
            {recovery ? (
              <Smartphone className="h-3.5 w-3.5" aria-hidden="true" />
            ) : (
              <KeyRound className="h-3.5 w-3.5" aria-hidden="true" />
            )}
            {recovery ? t('security.useAuthenticator') : t('security.useRecovery')}
          </button>
        )}
      </div>

      <Input
        ref={inputRef}
        autoFocus={autoFocus}
        id={id}
        value={code}
        onChange={(event) => handleChange(event.target.value)}
        onPaste={handlePaste}
        required
        autoComplete={recovery ? 'off' : 'one-time-code'}
        inputMode={recovery ? 'text' : 'numeric'}
        maxLength={recovery ? 32 : 6}
        pattern={recovery ? undefined : '[0-9]{6}'}
        placeholder={recovery ? t('security.recoveryPlaceholder') : '000000'}
        disabled={disabled}
        className={cn(
          'font-mono',
          recovery
            ? 'h-10 text-[14px] tracking-wider uppercase'
            : 'h-11 text-center text-[20px] font-semibold tracking-[0.42em] tabular-nums placeholder:tracking-[0.42em] placeholder:font-normal placeholder:opacity-40',
        )}
      />

      {!recovery && (
        <div className="space-y-1.5">
          <div className="grid grid-cols-6 gap-1.5" aria-hidden="true">
            {Array.from({ length: 6 }).map((_, idx) => {
              const filled = idx < code.length
              const active = idx === code.length && code.length < 6
              return (
                <span
                  key={idx}
                  className={cn(
                    'h-1 rounded-full transition-colors',
                    filled
                      ? 'bg-[var(--accent)]'
                      : active
                        ? 'bg-[var(--accent)]/40'
                        : 'bg-[var(--border-subtle)]',
                  )}
                />
              )
            })}
          </div>
          <p className="text-[11.5px] text-[var(--ink-muted)]">
            {t('security.codeInputHint')}
          </p>
        </div>
      )}
    </div>
  )
}
