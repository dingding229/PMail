import { useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Loader2, RefreshCw, Wrench } from 'lucide-react'
import { adminBackupApi, setMaintenanceHandler } from '@/api'
import { Button } from '@/components/ui/button'

export interface MaintenanceGateProps {
  children: ReactNode
  /** Optional initial state override (primarily for tests) */
  initialMaintenance?: boolean
}

export function MaintenanceGate({
  children,
  initialMaintenance = false,
}: MaintenanceGateProps) {
  const { t } = useTranslation()
  const [inMaintenance, setInMaintenance] = useState(initialMaintenance)
  const [checking, setChecking] = useState(false)

  useEffect(() => {
    setMaintenanceHandler((active) => {
      setInMaintenance(active)
    })
    return () => setMaintenanceHandler(null)
  }, [])

  const checkRecovery = async () => {
    setChecking(true)
    try {
      await adminBackupApi.ping()
      setInMaintenance(false)
      window.location.reload()
    } catch {
      // Still under maintenance
    } finally {
      setChecking(false)
    }
  }

  useEffect(() => {
    if (!inMaintenance) return
    const timer = window.setInterval(() => {
      void adminBackupApi
        .ping()
        .then(() => {
          setInMaintenance(false)
          window.location.reload()
        })
        .catch(() => {
          // keep waiting
        })
    }, 5_000)
    return () => window.clearInterval(timer)
  }, [inMaintenance])

  if (inMaintenance) {
    return (
      <main
        role="alert"
        aria-live="assertive"
        className="flex min-h-screen items-center justify-center bg-[var(--surface-canvas)] px-4 py-12 text-[var(--ink-primary)]"
      >
        <div className="w-full max-w-md rounded-[var(--radius-lg)] border border-[var(--border-subtle)] bg-[var(--surface-elevated)] p-8 text-center shadow-[var(--shadow-md)]">
          <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-2xl bg-[var(--warning-soft)] text-[var(--warning)]">
            <Wrench className="h-6 w-6" />
          </div>
          <h1 className="mt-4 text-[18px] font-semibold tracking-tight">
            {t('common.maintenance.title')}
          </h1>
          <p className="mt-2 text-[13.5px] leading-relaxed text-[var(--ink-secondary)]">
            {t('common.maintenance.desc')}
          </p>
          <div className="mt-6 flex justify-center">
            <Button
              type="button"
              variant="secondary"
              size="sm"
              disabled={checking}
              onClick={() => void checkRecovery()}
            >
              {checking ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <RefreshCw className="h-3.5 w-3.5" />
              )}
              {t('common.maintenance.retry')}
            </Button>
          </div>
        </div>
      </main>
    )
  }

  return <>{children}</>
}
