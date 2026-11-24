/**
 * URL of an external image as loaded through PMail's image proxy. The sender's
 * server then sees PMail's address, never the reader's IP, browser or cookies.
 */
export function proxiedImageUrl(emailId: number, token: string, url: string): string {
  const absolute = url.trim().startsWith('//') ? `https:${url.trim()}` : url.trim()
  return `/api/email/image/${emailId}?t=${encodeURIComponent(token)}&u=${encodeURIComponent(absolute)}`
}
