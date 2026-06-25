/**
 * Garapon prize-odds math, shared by the admin manager and the create/edit form.
 *
 * Appearance rates are relative weights (they need not total 100), so a prize's
 * displayed odds are its weight over the sum of all positive weights. Negative
 * weights are floored at 0 (the server sanitizes them the same way).
 */

/** Sum of the positive prize weights — the denominator for normalized odds. */
export function rateTotal(prizes: { rate: number }[]): number {
  return prizes.reduce((sum, p) => sum + (p.rate > 0 ? p.rate : 0), 0)
}

/** A weight's share of `total` as a percent string (e.g. "33.3%"), or "—" when total ≤ 0. */
export function ratePct(rate: number, total: number): string {
  if (total <= 0) return '—'
  return `${((Math.max(0, rate) / total) * 100).toFixed(1)}%`
}
