import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

export function SecondFactorField({ code, recovery, onCode, onRecovery, disabled, autoFocus }: {
  code: string
  recovery: boolean
  onCode: (code: string) => void
  onRecovery?: (recovery: boolean) => void
  disabled: boolean
  autoFocus?: boolean
}) {
  const { t } = useTranslation()
  const id = useId()
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{recovery ? t('security.recoveryCode') : t('security.authenticatorCode')}</Label>
      <Input autoFocus={autoFocus} id={id} value={code} onChange={(event) => onCode(event.target.value)} required
        autoComplete={recovery ? 'off' : 'one-time-code'} inputMode={recovery ? 'text' : 'numeric'}
        maxLength={recovery ? 32 : 6} pattern={recovery ? undefined : '[0-9]{6}'} disabled={disabled} />
      {onRecovery && (
        <Button type="button" variant="ghost" size="sm" disabled={disabled} onClick={() => {
          onCode('')
          onRecovery(!recovery)
        }}>
          {recovery ? t('security.useAuthenticator') : t('security.useRecovery')}
        </Button>
      )}
    </div>
  )
}
