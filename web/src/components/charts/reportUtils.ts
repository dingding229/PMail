export function computePassRate(pass: number, total: number, explicitRate?: number): number {
  if (typeof explicitRate === 'number' && Number.isFinite(explicitRate) && total > 0) {
    return explicitRate
  }
  if (total <= 0) return 0
  return Math.max(0, Math.min(100, (pass / total) * 100))
}
