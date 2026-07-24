import { cleanup, render, screen } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  goto: vi.fn(),
  apiFetch: vi.fn(),
  apiFetchPublic: vi.fn(),
  setSession: vi.fn(),
}));

vi.mock("$app/navigation", () => ({
  goto: mocks.goto,
}));

vi.mock("$lib/api/client", () => ({
  apiFetch: mocks.apiFetch,
  apiFetchPublic: mocks.apiFetchPublic,
}));

vi.mock("$lib/stores/session", () => ({
  setSession: mocks.setSession,
}));

vi.mock("$lib/utils/auth-mode", () => ({
  publicRegistrationEnabled: () => false,
}));

vi.mock("$lib/utils/setup", () => ({
  decideSetupRoute: () => "/setup",
}));

import LoginPage from "./+page.svelte";

describe("Login page", () => {
  afterEach(() => {
    cleanup();
    mocks.goto.mockReset();
    mocks.apiFetch.mockReset();
    mocks.apiFetchPublic.mockReset();
    mocks.setSession.mockReset();
  });

  beforeEach(() => {
    mocks.apiFetchPublic.mockResolvedValue({ required: false });
  });

  it("renders login heading", () => {
    render(LoginPage);
    expect(
      screen.getByRole("heading", { name: /sign in/i }),
    ).toBeInTheDocument();
  });

  it("prompts for an MFA code when the API requires a challenge", async () => {
    mocks.apiFetch.mockResolvedValue({
      mfa_required: true,
      challenge_token: "challenge-token",
    });

    render(LoginPage);

    await vi.waitFor(() => {
      expect(screen.getByRole("button", { name: "Sign in" })).toBeInTheDocument();
    });

    screen.getByLabelText("Email").setAttribute("value", "alex@openb00ks.local");
    screen.getByLabelText("Password").setAttribute("value", "secret123");
    (screen.getByLabelText("Email") as HTMLInputElement).value = "alex@openb00ks.local";
    (screen.getByLabelText("Password") as HTMLInputElement).value = "secret123";
    (screen.getByLabelText("Email") as HTMLInputElement).dispatchEvent(
      new Event("input", { bubbles: true }),
    );
    (screen.getByLabelText("Password") as HTMLInputElement).dispatchEvent(
      new Event("input", { bubbles: true }),
    );

    screen.getByRole("button", { name: "Sign in" }).click();

    await vi.waitFor(() => {
      expect(screen.getByLabelText("MFA code")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Verify code" })).toBeInTheDocument();
    });

    expect(mocks.apiFetch).toHaveBeenCalledWith("/auth/login", {
      method: "POST",
      body: { email: "alex@openb00ks.local", password: "secret123" },
    });
  });
});
