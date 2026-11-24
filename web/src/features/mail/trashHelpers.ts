/**
 * Computes remaining full/rounded-up days until a Trash item is purged.
 * Returns null if `purgeAt` is null/undefined (retention disabled).
 */
export function computePurgeDaysLeft(
  purgeAt: number | null | undefined,
  nowMs: number = Date.now(),
): number | null {
  if (purgeAt == null || !Number.isFinite(purgeAt)) return null
  const diffMs = purgeAt - nowMs
  if (diffMs <= 0) return 0
  return Math.ceil(diffMs / (24 * 60 * 60 * 1000))
}
