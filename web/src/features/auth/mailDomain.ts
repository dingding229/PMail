import type { BootstrapResponse } from '@/api/types'

export const DOMAIN_PATTERN = /^(?=.{3,253}$)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]{0,61}[a-z0-9]$/

/** Guess the mail domain from the address bar: mail.example.com -> example.com. */
export function guessMailDomain(hostname: string): string {
  const host = hostname.trim().toLowerCase().replace(/\.$/, '')
  if (!host || host === 'localhost' || /^[\d.]+$/.test(host) || host.includes(':')) return ''
  const labels = host.split('.')
  if (labels.length > 2 && ['mail', 'webmail', 'email', 'pmail', 'pm'].includes(labels[0] ?? '')) {
    return labels.slice(1).join('.')
  }
  return host
}

/** Poll the normal listener, not an HTML/proxy response or stale setup bootstrap. */
export async function waitForDomainSetup(
  bootstrap: (signal: AbortSignal) => Promise<BootstrapResponse>,
  { timeoutMs = 60_000, intervalMs = 1_000, requestMs = 4_000 } = {},
): Promise<BootstrapResponse | undefined> {
  const deadline = performance.now() + timeoutMs
  while (performance.now() < deadline) {
    const remaining = Math.ceil(deadline - performance.now())
    if (remaining <= 0) break
    const controller = new AbortController()
    const timer = setTimeout(() => controller.abort(), Math.min(requestMs, remaining))
    try {
      const next = await bootstrap(controller.signal)
      if (next?.need_domain !== true && typeof next?.need_register === 'boolean' &&
          typeof next.primary_domain === 'string' && next.primary_domain.trim()) return next
    } catch {
      // Listener handoffs, proxy errors and request timeouts are retryable.
    } finally {
      clearTimeout(timer)
    }
    const delay = Math.min(intervalMs, deadline - performance.now())
    if (delay > 0) await new Promise((resolve) => setTimeout(resolve, delay))
  }
}
