import type { QueryClient } from '@tanstack/react-query'
import { authApi } from '@/api/auth'
import { ApiError, setBearerToken } from '@/api/client'
import { queryKeys } from '@/hooks/useMailQueries'

type LogoutErrorKey = 'auth.logoutFailed' | 'auth.logoutOriginForbidden'

interface LogoutActions {
  setPending: (pending: boolean) => void
  onError: (key: LogoutErrorKey) => void
  onSuccess: () => void
  navigateToLogin: () => void
}

function logoutErrorKey(error: unknown): LogoutErrorKey | null {
  if (error instanceof ApiError) {
    if (error.status === 401) return null
    if (error.status < 500) {
      if (error.serverMessage === 'origin_forbidden') return 'auth.logoutOriginForbidden'
      if (error.isUnauthenticated) return null
      if (error.status === 403) return 'auth.logoutOriginForbidden'
    }
  }
  return 'auth.logoutFailed'
}

export function createLogoutHandler(queryClient: QueryClient) {
  let pending = false

  return async ({ setPending, onError, onSuccess, navigateToLogin }: LogoutActions) => {
    if (pending) return
    pending = true
    setPending(true)
    try {
      try {
        await authApi.logout()
      } catch (error) {
        const key = logoutErrorKey(error)
        if (key) {
          onError(key)
          return
        }
      }

      // Cancel pre-logout requests and bypass both the query cache and HTTP cache.
      await queryClient.cancelQueries()
      setBearerToken(null)
      const bootstrap = await authApi.bootstrap().catch(() => null)
      if (bootstrap?.logged_in === true) {
        onError('auth.logoutFailed')
        return
      }
      queryClient.clear()
      if (bootstrap?.logged_in === false) {
        queryClient.setQueryData(queryKeys.bootstrap, bootstrap)
      }
      // If bootstrap itself is unavailable, the gate re-fetches without any
      // cached signed-in state. The logout POST was already accepted.
      onSuccess()
      navigateToLogin()
    } finally {
      pending = false
      setPending(false)
    }
  }
}
