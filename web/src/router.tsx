import { lazy, Suspense, type ReactNode } from 'react'
import { createBrowserRouter, Navigate } from 'react-router'
import { BootstrapGate, RouteSkeletonFallback } from '@/components/common/RouteGate'
import { ErrorBoundary } from '@/components/common/ErrorBoundary'

const MailWorkspace = lazy(() =>
  import('@/features/mail/MailWorkspace').then((m) => ({ default: m.MailWorkspace })),
)
const LoginPage = lazy(() =>
  import('@/features/auth/LoginPage').then((m) => ({ default: m.LoginPage })),
)
const RegisterPage = lazy(() =>
  import('@/features/auth/RegisterPage').then((m) => ({ default: m.RegisterPage })),
)
const SettingsPage = lazy(() => import('@/features/settings/SettingsPage'))
const AdminPage = lazy(() => import('@/features/admin/AdminPage'))

function wrapLazyRoute(key: string, node: ReactNode) {
  return (
    <ErrorBoundary resetKey={key}>
      <Suspense fallback={<RouteSkeletonFallback />}>{node}</Suspense>
    </ErrorBoundary>
  )
}

export const router = createBrowserRouter([
  {
    element: <BootstrapGate />,
    children: [
      {
        path: '/',
        element: <Navigate to="/mail/inbox" replace />,
      },
      {
        path: '/register',
        element: wrapLazyRoute('register', <RegisterPage />),
      },
      {
        path: '/login',
        element: wrapLazyRoute('login', <LoginPage />),
      },
      {
        path: '/mail',
        element: <Navigate to="/mail/inbox" replace />,
      },
      {
        path: '/mail/:folder',
        element: wrapLazyRoute('mail', <MailWorkspace />),
      },
      {
        path: '/settings',
        element: wrapLazyRoute('settings', <SettingsPage />),
      },
      {
        path: '/settings/:tab',
        element: wrapLazyRoute('settings-tab', <SettingsPage />),
      },
      {
        path: '/admin',
        element: wrapLazyRoute('admin', <AdminPage />),
      },
      {
        path: '/admin/:tab',
        element: wrapLazyRoute('admin-tab', <AdminPage />),
      },
      {
        path: '*',
        element: <Navigate to="/mail/inbox" replace />,
      },
    ],
  },
])
