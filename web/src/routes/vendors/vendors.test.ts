import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";
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

import VendorsPage from "./+page.svelte";

const vendorRow = {
  id: "v1",
  entity_id: "entity-1",
  name: "Blue Bottle Coffee",
  normalized_name: "bluebottlecoffee",
  match_pattern: "BLUE BOTTLE",
  default_account_id: "acct-meals",
  receipt_count: 4,
};

// The accounts endpoint returns a bare array (not { rows }).
const accountsArray = [{ id: "acct-meals", name: "Meals", type: "expense" }];

describe("Vendors page", () => {
  afterEach(() => {
    cleanup();
    mocks.apiFetch.mockReset();
  });

  it("lists vendors with their default account name and seen count", async () => {
    mocks.apiFetch.mockImplementation((path: string) => {
      if (path.startsWith("/vendors?")) {
        return Promise.resolve({ rows: [vendorRow] });
      }
      if (path.startsWith("/entities/entity-1/accounts")) {
        return Promise.resolve(accountsArray);
      }
      return Promise.reject(new Error(`Unexpected call: ${path}`));
    });

    render(VendorsPage);

    expect(await screen.findByText("Blue Bottle Coffee")).toBeInTheDocument();
    // Resolves the default account id to its human name (not the raw uuid) — appears in both the row
    // and the create-form option, so assert presence + that the raw uuid is never shown.
    await vi.waitFor(() => expect(screen.getAllByText("Meals").length).toBeGreaterThan(0));
    expect(screen.queryByText("acct-meals")).not.toBeInTheDocument();
    expect(screen.getByText("Seen 4×")).toBeInTheDocument();
    expect(screen.getByText(/Matches/)).toBeInTheDocument();
  });

  it("creates a vendor with the entity scope and reloads", async () => {
    mocks.apiFetch.mockImplementation((path: string, opts?: { method?: string }) => {
      if (path === "/vendors" && opts?.method === "POST") {
        return Promise.resolve({ id: "v2" });
      }
      if (path.startsWith("/vendors?")) {
        return Promise.resolve({ rows: [] });
      }
      if (path.startsWith("/entities/entity-1/accounts")) {
        return Promise.resolve(accountsArray);
      }
      return Promise.reject(new Error(`Unexpected call: ${path}`));
    });

    render(VendorsPage);

    await fireEvent.input(screen.getByLabelText("Name"), { target: { value: "Sweetgreen" } });
    await fireEvent.input(screen.getByLabelText("Match pattern"), { target: { value: "SWEETGREEN" } });
    await fireEvent.click(screen.getByRole("button", { name: "Add vendor" }));

    await vi.waitFor(() => {
      const call = mocks.apiFetch.mock.calls.find(
        ([path, opts]) => path === "/vendors" && (opts as { method?: string })?.method === "POST",
      );
      expect(call).toBeTruthy();
      const body = (call?.[1] as { body: Record<string, unknown> }).body;
      expect(body).toMatchObject({
        entity_id: "entity-1",
        name: "Sweetgreen",
        match_pattern: "SWEETGREEN",
      });
    });
  });
});
