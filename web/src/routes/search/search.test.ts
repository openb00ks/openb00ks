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

import SearchPage from "./+page.svelte";

describe("Search page", () => {
  afterEach(() => {
    cleanup();
    mocks.apiFetch.mockReset();
  });

  it("searches unified indexed documents", async () => {
    mocks.apiFetch.mockImplementation((path: string) => {
      if (path.startsWith("/entities/entity-1/accounts")) {
        return Promise.resolve([
          {
            id: "acct-1",
            name: "Internet",
            type: "expense",
            role_tags: ["internet"],
          },
        ]);
      }
      if (path.startsWith("/search?")) {
        return Promise.resolve({
          rows: [
            {
              id: "receipt_receipt-1",
              kind: "receipt",
              object_id: "receipt-1",
              account_id: "acct-1",
              account_name: "Internet",
              title: "office.pdf",
              subtitle: "uploaded",
              body: "office receipt",
              status: "uploaded",
              tags: ["tax"],
              date: "2026-01-02",
              amount_cents: 1200,
              href: "/receipts/receipt-1",
              score: 0.9,
            },
          ],
        });
      }
      return Promise.reject(new Error(`Unexpected call: ${path}`));
    });

    render(SearchPage);

    await fireEvent.input(screen.getByLabelText("Query"), { target: { value: "office" } });
    await fireEvent.change(screen.getByLabelText("Type"), { target: { value: "receipt" } });
    await fireEvent.input(screen.getByLabelText("Status"), { target: { value: "uploaded" } });
    await vi.waitFor(() => {
      expect(screen.getByRole("option", { name: "Internet" })).toBeInTheDocument();
    });
    await fireEvent.change(screen.getByLabelText("Account"), { target: { value: "acct-1" } });
    await fireEvent.input(screen.getByLabelText("Tags"), { target: { value: "tax" } });
    await fireEvent.input(screen.getByLabelText("From"), { target: { value: "2026-01-01" } });
    await fireEvent.input(screen.getByLabelText("To"), { target: { value: "2026-01-31" } });
    await fireEvent.click(screen.getByRole("button", { name: "Search" }));

    await vi.waitFor(() => {
      expect(mocks.apiFetch).toHaveBeenCalledWith(
        expect.stringContaining("/search?"),
      );
    });
    const calledPath = mocks.apiFetch.mock.calls.find(([path]) =>
      String(path).startsWith("/search?"),
    )?.[0] as string;
    expect(calledPath).toContain("entity_id=entity-1");
    expect(calledPath).toContain("q=office");
    expect(calledPath).toContain("kinds=receipt");
    expect(calledPath).toContain("statuses=uploaded");
    expect(calledPath).toContain("account_ids=acct-1");
    expect(calledPath).toContain("tags=tax");
    expect(calledPath).toContain("start_date=2026-01-01");
    expect(calledPath).toContain("end_date=2026-01-31");
    expect(await screen.findByText("office.pdf")).toBeInTheDocument();
    expect(screen.getByText("$12.00")).toBeInTheDocument();
    expect(screen.getByText("tax")).toBeInTheDocument();
  });
});
