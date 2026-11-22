/**
 * Idempotent route & modal chunk prefetchers.
 * Triggered on hover/focus of primary navigation controls so lazy routes and
 * the TipTap compose modal open with near-zero perceived latency.
 */
const prefetched = new Set<string>()

function runOnce(key: string, loader: () => Promise<unknown>) {
  if (prefetched.has(key)) return
  prefetched.add(key)
  void loader().catch(() => {
    prefetched.delete(key)
  })
}

export function prefetchComposeModal() {
  runOnce('compose', () => import('@/features/mail/ComposeModal'))
}

export function prefetchSettingsPage() {
  runOnce('settings', () => import('@/features/settings/SettingsPage'))
}

export function prefetchAdminPage() {
  runOnce('admin', () => import('@/features/admin/AdminPage'))
}

export function prefetchMailWorkspace() {
  runOnce('mail', () => import('@/features/mail/MailWorkspace'))
}
