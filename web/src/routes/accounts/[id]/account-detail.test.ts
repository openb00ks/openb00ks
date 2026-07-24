import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => {
  const pageStore = {
    subscribe(run: (value: unknown) => void) {
      run({ params: { id: "acct-1" }, url: new URL("http://localhost/accounts/acct-1") });
      return () => void 0;
    },
  };
  return { pageStore, apiFetch: vi.fn() };
});

vi.mock("$app/stores", () => ({ page: mocks.pageStore }));
vi.mock("$lib/api/client", () => ({ apiFetch: mocks.apiFetch }));

import AccountDetailPage from "./+page.svelte";

describe("Account detail page", () => {
  afterEach(() => {
    cleanup();
    mocks.apiFetch.mockReset();
  });

  it("renders the account, balance, and its transactions", async () => {
    mocks.apiFetch.mockResolvedValue({
      account: { id: "acct-1", entity_id: "e1", name: "Cash", type: "asset", code: "1000" },
      balance_cents: 27900,
      rows: [{ transaction_id: "t1", date: "2026-01-05", memo: "Screaming Frog", debit_cents: 5000, credit_cents: 0 }],
    });

    render(AccountDetailPage);

    await vi.waitFor(() => {
      expect(screen.getByText("Cash")).toBeInTheDocument();
    });
    // Current balance card + the newest row's running balance both read the current balance.
    expect(screen.getAllByText("$279.00").length).toBeGreaterThanOrEqual(2);
    expect(screen.getByText("$50.00")).toBeInTheDocument(); // the row's debit
    expect(screen.getByText("Screaming Frog")).toBeInTheDocument();
    expect(mocks.apiFetch).toHaveBeenCalledWith("/accounts/acct-1/transactions");
  });

  it("loads an older page on demand", async () => {
    mocks.apiFetch
      .mockResolvedValueOnce({
        account: { id: "acct-1", entity_id: "e1", name: "Cash", type: "asset", code: "1000" },
        balance_cents: 10000,
        rows: [{ transaction_id: "t2", date: "2026-02-01", memo: "Newer", debit_cents: 5000, credit_cents: 0 }],
        has_more: true,
      })
      .mockResolvedValueOnce({
        account: { id: "acct-1", entity_id: "e1", name: "Cash", type: "asset", code: "1000" },
        balance_cents: 10000,
        rows: [{ transaction_id: "t1", date: "2026-01-01", memo: "Older", debit_cents: 5000, credit_cents: 0 }],
        has_more: false,
      });

    render(AccountDetailPage);

    await vi.waitFor(() => expect(screen.getByText("Newer")).toBeInTheDocument());
    await fireEvent.click(screen.getByRole("button", { name: "Load more" }));

    await vi.waitFor(() => expect(screen.getByText("Older")).toBeInTheDocument());
    expect(mocks.apiFetch).toHaveBeenLastCalledWith("/accounts/acct-1/transactions?offset=1");
    expect(screen.queryByRole("button", { name: "Load more" })).not.toBeInTheDocument();
  });

  it("shows an empty state when the account has no transactions", async () => {
    mocks.apiFetch.mockResolvedValue({
      account: { id: "acct-1", entity_id: "e1", name: "Cash", type: "asset", code: "1000" },
      balance_cents: 0,
      rows: [],
    });

    render(AccountDetailPage);

    await vi.waitFor(() => {
      expect(screen.getByText(/No posted transactions/i)).toBeInTheDocument();
    });
  });
});
