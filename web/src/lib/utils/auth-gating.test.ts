import { describe, expect, it } from "vitest";
import { shouldRenderRouteContent } from "./auth-gating";

describe("shouldRenderRouteContent", () => {
  it("hides protected routes until the session is hydrated", () => {
    expect(shouldRenderRouteContent(false, "/", false)).toBe(false);
  });

  it("allows login and setup routes before session hydration", () => {
    expect(shouldRenderRouteContent(false, "/login", false)).toBe(true);
    expect(shouldRenderRouteContent(false, "/setup", false)).toBe(true);
  });

  it("shows public routes after session hydration even without auth", () => {
    expect(shouldRenderRouteContent(false, "/login", false)).toBe(true);
    expect(shouldRenderRouteContent(false, "/setup", false)).toBe(true);
  });

  it("shows protected routes only with a session", () => {
    expect(shouldRenderRouteContent(false, "/", false)).toBe(false);
    expect(shouldRenderRouteContent(true, "/", false)).toBe(true);
  });
});
