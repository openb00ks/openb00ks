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

import StatementsPage from "./+page.svelte";

describe("Statements page", () => {
  afterEach(() => {
    cleanup();
    mocks.apiFetch.mockReset();
  });

  it("loads statement reconciliation rows and creates a statement", async () => {
    mocks.apiFetch.mockImplementation((path: string, init?: RequestInit & { body?: unknown }) => {
      if (path === "/entities/entity-1/accounts") {
        return Promise.resolve([{ id: "acct-1", name: "Operating", type: "asset" }]);
      }
      if (path === "/imports?entity_id=entity-1") {
        return Promise.resolve({
          rows: [{ id: "import-1", original_name: "bank.csv", status: "ready_for_review" }],
        });
      }
      if (path === "/account-statements?entity_id=entity-1") {
        return Promise.resolve({
          rows: [
            {
              id: "stmt-1",
              entity_id: "entity-1",
              account_id: "acct-1",
              account_name: "Operating",
              source_receipt_id: "import-1",
              source_receipt_name: "bank.csv",
              period_start: "2026-01-01",
              period_end: "2026-01-31",
              starting_balance_cents: 10000,
              ending_balance_cents: 11000,
              imported_inflow_cents: 2500,
              imported_outflow_cents: 1000,
              posted_inflow_cents: 2500,
              posted_outflow_cents: 1000,
              expected_ending_balance_cents: 11500,
              difference_cents: -500,
              unposted_rows: 1,
              status: "needs_review",
            },
          ],
        });
      }
      if (path === "/account-statements" && init?.method === "POST") {
        return Promise.resolve({});
      }
      return Promise.reject(new Error(`Unexpected call: ${path}`));
    });

    render(StatementsPage);

    await screen.findByText("Operating");
    expect(screen.getAllByText("bank.csv").length).toBeGreaterThan(0);
    expect(screen.getByText("Difference")).toBeInTheDocument();
    expect(screen.getByText("-$5.00")).toBeInTheDocument();
    expect(screen.getByText("1 unposted row(s)")).toBeInTheDocument();

    await fireEvent.change(screen.getByLabelText("Account"), { target: { value: "acct-1" } });
    await fireEvent.change(screen.getByLabelText("Source import"), { target: { value: "import-1" } });
    await fireEvent.input(screen.getByLabelText("Period start"), { target: { value: "2026-01-01" } });
    await fireEvent.input(screen.getByLabelText("Period end"), { target: { value: "2026-01-31" } });
    await fireEvent.input(screen.getByLabelText("Starting balance"), { target: { value: "100.00" } });
    await fireEvent.input(screen.getByLabelText("Ending balance"), { target: { value: "115.00" } });
    await fireEvent.click(screen.getByRole("button", { name: "Create statement" }));

    await vi.waitFor(() => {
      expect(mocks.apiFetch).toHaveBeenCalledWith("/account-statements", {
        method: "POST",
        body: {
          entity_id: "entity-1",
          account_id: "acct-1",
          source_receipt_id: "import-1",
          period_start: "2026-01-01",
          period_end: "2026-01-31",
          starting_balance_cents: 10000,
          ending_balance_cents: 11500,
          notes: "",
        },
      });
    });
  });

  it("reconciles a statement from the row action", async () => {
    mocks.apiFetch.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/entities/entity-1/accounts") {
        return Promise.resolve([{ id: "acct-1", name: "Operating", type: "asset" }]);
      }
      if (path === "/imports?entity_id=entity-1") {
        return Promise.resolve({ rows: [] });
      }
      if (path === "/account-statements?entity_id=entity-1") {
        return Promise.resolve({
          rows: [
            {
              id: "stmt-1",
              entity_id: "entity-1",
              account_id: "acct-1",
              account_name: "Operating",
              period_start: "2026-01-01",
              period_end: "2026-01-31",
              starting_balance_cents: 10000,
              ending_balance_cents: 11500,
              imported_inflow_cents: 2500,
              imported_outflow_cents: 1000,
              posted_inflow_cents: 2500,
              posted_outflow_cents: 1000,
              expected_ending_balance_cents: 11500,
              difference_cents: 0,
              unposted_rows: 0,
              status: "draft",
            },
          ],
        });
      }
      if (path === "/account-statements/stmt-1/reconcile" && init?.method === "POST") {
        return Promise.resolve({});
      }
      return Promise.reject(new Error(`Unexpected call: ${path}`));
    });

    render(StatementsPage);

    await screen.findByRole("button", { name: "Reconcile" });
    await fireEvent.click(screen.getByRole("button", { name: "Reconcile" }));

    await vi.waitFor(() => {
      expect(mocks.apiFetch).toHaveBeenCalledWith("/account-statements/stmt-1/reconcile", {
        method: "POST",
      });
    });
  });
});
