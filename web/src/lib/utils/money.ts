// Currency formatting for integer-cent amounts (the app stores money as int64 cents).

const usd = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD",
});

/**
 * Format an integer-cent amount as a USD currency string, e.g. 861 -> "$8.61".
 *
 * Accepts numbers, numeric strings, and null/undefined. Missing or non-finite
 * values render as "$0.00" — a real zero balance is money, not missing data, so
 * it must not collapse to an em-dash.
 */
function toCents(cents: number | string | null | undefined): number | null {
  const n = typeof cents === "string" ? Number(cents) : cents;
  return typeof n === "number" && Number.isFinite(n) ? n : null;
}

export function formatCents(cents: number | string | null | undefined): string {
  return usd.format((toCents(cents) ?? 0) / 100);
}

/**
 * Like formatCents, but renders a genuinely-absent amount (null/undefined or a
 * non-numeric value) as an em-dash. A real zero still renders as "$0.00".
 */
export function formatCentsOrDash(
  cents: number | string | null | undefined,
): string {
  const n = toCents(cents);
  return n === null ? "—" : usd.format(n / 100);
}
