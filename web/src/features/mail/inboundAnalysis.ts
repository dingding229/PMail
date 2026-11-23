import type { TFunction } from 'i18next'
import type { MailRiskFinding, MailSpamReason } from '@/api'

const SPAM_REASON_KEYS: Record<string, string> = {
  auth_dmarc_fail: 'mail.spam.reasons.auth_dmarc_fail',
  auth_dmarc_permerror: 'mail.spam.reasons.auth_dmarc_permerror',
  auth_spf_fail: 'mail.spam.reasons.auth_spf_fail',
  auth_spf_softfail: 'mail.spam.reasons.auth_spf_softfail',
  auth_spf_permerror: 'mail.spam.reasons.auth_spf_permerror',
  auth_dkim_fail: 'mail.spam.reasons.auth_dkim_fail',
  auth_dkim_permerror: 'mail.spam.reasons.auth_dkim_permerror',
  auth_dkim_unaligned: 'mail.spam.reasons.auth_dkim_unaligned',
  dnsbl_listed: 'mail.spam.reasons.dnsbl_listed',
  dnsbl_unavailable: 'mail.spam.reasons.dnsbl_unavailable',
  dnsbl_resolver_refused: 'mail.spam.reasons.dnsbl_resolver_refused',
  clamd_unavailable: 'mail.spam.reasons.clamd_unavailable',
  bayes_spam: 'mail.spam.reasons.bayes_spam',
  bayes_ham: 'mail.spam.reasons.bayes_ham',
  manual_spam: 'mail.spam.reasons.manual_spam',
  manual_ham: 'mail.spam.reasons.manual_ham',
  block_rule_match: 'mail.spam.reasons.block_rule_match',
  allow_rule_match: 'mail.spam.reasons.allow_rule_match',
  keyword_match: 'mail.spam.reasons.keyword_match',
  suspicious_links: 'mail.spam.reasons.suspicious_links',
  many_links: 'mail.spam.reasons.many_links',
  html_only: 'mail.spam.reasons.html_only',
  upper_subject: 'mail.spam.reasons.upper_subject',
  local_display_name_spoof: 'mail.spam.reasons.local_display_name_spoof',
  reply_to_domain_mismatch: 'mail.spam.reasons.reply_to_domain_mismatch',
  empty_subject_attachment: 'mail.spam.reasons.empty_subject_attachment',
  executable_attachment: 'mail.spam.reasons.executable_attachment',
  header_anomaly: 'mail.spam.reasons.header_anomaly',
  bulk_sender: 'mail.spam.reasons.bulk_sender',
  missing_auth: 'mail.spam.reasons.missing_auth',
  url_shortener: 'mail.spam.reasons.url_shortener',
}

const RISK_FINDING_KEYS: Record<string, string> = {
  shell_curl_pipe_sh: 'mail.risk.findings.shell_curl_pipe_sh',
  shell_wget_pipe_sh: 'mail.risk.findings.shell_wget_pipe_sh',
  shell_remote_exec: 'mail.risk.findings.shell_remote_exec',
  shell_destructive_rm: 'mail.risk.findings.shell_destructive_rm',
  shell_encoded_payload: 'mail.risk.findings.shell_encoded_payload',
  powershell_encoded: 'mail.risk.findings.powershell_encoded',
  js_eval_obfuscated: 'mail.risk.findings.js_eval_obfuscated',
  credential_exfiltration: 'mail.risk.findings.credential_exfiltration',
  attachment_dangerous_type: 'mail.risk.findings.attachment_dangerous_type',
  attachment_double_extension: 'mail.risk.findings.attachment_double_extension',
  attachment_macro_document: 'mail.risk.findings.attachment_macro_document',
  attachment_malware_detected: 'mail.risk.findings.attachment_malware_detected',
  attachment_script_payload: 'mail.risk.findings.attachment_script_payload',
  attachment_encrypted_archive: 'mail.risk.findings.attachment_encrypted_archive',
  attachment_type_mismatch: 'mail.risk.findings.attachment_type_mismatch',
  attachment_archive_uninspected: 'mail.risk.findings.attachment_archive_uninspected',
}

const RISK_COPY_WARNING_KEYS: Record<string, string> = {
  shell_curl_pipe_sh: 'mail.risk.copyWarnings.shell_curl_pipe_sh',
  shell_wget_pipe_sh: 'mail.risk.copyWarnings.shell_wget_pipe_sh',
  shell_remote_exec: 'mail.risk.copyWarnings.shell_remote_exec',
  shell_destructive_rm: 'mail.risk.copyWarnings.shell_destructive_rm',
  shell_encoded_payload: 'mail.risk.copyWarnings.shell_encoded_payload',
  powershell_encoded: 'mail.risk.copyWarnings.powershell_encoded',
  js_eval_obfuscated: 'mail.risk.copyWarnings.js_eval_obfuscated',
  credential_exfiltration: 'mail.risk.copyWarnings.credential_exfiltration',
}

export function formatSpamReason(reason: MailSpamReason, t: TFunction): string {
  const key = SPAM_REASON_KEYS[reason.code]
  const label = key ? t(key) : t('mail.spam.reasons.unknown')
  return reason.detail ? `${label} (${reason.detail})` : label
}

export function formatRiskFindingTitle(finding: MailRiskFinding, t: TFunction): string {
  const key = RISK_FINDING_KEYS[finding.code]
  return key ? t(key) : t('mail.risk.findings.unknown')
}

export function formatRiskCopyWarning(findings: MailRiskFinding[], t: TFunction): string {
  for (const finding of findings) {
    const key = RISK_COPY_WARNING_KEYS[finding.code]
    if (key) return t(key)
  }
  return t('mail.risk.copyWarnings.default')
}

export function matchCodeBlockRiskFindings(
  code: string,
  bodyFindings: MailRiskFinding[],
): MailRiskFinding[] {
  if (!bodyFindings.length) return []
  const normalizedCode = code.toLowerCase()
  const matched = bodyFindings.filter((f) => {
    if (!f.excerpt?.trim()) return true
    const snippet = f.excerpt.trim().toLowerCase()
    return normalizedCode.includes(snippet) || snippet.includes(normalizedCode.slice(0, 40))
  })
  return matched.length > 0 ? matched : bodyFindings
}
