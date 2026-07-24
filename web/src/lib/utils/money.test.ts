import { describe, expect, it } from "vitest";
import { formatCents, formatCentsOrDash } from "./money";

describe("formatCents", () => {
  it("formats integer cents as USD", () => {
    expect(formatCents(861)).toBe("$8.61");
    expect(formatCents(100)).toBe("$1.00");
    expect(formatCents(123456)).toBe("$1,234.56");
  });

  it("renders a real zero as $0.00, not an em-dash", () => {
    expect(formatCents(0)).toBe("$0.00");
  });

  it("treats null/undefined/non-finite as $0.00", () => {
    expect(formatCents(null)).toBe("$0.00");
    expect(formatCents(undefined)).toBe("$0.00");
    expect(formatCents(Number.NaN)).toBe("$0.00");
    expect(formatCents("abc")).toBe("$0.00");
  });

  it("accepts numeric strings", () => {
    expect(formatCents("861")).toBe("$8.61");
  });

  it("formats negative amounts", () => {
    expect(formatCents(-500)).toBe("-$5.00");
  });
});

describe("formatCentsOrDash", () => {
  it("renders a real zero as $0.00", () => {
    expect(formatCentsOrDash(0)).toBe("$0.00");
  });

  it("renders genuinely-missing values as an em-dash", () => {
    expect(formatCentsOrDash(null)).toBe("—");
    expect(formatCentsOrDash(undefined)).toBe("—");
    expect(formatCentsOrDash("abc")).toBe("—");
  });

  it("formats present amounts", () => {
    expect(formatCentsOrDash(861)).toBe("$8.61");
  });
});
