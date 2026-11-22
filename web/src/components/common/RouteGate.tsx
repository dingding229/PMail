import { useEffect } from 'react'
import { Navigate, Outlet, useLocation, useNavigate } from 'react-router'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Loader2 } from 'lucide-react'
import { setNeedBootstrapHandler, setUnauthorizedHandler } from '@/api'
import { normalizeLanguage, setAppLanguage } from '@/i18n'
import { getRawStorage } from '@/lib/storage'
import { queryKeys, useBootstrapQuery } from '@/hooks/useMailQueries'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { BrandLogo } from '@/components/common/BrandLogo'

export function RouteSkeletonFallback() {
  return (
    <div className="flex h-full flex-col bg-[var(--surface-canvas)] p-6">
      <div className="flex items-center justify-between border-b border-[var(--border-hairline)] pb-4">
        <Skeleton className="h-7 w-40" />
        <Skeleton className="h-7 w-20" />
      </div>
      <div className="mt-6 grid grid-cols-1 gap-4 sm:grid-cols-2">
        <Skeleton className="h-28 w-full rounded-[var(--radius-lg)]" />
        <Skeleton className="h-28 w-full rounded-[var(--radius-lg)]" />
        <Skeleton className="h-28 w-full rounded-[var(--radius-lg)]" />
        <Skeleton className="h-28 w-full rounded-[var(--radius-lg)]" />
      </div>
    </div>
  )
}

export function BootstrapGate() {
  const { t } = useTranslation()
  const location = useLocation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { data: bootstrap, isLoading, isError, refetch } = useBootstrapQuery()

  // Apply backend default_lang if user hasn't explicitly chosen a language
  useEffect(() => {
    if (!bootstrap?.default_lang) return
    if (!getRawStorage('lang')) {
      setAppLanguage(normalizeLanguage(bootstrap.default_lang), false)
    }
  }, [bootstrap?.default_lang])

  // Global 401 / errorNo 403 -> /login; errorNo 402 / pending-register 403 -> re-fetch /api/bootstrap
  useEffect(() => {
    setUnauthorizedHandler(() => {
      if (location.pathname !== '/login' && location.pathname !== '/register') {
        const redirect = encodeURIComponent(`${location.pathname}${location.search}`)
        navigate(`/login?redirect=${redirect}`, { replace: true })
      }
    })

    setNeedBootstrapHandler(() => {
      void queryClient
        .fetchQuery({ queryKey: queryKeys.bootstrap, staleTime: 0 })
        .then((next) => {
          const data = next as { need_register?: boolean } | undefined
          if (data?.need_register && location.pathname !== '/register') {
            navigate('/register', { replace: true })
          }
        })
        .catch(() => {
          // Ignore network error during re-bootstrap
        })
    })

    return () => {
      setUnauthorizedHandler(null)
      setNeedBootstrapHandler(null)
    }
  }, [location.pathname, location.search, navigate, queryClient])

  if (isLoading) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-4 bg-[var(--surface-canvas)]">
        <BrandLogo />
        <div className="flex items-center gap-2 text-[13px] text-[var(--ink-muted)]">
          <Loader2 className="h-4 w-4 animate-spin text-[var(--accent)]" />
          <span>{t('app.loading')}</span>
        </div>
      </div>
    )
  }

  if (isError || !bootstrap) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-4 bg-[var(--surface-canvas)] px-6 text-center">
        <BrandLogo />
        <p className="max-w-sm text-[13.5px] text-[var(--ink-secondary)]">
          {t('app.bootError')}
        </p>
        <Button variant="primary" size="sm" onClick={() => void refetch()}>
          {t('app.retry')}
        </Button>
      </div>
    )
  }

  const pathname = location.pathname

  // 1. First-time deployment: must register with registration key
  if (bootstrap.need_register) {
    if (pathname !== '/register') {
      return <Navigate to="/register" replace />
    }
    return <Outlet />
  }

  // 2. Already initialized: disallow /register
  if (pathname === '/register') {
    return <Navigate to={bootstrap.logged_in ? '/mail/inbox' : '/login'} replace />
  }

  // 3. Not logged in: redirect protected routes to /login
  if (!bootstrap.logged_in) {
    if (pathname !== '/login') {
      const redirect = encodeURIComponent(`${pathname}${location.search}`)
      return <Navigate to={`/login?redirect=${redirect}`} replace />
    }
    return <Outlet />
  }

  // 4. Logged in: redirect /login to inbox
  if (pathname === '/login') {
    return <Navigate to="/mail/inbox" replace />
  }

  return <Outlet />
}
