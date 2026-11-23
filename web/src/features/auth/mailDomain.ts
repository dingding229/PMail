export const DOMAIN_PATTERN = /^(?=.{3,253}$)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]{1,62}$/

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
