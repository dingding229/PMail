import type { TFunction } from 'i18next'
import type { MailRiskFinding, MailSpamReason } from '@/api'

const SPAM_REASON_KEYS: Record<string, string> = {
  auth_dmarc_fail: 'mail.spam.reasons.auth_dmarc_fail',
  auth_spf_fail: 'mail.spam.reasons.auth_spf_fail',
  auth_dkim_fail: 'mail.spam.reasons.auth_dkim_fail',
  dnsbl_listed: 'mail.spam.reasons.dnsbl_listed',
  bayes_spam: 'mail.spam.reasons.bayes_spam',
  bayes_ham: 'mail.spam.reasons.bayes_ham',
  block_rule_match: 'mail.spam.reasons.block_rule_match',
  allow_rule_match: 'mail.spam.reasons.allow_rule_match',
  keyword_match: 'mail.spam.reasons.keyword_match',
  suspicious_links: 'mail.spam.reasons.suspicious_links',
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
  const label = key ? t(key) : reason.code
  return reason.detail ? `${label} (${reason.detail})` : label
}

export function formatRiskFindingTitle(finding: MailRiskFinding, t: TFunction): string {
  const key = RISK_FINDING_KEYS[finding.code]
  return key ? t(key) : finding.code
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
