import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ apiFetch: vi.fn() }));
vi.mock("$lib/api/client", () => ({ apiFetch: mocks.apiFetch }));

import NewJournalEntry from "./NewJournalEntry.svelte";

const accounts = [
  { id: "a1", name: "Cash", type: "asset", code: "1000" },
  { id: "a2", name: "Revenue", type: "income", code: "4000" },
];

function setValue(el: HTMLElement, value: string) {
  (el as HTMLInputElement).value = value;
  el.dispatchEvent(new Event("input", { bubbles: true }));
  el.dispatchEvent(new Event("change", { bubbles: true }));
}

describe("NewJournalEntry", () => {
  beforeEach(() => mocks.apiFetch.mockReset());
  afterEach(() => cleanup());

  it("keeps Post disabled until debits and credits balance, then posts", async () => {
    const oncreated = vi.fn();
    mocks.apiFetch.mockResolvedValue({});
    render(NewJournalEntry, { props: { entityId: "e1", accounts, oncreated } });

    await fireEvent.click(screen.getByRole("button", { name: "New entry" }));

    const selects = screen.getAllByRole("combobox");
    setValue(selects[0], "a1");
    setValue(selects[1], "a2");

    const amounts = screen.getAllByPlaceholderText("0.00");
    // amounts: [line0 debit, line0 credit, line1 debit, line1 credit]
    setValue(amounts[0], "100"); // debit Cash

    // Only one side filled → out of balance, Post disabled.
    await vi.waitFor(() => {
      expect(screen.getByText("Out of balance")).toBeInTheDocument();
    });
    expect(screen.getByRole("button", { name: "Post entry" })).toBeDisabled();

    setValue(amounts[3], "100"); // credit Revenue

    await vi.waitFor(() => {
      expect(screen.getByText("Balanced")).toBeInTheDocument();
    });

    await fireEvent.click(screen.getByRole("button", { name: "Post entry" }));

    expect(mocks.apiFetch).toHaveBeenCalledWith("/transactions", {
      method: "POST",
      body: {
        entity_id: "e1",
        date: expect.any(String),
        memo: "",
        lines: [
          { account_id: "a1", debit_cents: 10000, credit_cents: 0 },
          { account_id: "a2", debit_cents: 0, credit_cents: 10000 },
        ],
      },
    });
    expect(oncreated).toHaveBeenCalled();
  });
});
