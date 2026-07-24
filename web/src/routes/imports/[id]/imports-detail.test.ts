import { cleanup, render, screen } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  apiFetch: vi.fn(),
  pushNotification: vi.fn(),
}));

vi.mock("$app/stores", () => ({
  page: {
    subscribe(run: (value: { params: { id: string } }) => void) {
      run({ params: { id: "import-1" } });
      return () => {};
    },
  },
}));

vi.mock("$lib/api/client", () => ({
  apiFetch: mocks.apiFetch,
}));

vi.mock("$lib/stores/notifications", () => ({
  pushNotification: mocks.pushNotification,
}));

import ImportsDetailPage from "./+page.svelte";

describe("Import detail page", () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    mocks.apiFetch.mockReset();
    mocks.pushNotification.mockReset();
  });

  it("renders import summary and row-level mapping preview", async () => {
    mocks.apiFetch.mockImplementation((path: string) => {
      if (path === "/imports/import-1") {
        return Promise.resolve({
          id: "import-1",
          entity_id: "entity-1",
          status: "ready_for_review",
          content_type: "text/csv",
          size_bytes: 120,
          uploaded_at: "2026-05-31T00:00:00Z",
          original_name: "statement.csv",
        });
      }
      if (path === "/imports/import-1/suggestion") {
        return Promise.resolve({
          rows: [
            {
              id: "s-1",
              status: "succeeded",
              confidence: 0.8,
              created_at: "2026-05-31T00:00:00Z",
              parsed_json: {
                import_summary: {
                  row_count: 3,
                  parsed_rows: 3,
                  total_cents: 2600,
                  top_vendor: "Acme",
                  top_vendors: [
                    { vendor: "Acme", count: 2, total_cents: 2000 },
                    { vendor: "Books", count: 1, total_cents: 600 },
                  ],
                },
                import_rows: [
                  {
                    row_index: 1,
                    vendor: "Acme",
                    date: "2026-05-01",
                    amount_cents: 1200,
                    account_id: "acct-meals",
                    rule_match_type: "contains",
                    rule_pattern: "acme",
                  },
                  {
                    row_index: 2,
                    vendor: "Acme",
                    date: "2026-05-02",
                    amount_cents: 800,
                    account_id: "acct-meals",
                  },
                  {
                    row_index: 3,
                    vendor: "Books",
                    date: "2026-05-03",
                    amount_cents: 600,
                  },
                ],
              },
            },
          ],
        });
      }
      if (path === "/imports/import-1/ocr") {
        return Promise.resolve({ rows: [] });
      }
      if (path === "/imports/import-1/rows") {
        return Promise.resolve({
          rows: [
            {
              id: "row-1",
              row_index: 1,
              date: "2026-05-01",
              vendor: "Acme",
              amount_cents: 1200,
              direction: "outflow",
              account_id: "acct-meals",
              status: "posted",
              fingerprint: "dup-1",
            },
            {
              id: "row-2",
              row_index: 2,
              date: "2026-05-02",
              vendor: "Acme",
              amount_cents: 800,
              direction: "outflow",
              account_id: "acct-meals",
              status: "mapped",
              fingerprint: "dup-1",
            },
            {
              id: "row-3",
              row_index: 3,
              date: "2026-05-03",
              vendor: "Books",
              amount_cents: 600,
              direction: "outflow",
              status: "needs_review",
              fingerprint: "uniq-1",
            },
          ],
        });
      }
      return Promise.reject(new Error(`Unexpected call: ${path}`));
    });

    render(ImportsDetailPage);

    await vi.waitFor(() => {
      expect(screen.getByText("Import details")).toBeInTheDocument();
      expect(screen.getByText("Import summary")).toBeInTheDocument();
      expect(screen.getByText("Row mapping preview")).toBeInTheDocument();
    });

    expect(screen.getByText("2 mapped / 3")).toBeInTheDocument();
    expect(screen.getAllByText("acct-meals").length).toBeGreaterThanOrEqual(2);
    expect(screen.getAllByText("Unmapped").length).toBeGreaterThanOrEqual(1);
    expect(
      screen.getByText((_, element) => element?.textContent === "Total rows: 3"),
    ).toBeInTheDocument();
    expect(
      screen.getByText((_, element) => element?.textContent === "Duplicate-suspect rows: 2"),
    ).toBeInTheDocument();
    expect(screen.getAllByText("Duplicate-suspect").length).toBeGreaterThanOrEqual(2);
    expect(screen.getByRole("link", { name: "Open review queue" })).toHaveAttribute(
      "href",
      "/review",
    );
  });
});
