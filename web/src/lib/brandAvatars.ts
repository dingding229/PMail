import type { MailAuthStatus, MailRiskAssessment, MailRiskSummary } from '@/api'
import {
  BRAND_ICON_CATALOG,
  USED_ICON_LIBRARIES,
  type BrandIconEntry,
} from '@/generated/brandIcons'

export type { BrandIconEntry }
export { BRAND_ICON_CATALOG, USED_ICON_LIBRARIES }

/**
 * Extract the normalized lowercase domain from an email address or "Name <user@domain>" header.
 */
export function extractEmailDomain(rawEmail?: string | null): string {
  if (!rawEmail) return ''
  let candidate = rawEmail.trim()
  const angleMatch = candidate.match(/<([^<>]+)>/)
  if (angleMatch?.[1]) {
    candidate = angleMatch[1].trim()
  }
  const atIndex = candidate.lastIndexOf('@')
  if (atIndex < 0 || atIndex === candidate.length - 1) return ''
  return candidate
    .slice(atIndex + 1)
    .trim()
    .replace(/\.+$/, '')
    .toLowerCase()
}

// Pre-index all (domain -> BrandIconEntry) pairs sorted by domain length descending
// so more specific domains (e.g. cloud.google.com, gemini.google.com) match before google.com.
const SORTED_DOMAIN_RULES: readonly { domain: string; brand: BrandIconEntry }[] =
  BRAND_ICON_CATALOG.flatMap((brand) =>
    brand.domains.map((d) => ({
      domain: d.trim().toLowerCase(),
      brand,
    })),
  ).sort((a, b) => b.domain.length - a.domain.length)

/**
 * Match a sender email address against the curated brand catalog.
 * Matches exact domain or subdomain (`host === domain || host.endsWith('.' + domain)`).
 * Rejects spoofed lookalikes such as `evilgithub.com`.
 */
/**
 * Free mailbox providers: anyone can own user@<domain>, and DMARC passes for
 * all of them, so the exact host must never get a brand avatar. Subdomains
 * (weixin.qq.com, music.163.com) are operated by the company itself.
 */
export const PUBLIC_MAILBOX_DOMAINS: readonly string[] = [
  'gmail.com', 'googlemail.com', 'outlook.com', 'hotmail.com', 'live.com', 'msn.com',
  'icloud.com', 'me.com', 'mac.com', 'yahoo.com', 'ymail.com', 'aol.com',
  'proton.me', 'protonmail.com', 'pm.me', 'fastmail.com', 'gmx.com', 'gmx.net', 'mail.com',
  'zoho.com', 'yandex.com', 'yandex.ru', 'tutanota.com', 'tuta.io',
  'qq.com', 'foxmail.com', '163.com', '126.com', 'yeah.net', 'sina.com', 'sina.cn',
  'sina.com.cn', 'sohu.com', 'aliyun.com', '139.com', '189.cn', 'wo.cn',
]

/**
 * Shared sending platforms and per-customer subdomains: any customer can send
 * from <anything>.<domain>, so the domain and all of its subdomains are excluded.
 */
export const SHARED_SENDER_DOMAINS: readonly string[] = [
  'sendgrid.net', 'mailgun.org', 'amazonses.com', 'amazonaws.com', 'firebaseapp.com',
  'atlassian.net', 'shopifyemail.com', 'mcsv.net', 'mcdlv.net',
]

export function isUnbrandableHost(host: string): boolean {
  const h = host.toLowerCase()
  return PUBLIC_MAILBOX_DOMAINS.includes(h) || SHARED_SENDER_DOMAINS.some((d) => h === d || h.endsWith(`.${d}`))
}

export function matchBrandByEmail(email?: string | null): BrandIconEntry | null {
  const host = extractEmailDomain(email)
  if (!host) return null
  if (isUnbrandableHost(host)) return null

  for (const rule of SORTED_DOMAIN_RULES) {
    if (host === rule.domain || host.endsWith(`.${rule.domain}`)) {
      return rule.brand
    }
  }
  return null
}

/**
 * Anti-spoofing gate:
 * Brand avatars may ONLY be shown when:
 *   1. `auth_status === 'pass'` (DMARC alignment passed), AND
 *   2. `risk` contains no medium or high risk.
 */
export function isSenderAuthVerified(
  authStatus?: MailAuthStatus | string | null,
  risk?: MailRiskAssessment | MailRiskSummary | null,
): boolean {
  if (authStatus !== 'pass') return false
  if (risk) {
    if (risk.level === 'medium' || risk.level === 'high') return false
    if (
      'findings' in risk &&
      Array.isArray(risk.findings) &&
      risk.findings.some((f) => f.severity === 'medium' || f.severity === 'high')
    ) {
      return false
    }
  }
  return true
}

export interface ThirdPartyIconAttribution {
  id: string
  name: string
  pkg: string
  license: string
  url: string
}

const ICON_LIBRARY_META: Record<string, ThirdPartyIconAttribution> = {
  logos: {
    id: 'logos',
    name: 'SVG Porn / Iconify Logos',
    pkg: '@iconify-json/logos@1.2.20',
    license: 'CC0-1.0',
    url: 'https://github.com/gilbarbara/logos',
  },
  lobehub: {
    id: 'lobehub',
    name: 'LobeHub Icons',
    pkg: '@lobehub/icons-static-svg@1.106.0',
    license: 'MIT',
    url: 'https://github.com/lobehub/lobe-icons',
  },
  'simple-icons': {
    id: 'simple-icons',
    name: 'Simple Icons',
    pkg: '@iconify-json/simple-icons@1.2.103',
    license: 'CC0-1.0',
    url: 'https://simpleicons.org/',
  },
  ri: {
    id: 'ri',
    name: 'Remix Icon',
    pkg: '@iconify-json/ri@1.2.10',
    license: 'Apache-2.0',
    url: 'https://remixicon.com/',
  },
  'fa6-brands': {
    id: 'fa6-brands',
    name: 'Font Awesome 6 Brands',
    pkg: '@iconify-json/fa6-brands@1.2.6',
    license: 'CC-BY-4.0',
    url: 'https://fontawesome.com/',
  },
}

export function getUsedIconAttributions(): ThirdPartyIconAttribution[] {
  return USED_ICON_LIBRARIES.map((id) => ICON_LIBRARY_META[id]).filter(
    (item): item is ThirdPartyIconAttribution => Boolean(item),
  )
}
