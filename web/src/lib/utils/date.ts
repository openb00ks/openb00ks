export function formatLocalDate(dateValue: Date) {
  const year = dateValue.getFullYear();
  const month = String(dateValue.getMonth() + 1).padStart(2, "0");
  const day = String(dateValue.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

export function parseLocalDate(value: string) {
  const [year, month, day] = value.split("-").map(Number);
  if (!year || !month || !day) {
    return null;
  }
  return new Date(year, month - 1, day);
}

export function todayLocalDate() {
  return formatLocalDate(new Date());
}

export function localDateToUTCDate(value: string) {
  const parsed = parseLocalDate(value);
  if (!parsed) {
    return value;
  }
  return parsed.toISOString().slice(0, 10);
}

// --- Display formatting (human-facing) -------------------------------------
// Empty/missing renders as an em-dash; an unparseable value is returned as-is.

function formatDisplay(
  value: string | null | undefined,
  render: (d: Date) => string,
): string {
  if (!value) {
    return "—";
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return render(parsed);
}

/** e.g. "Jan 5" */
export function formatShortDate(value: string | null | undefined) {
  return formatDisplay(value, (d) => d.toLocaleDateString("en-US", { month: "short", day: "numeric" }));
}

/** e.g. "Jan 5, 2026" */
export function formatShortDateYear(value: string | null | undefined) {
  return formatDisplay(value, (d) =>
    d.toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" }));
}

/** e.g. "January 5, 2026" */
export function formatLongDate(value: string | null | undefined) {
  return formatDisplay(value, (d) =>
    d.toLocaleDateString("en-US", { year: "numeric", month: "long", day: "numeric" }));
}

/** e.g. "Jan 5, 2026, 3:04 PM" */
export function formatDateTime(value: string | null | undefined) {
  return formatDisplay(value, (d) =>
    d.toLocaleString("en-US", { month: "short", day: "numeric", year: "numeric", hour: "numeric", minute: "2-digit" }));
}

export function utcDateToLocalDate(value: string) {
  if (!value) {
    return value;
  }
  const parsed = new Date(`${value}T00:00:00Z`);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return formatLocalDate(parsed);
}
