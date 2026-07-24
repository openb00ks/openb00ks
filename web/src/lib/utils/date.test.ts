import { describe, expect, it } from "vitest";
import { formatDateTime, formatLongDate, formatShortDate, formatShortDateYear } from "./date";

// Local-time inputs (no trailing Z) so rendering is timezone-independent.
const noon = "2026-01-05T12:00:00";

describe("date display formatters", () => {
  it("formatShortDate: month + day", () => {
    expect(formatShortDate(noon)).toBe("Jan 5");
  });

  it("formatShortDateYear: month + day + year", () => {
    expect(formatShortDateYear(noon)).toBe("Jan 5, 2026");
  });

  it("formatLongDate: full month + year", () => {
    expect(formatLongDate(noon)).toBe("January 5, 2026");
  });

  it("formatDateTime: includes date and time", () => {
    const out = formatDateTime(noon);
    expect(out).toMatch(/Jan 5, 2026/);
    expect(out).toMatch(/12:00/);
  });

  it("renders empty/missing as an em-dash", () => {
    expect(formatShortDate("")).toBe("—");
    expect(formatShortDate(null)).toBe("—");
    expect(formatShortDate(undefined)).toBe("—");
  });

  it("returns an unparseable value unchanged", () => {
    expect(formatShortDate("not-a-date")).toBe("not-a-date");
  });
});
