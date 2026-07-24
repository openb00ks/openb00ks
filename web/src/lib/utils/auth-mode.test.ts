import { describe, expect, it } from "vitest";
import { parsePublicRegistrationEnabled } from "./auth-mode";

describe("parsePublicRegistrationEnabled", () => {
  it("defaults to false for empty values", () => {
    expect(parsePublicRegistrationEnabled("")).toBe(false);
    expect(parsePublicRegistrationEnabled(undefined)).toBe(false);
  });

  it("accepts common truthy values", () => {
    expect(parsePublicRegistrationEnabled("true")).toBe(true);
    expect(parsePublicRegistrationEnabled("1")).toBe(true);
    expect(parsePublicRegistrationEnabled(" yes ")).toBe(true);
    expect(parsePublicRegistrationEnabled("ON")).toBe(true);
  });

  it("rejects other values", () => {
    expect(parsePublicRegistrationEnabled("false")).toBe(false);
    expect(parsePublicRegistrationEnabled("0")).toBe(false);
    expect(parsePublicRegistrationEnabled("oss")).toBe(false);
  });
});
