import { cleanup, render, screen } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  apiFetch: vi.fn(),
}));

vi.mock("$lib/api/client", () => ({
  apiFetch: mocks.apiFetch,
}));

// AdminGuard renders children only for a loaded admin user.
vi.mock("$lib/stores/current-user", () => ({
  currentUser: {
    subscribe(run: (value: unknown) => void) {
      run({ id: "u1", email: "admin@test.local", is_admin: true });
      return () => void 0;
    },
  },
  isAdmin: {
    subscribe(run: (value: boolean) => void) {
      run(true);
      return () => void 0;
    },
  },
}));

import SystemSettingsPage from "./+page.svelte";

describe("System settings page", () => {
  afterEach(() => {
    cleanup();
    mocks.apiFetch.mockReset();
  });

  beforeEach(() => {
    mocks.apiFetch.mockResolvedValue({
      settings: {
        require_mfa: true,
        enforce_session_timeout: false,
      },
      integrations: {
        ai_provider: "openai",
        ai_model: "gpt-4.1",
        receipt_storage: "local",
        receipt_local_dir: "./.data/receipts",
        receipt_max_bytes: 10_485_760,
      },
    });
  });

  it("shows the MFA enforcement state", async () => {
    render(SystemSettingsPage);

    await vi.waitFor(() => {
      expect(screen.getByText("MFA is required.")).toBeInTheDocument();
    });

    expect(screen.getByRole("link", { name: "Open user management" })).toHaveAttribute(
      "href",
      "/users",
    );
  });
});
