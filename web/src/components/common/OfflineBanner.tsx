import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { WifiOff } from 'lucide-react'

export function OfflineBanner() {
  const { t } = useTranslation()
  const [offline, setOffline] = useState(() =>
    typeof navigator !== 'undefined' ? !navigator.onLine : false,
  )

  useEffect(() => {
    const onOffline = () => setOffline(true)
    const onOnline = () => {
      setOffline(false)
      toast.success(t('common.onlineRestored'))
    }
    window.addEventListener('offline', onOffline)
    window.addEventListener('online', onOnline)
    return () => {
      window.removeEventListener('offline', onOffline)
      window.removeEventListener('online', onOnline)
    }
  }, [t])

  if (!offline) return null

  return (
    <div
      role="status"
      aria-live="polite"
      className="fixed bottom-4 left-1/2 z-50 flex -translate-x-1/2 items-center gap-2 rounded-full border border-[var(--warning)]/35 bg-[var(--surface-elevated)] px-4 py-2 text-[12.5px] font-medium text-[var(--warning)] shadow-[var(--shadow-md)]"
    >
      <WifiOff className="h-3.5 w-3.5 shrink-0" />
      <span>{t('common.offlineBanner')}</span>
    </div>
  )
}
