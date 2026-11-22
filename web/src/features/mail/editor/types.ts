/**
 * Editor inline image & signature types aligned with C9 contract.
 */

export interface InlineImageDraft {
  cid: string
  filename: string
  contentType: string
  size: number
  url: string
}

export const SIGNATURE_BLOCK_ATTR = 'data-pmail-signature'
