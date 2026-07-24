import { cleanup, render, screen } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => {
  const themeValue = "system";
  const theme = {
    subscribe(run: (value: string) => void) {
      run(themeValue);
      return () => void 0;
    },
  };

  return {
    theme,
    apiFetch: vi.fn(),
    clearSession: vi.fn(),
  };
});

vi.mock("$lib/stores/theme", () => ({
  theme: mocks.theme,
  setTheme: vi.fn(),
}));

vi.mock("$lib/api/client", () => ({
  apiFetch: mocks.apiFetch,
}));

vi.mock("$lib/stores/session", () => ({
  clearSession: mocks.clearSession,
}));

import UserSettingsPage from "./+page.svelte";

describe("User settings MFA", () => {
  afterEach(() => {
    cleanup();
    mocks.apiFetch.mockReset();
    mocks.clearSession.mockReset();
  });

  beforeEach(() => {
    mocks.apiFetch.mockImplementation((path: string, init?: { method?: string; body?: unknown }) => {
      if (path === "/me/mfa" && !init?.method) {
        return Promise.resolve({ configured: false, enabled: false });
      }
      if (path === "/me/preferences" && !init?.method) {
        return Promise.resolve({ default_entity_id: "" });
      }
      return Promise.reject(new Error(`Unexpected call: ${path}`));
    });
  });

  it("renders the MFA section and setup action", async () => {
    render(UserSettingsPage);

    expect(screen.getByRole("heading", { name: "MFA" })).toBeInTheDocument();

    await vi.waitFor(() => {
      expect(screen.getByRole("button", { name: "Set up MFA" })).toBeInTheDocument();
    });
  });
});
