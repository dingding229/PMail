/**
 * Reserved interfaces for C9 backend integration:
 * - CID inline images (`InlineImageDraft`, `InlineImageAdapter`)
 * - Per-address rich-text signatures (`SignatureItem`, `SignatureAdapter`)
 */

export interface InlineImageDraft {
  cid: string
  filename: string
  contentType: string
  size: number
  dataUrl: string
}

export interface InlineImageAdapter {
  createInlineImage: (file: File) => Promise<InlineImageDraft>
}

export interface SignatureItem {
  id: string
  /** Mailbox address this signature belongs to, or '*' for all addresses */
  address: string
  label: string
  html: string
  enabled: boolean
  isDefault?: boolean
}

export interface SignatureAdapter {
  listSignatures: () => Promise<SignatureItem[]>
}

export const SIGNATURE_BLOCK_ATTR = 'data-pmail-signature'

/**
 * Default preview signatures shown in MSW mock mode or before C9 signature API is deployed.
 */
export const MOCK_SIGNATURES: SignatureItem[] = [
  {
    id: 'sig-work',
    address: 'admin@pmail.dev',
    label: '工作签名（默认）',
    enabled: true,
    isDefault: true,
    html: '<p style="margin:0;font-weight:600;">林深 · PMail Pro</p><p style="margin:2px 0 0;font-size:12px;color:#64748b;">管理员 · admin@pmail.dev · <a href="https://pmail.dev">pmail.dev</a></p>',
  },
  {
    id: 'sig-minimal',
    address: '*',
    label: '简洁签名',
    enabled: true,
    html: '<p style="margin:0;font-size:13px;color:#64748b;">此致，林深</p>',
  },
]
