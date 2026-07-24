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

import UsersPage from "./+page.svelte";

describe("Users page", () => {
  afterEach(() => {
    cleanup();
    mocks.apiFetch.mockReset();
  });

  beforeEach(() => {
    mocks.apiFetch.mockImplementation((path: string, init?: { method?: string }) => {
      if (path === "/users" && !init?.method) {
        return Promise.resolve([
          {
            id: "user-1",
            email: "admin@test.local",
            is_admin: true,
            mfa_enabled: true,
            mfa_configured: true,
            created_at: "2026-06-12T00:00:00Z",
          },
        ]);
      }
      if (path === "/users/user-1/mfa/reset" && init?.method === "POST") {
        return Promise.resolve(undefined);
      }
      return Promise.reject(new Error(`Unexpected call: ${path}`));
    });
  });

  it("renders users and allows MFA reset", async () => {
    render(UsersPage);

    await vi.waitFor(() => {
      expect(screen.getByText("admin@test.local")).toBeInTheDocument();
    });

    // Two-step: arm, then confirm.
    screen.getByRole("button", { name: "Reset MFA" }).click();
    await vi.waitFor(() => {
      expect(screen.getByRole("button", { name: "Confirm reset" })).toBeInTheDocument();
    });
    screen.getByRole("button", { name: "Confirm reset" }).click();

    await vi.waitFor(() => {
      expect(
        screen.getByText("MFA reset for admin@test.local. They will need to re-enroll."),
      ).toBeInTheDocument();
    });
  });
});
