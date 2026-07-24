import { cleanup, render, screen } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  apiFetch: vi.fn(),
  apiFetchPublic: vi.fn(),
  goto: vi.fn(),
}));

vi.mock("$app/environment", () => ({
  browser: true,
}));

vi.mock("$app/navigation", () => ({
  goto: mocks.goto,
}));

vi.mock("$lib/api/client", () => ({
  apiFetch: mocks.apiFetch,
  apiFetchPublic: mocks.apiFetchPublic,
}));

import SetupPage from "./+page.svelte";

describe("Setup page", () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    mocks.apiFetch.mockReset();
    mocks.apiFetchPublic.mockReset();
    mocks.goto.mockReset();
  });

  it("explains what a tenant is", async () => {
    mocks.apiFetchPublic.mockResolvedValue({ required: true });
    mocks.apiFetch.mockResolvedValue(undefined);

    render(SetupPage);

    expect(
      screen.getByRole("heading", { name: /create the first workspace/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/a tenant is one isolated bookkeeping workspace/i),
    ).toBeInTheDocument();
  });
});
