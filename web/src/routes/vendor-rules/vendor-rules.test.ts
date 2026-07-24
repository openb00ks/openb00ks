import { cleanup, render, screen } from "@testing-library/svelte";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  apiFetch: vi.fn(),
}));

vi.mock("$lib/api/client", () => ({
  apiFetch: mocks.apiFetch,
}));

vi.mock("$lib/stores/entity", () => ({
  activeEntity: {
    subscribe(run: (value: string | null) => void) {
      run("entity-1");
      return () => {};
    },
  },
}));

import VendorRulesPage from "./+page.svelte";

describe("Vendor rules page", () => {
  afterEach(() => {
    cleanup();
    mocks.apiFetch.mockReset();
  });

  it("resolves the rule's account id to its name (accounts endpoint is a bare array)", async () => {
    mocks.apiFetch.mockImplementation((path: string) => {
      if (path.startsWith("/vendor-rules?")) {
        return Promise.resolve({
          rows: [
            {
              id: "r1",
              entity_id: "entity-1",
              match_type: "contains",
              pattern: "Starbucks",
              account_id: "acct-meals",
              created_at: "2026-01-02T00:00:00Z",
            },
          ],
        });
      }
      if (path.startsWith("/entities/entity-1/accounts")) {
        // Bare array — the bug was reading `.rows` here, leaving the map empty.
        return Promise.resolve([{ id: "acct-meals", name: "Meals" }]);
      }
      return Promise.reject(new Error(`Unexpected call: ${path}`));
    });

    render(VendorRulesPage);

    expect(await screen.findByText("Starbucks")).toBeInTheDocument();
    await vi.waitFor(() => expect(screen.getAllByText("Meals").length).toBeGreaterThan(0));
    // The raw account uuid must never be shown in place of the name.
    expect(screen.queryByText("acct-meals")).not.toBeInTheDocument();
  });
});
