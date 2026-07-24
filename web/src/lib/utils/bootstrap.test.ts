import { describe, expect, it } from "vitest";

import { resolveBootstrapTarget } from "./bootstrap";

describe("resolveBootstrapTarget", () => {
  it("routes to setup while setup is still required", () => {
    expect(resolveBootstrapTarget(true, "/", false)).toBe("/setup");
    expect(resolveBootstrapTarget(true, "/login", true)).toBe("/setup");
    expect(resolveBootstrapTarget(true, "/setup", false)).toBeNull();
  });

  it("routes unauthenticated users to login after setup completes", () => {
    expect(resolveBootstrapTarget(false, "/", false)).toBe("/login");
    expect(resolveBootstrapTarget(false, "/reports", false)).toBe("/login");
    expect(resolveBootstrapTarget(false, "/login", false)).toBeNull();
    expect(resolveBootstrapTarget(false, "/setup", false)).toBeNull();
  });

  it("keeps an authenticated user on their current page across a reload", () => {
    // Regression: previously this returned "/login" for every app route, logging users out on reload.
    expect(resolveBootstrapTarget(false, "/", true)).toBeNull();
    expect(resolveBootstrapTarget(false, "/reports", true)).toBeNull();
    expect(resolveBootstrapTarget(false, "/receipts/abc-123", true)).toBeNull();
  });

  it("sends an authenticated user off the auth pages to home", () => {
    expect(resolveBootstrapTarget(false, "/login", true)).toBe("/");
    expect(resolveBootstrapTarget(false, "/setup", true)).toBe("/");
  });

  it("honors registration as a public route only when enabled", () => {
    expect(resolveBootstrapTarget(false, "/register", false, true)).toBeNull();
    expect(resolveBootstrapTarget(false, "/register", false, false)).toBe("/login");
  });
});
