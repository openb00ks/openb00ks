import { describe, expect, it } from "vitest";
import { decideSetupRoute, isUnauthenticatedPublicRoute } from "./setup";

describe("decideSetupRoute", () => {
  it("returns /setup when required", () => {
    expect(decideSetupRoute(true)).toBe("/setup");
  });

  it("returns /login when not required", () => {
    expect(decideSetupRoute(false)).toBe("/login");
  });
});

describe("isUnauthenticatedPublicRoute", () => {
  it("does not treat the app root as public for fresh unauthenticated installs", () => {
    expect(isUnauthenticatedPublicRoute("/", false)).toBe(false);
  });

  it("allows login and setup while unauthenticated", () => {
    expect(isUnauthenticatedPublicRoute("/login", false)).toBe(true);
    expect(isUnauthenticatedPublicRoute("/setup", false)).toBe(true);
  });

  it("only allows registration when public registration is enabled", () => {
    expect(isUnauthenticatedPublicRoute("/register", false)).toBe(false);
    expect(isUnauthenticatedPublicRoute("/register", true)).toBe(true);
  });
});
