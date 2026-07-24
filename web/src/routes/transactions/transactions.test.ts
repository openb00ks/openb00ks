import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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

import TransactionsPage from "./+page.svelte";

describe("Transactions page", () => {
  afterEach(() => {
    cleanup();
    mocks.apiFetch.mockReset();
  });

  beforeEach(() => {
    mocks.apiFetch.mockImplementation((path: string) => {
      if (path.startsWith("/transactions?")) {
        return Promise.resolve({ rows: [] });
      }
      if (path === "/entities/entity-1/accounts") {
        // The accounts endpoint returns a bare array, not { rows }.
        return Promise.resolve([]);
      }
      if (path.startsWith("/search/transactions?")) {
        return Promise.resolve({
          rows: [
            {
              transaction_id: "tx-1",
              entity_id: "entity-1",
              date: "2026-01-05",
              memo: "Internet service",
              account_names: ["Internet"],
              account_role_tags: ["internet"],
              amount_cents: 6500,
              score: 0.95,
            },
          ],
        });
      }
      return Promise.reject(new Error(`Unexpected call: ${path}`));
    });
  });

  it("searches indexed transactions through the shared API client", async () => {
    render(TransactionsPage);

    await vi.waitFor(() => {
      expect(screen.getByText("No transactions in this date range.")).toBeInTheDocument();
    });

    const input = screen.getByLabelText("Search") as HTMLInputElement;
    await fireEvent.input(input, { target: { value: "internet" } });
    await fireEvent.click(screen.getByRole("button", { name: "Search" }));

    await vi.waitFor(() => {
      expect(screen.getByText("Internet service")).toBeInTheDocument();
    });
    expect(mocks.apiFetch).toHaveBeenCalledWith(
      expect.stringContaining("/search/transactions?"),
    );
    expect(screen.getByText("Internet")).toBeInTheDocument();
    expect(screen.getByText("$65.00")).toBeInTheDocument();
  });
});
