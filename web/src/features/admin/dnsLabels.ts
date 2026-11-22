import type { TFunction } from 'i18next'
import type { DnsRecord } from '@/api'

/**
 * Known backend DNS `purpose` enum values (from server/services/domain/guide.go).
 * Any unknown purpose falls back to `other` ("其他记录" / "Other Record") rather than raw enum text.
 */
export const DNS_PURPOSES = [
  'receiving',
  'mail_host',
  'web',
  'authentication',
  'reverse_dns',
  'client_configuration',
  'other',
] as const

export type DnsPurposeKey = (typeof DNS_PURPOSES)[number]

/**
 * Known backend DNS record `key` values (from server/services/domain/guide.go).
 */
export const DNS_RECORD_KEYS = [
  'mx',
  'mail_a',
  'mail_aaaa',
  'web_a',
  'web_aaaa',
  'spf',
  'dkim',
  'dmarc',
  'ptr',
  'imap_alias',
  'pop_alias',
  'autoconfig',
  'autodiscover',
  'imaps',
  'submission',
  'pop3s',
] as const

export type DnsRecordKey = (typeof DNS_RECORD_KEYS)[number]

const RECORD_KEY_ALIASES: Record<string, DnsRecordKey> = {
  a: 'mail_a',
  aaaa: 'mail_aaaa',
}

const DNS_RECORD_KEY_SET = new Set<string>(DNS_RECORD_KEYS)
const DNS_PURPOSE_SET = new Set<string>(DNS_PURPOSES)

/**
 * Returns a localized human-readable title for a DNS record.
 * Priority: exact `record.key` i18n -> `record.purpose` i18n -> generic "其他记录" / "Other Record".
 * Never exposes raw internal enum strings like `receiving` or `client_configuration`.
 */
export function getDnsRecordLabel(
  record: Pick<DnsRecord, 'key' | 'purpose'>,
  t: TFunction,
): string {
  const rawKey = record.key?.trim().toLowerCase() ?? ''
  const canonicalKey = RECORD_KEY_ALIASES[rawKey] ?? rawKey
  if (canonicalKey && DNS_RECORD_KEY_SET.has(canonicalKey)) {
    return t(`admin.dnsWizard.recordKeys.${canonicalKey as DnsRecordKey}`)
  }

  const rawPurpose = record.purpose?.trim().toLowerCase() ?? ''
  if (rawPurpose && rawPurpose !== 'other' && DNS_PURPOSE_SET.has(rawPurpose)) {
    return t(`admin.dnsWizard.purposes.${rawPurpose as DnsPurposeKey}`)
  }

  return t('admin.dnsWizard.purposes.other')
}

/**
 * Returns a localized category label for a DNS record's `purpose` enum.
 * Unknown purposes always fall back to "其他记录" / "Other Record".
 */
export function getDnsPurposeLabel(
  purpose: string | undefined,
  t: TFunction,
): string {
  const normalized = purpose?.trim().toLowerCase() ?? ''
  if (normalized && DNS_PURPOSE_SET.has(normalized)) {
    return t(`admin.dnsWizard.purposes.${normalized as DnsPurposeKey}`)
  }
  return t('admin.dnsWizard.purposes.other')
}
