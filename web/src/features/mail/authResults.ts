import type { TFunction } from 'i18next'
import type { MailAuthCheckResult, MailAuthDmarcPolicy } from '@/api'

const DIRECT_REASON_KEYS: Record<string, string> = {
  no_signature: 'mail.security.authDetails.reasons.no_signature',
  signature_evaluation_error: 'mail.security.authDetails.reasons.signature_evaluation_error',
  aligned_signature: 'mail.security.authDetails.reasons.aligned_signature',
  unaligned_signature: 'mail.security.authDetails.reasons.unaligned_signature',
  signature_dns_unavailable: 'mail.security.authDetails.reasons.signature_dns_unavailable',
  invalid_signature_parameters: 'mail.security.authDetails.reasons.invalid_signature_parameters',
  signature_invalid: 'mail.security.authDetails.reasons.signature_invalid',
  no_aligned_pass: 'mail.security.authDetails.reasons.no_aligned_pass',
  aligned_spf_or_dkim: 'mail.security.authDetails.reasons.aligned_spf_or_dkim',
  upstream_authentication_unavailable:
    'mail.security.authDetails.reasons.upstream_authentication_unavailable',
  invalid_from_identity: 'mail.security.authDetails.reasons.invalid_from_identity',
  policy_dns_unavailable: 'mail.security.authDetails.reasons.policy_dns_unavailable',
  multiple_policies: 'mail.security.authDetails.reasons.multiple_policies',
  invalid_policy: 'mail.security.authDetails.reasons.invalid_policy',
  no_policy: 'mail.security.authDetails.reasons.no_policy',
  no_valid_mail_from: 'mail.security.authDetails.reasons.no_valid_mail_from',
  no_valid_helo: 'mail.security.authDetails.reasons.no_valid_helo',
}

const SPF_BASE_REASON_KEYS: Record<string, string> = {
  dns_unavailable: 'mail.security.authDetails.reasons.dns_unavailable',
  void_lookup_limit: 'mail.security.authDetails.reasons.void_lookup_limit',
  lookup_limit: 'mail.security.authDetails.reasons.lookup_limit',
  multiple_records: 'mail.security.authDetails.reasons.multiple_records',
  no_record: 'mail.security.authDetails.reasons.no_record',
  record_too_complex: 'mail.security.authDetails.reasons.record_too_complex',
  invalid_redirect: 'mail.security.authDetails.reasons.invalid_redirect',
  redirect_no_record: 'mail.security.authDetails.reasons.redirect_no_record',
  no_match: 'mail.security.authDetails.reasons.no_match',
  invalid_mechanism: 'mail.security.authDetails.reasons.invalid_mechanism',
  include_no_record: 'mail.security.authDetails.reasons.include_no_record',
}

export function formatAuthResultLabel(result: MailAuthCheckResult, t: TFunction): string {
  const key = `mail.security.authDetails.results.${result}`
  const translated = t(key, { defaultValue: '' })
  return translated || result
}

export function formatDmarcPolicyLabel(policy: MailAuthDmarcPolicy, t: TFunction): string {
  const key = `mail.security.authDetails.policies.${policy}`
  const translated = t(key, { defaultValue: '' })
  return translated || policy
}

export function formatAuthReason(reason: string | undefined, t: TFunction): string {
  if (!reason) return ''
  const trimmed = reason.trim()
  if (!trimmed) return ''

  const directKey = DIRECT_REASON_KEYS[trimmed]
  if (directKey) {
    return t(directKey)
  }

  let prefix = ''
  let rest = trimmed
  if (trimmed.startsWith('mail_from_')) {
    prefix = t('mail.security.authDetails.reasons.spfPrefixMailFrom')
    rest = trimmed.slice('mail_from_'.length)
  } else if (trimmed.startsWith('helo_')) {
    prefix = t('mail.security.authDetails.reasons.spfPrefixHelo')
    rest = trimmed.slice('helo_'.length)
  }

  if (rest.startsWith('matched_') && rest.length > 'matched_'.length) {
    const mechanism = rest.slice('matched_'.length)
    return `${prefix}${t('mail.security.authDetails.reasons.matched_mechanism', { mechanism })}`
  }

  const spfBaseKey = SPF_BASE_REASON_KEYS[rest]
  if (spfBaseKey) {
    return `${prefix}${t(spfBaseKey)}`
  }

  // Fallback to raw machine code for unknown future backend reasons
  return trimmed
}

export function getAuthBadgeVariant(
  result: MailAuthCheckResult,
): 'success' | 'danger' | 'warning' | 'default' {
  switch (result) {
    case 'pass':
      return 'success'
    case 'fail':
      return 'danger'
    case 'softfail':
    case 'temperror':
    case 'permerror':
      return 'warning'
    default:
      return 'default'
  }
}
