import { cleanup, render, screen } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  apiFetch: vi.fn(),
  apiFetchBlob: vi.fn(),
}));

vi.mock("$lib/api/client", () => ({
  apiFetch: mocks.apiFetch,
  apiFetchBlob: mocks.apiFetchBlob,
}));

vi.mock("$lib/stores/entity", () => ({
  activeEntity: {
    subscribe(run: (value: string | null) => void) {
      run("entity-1");
      return () => {};
    },
  },
  entities: {
    subscribe(run: (value: Array<{ id: string; name: string }>) => void) {
      run([{ id: "entity-1", name: "Northwind LLC" }]);
      return () => {};
    },
  },
}));

import ExportsPage from "./+page.svelte";

describe("Exports page", () => {
  afterEach(() => {
    cleanup();
    mocks.apiFetch.mockReset();
    mocks.apiFetchBlob.mockReset();
  });

  beforeEach(() => {
    mocks.apiFetch.mockImplementation((path: string) => {
      if (path.startsWith("/reports/tax-readiness")) {
        return Promise.resolve({
          ready: false,
          exception_count: 3,
          posted_entry_line_count: 12,
          tax_use_profile: {
            tax_year: 2025,
            status: "complete",
            home_office_sqft: 250,
            home_total_sqft: 1000,
            home_utilities_business_use_percent: 25,
            cell_phone_business_use_percent: 75,
            home_internet_business_use_percent: 60,
            href: "/settings/entity",
          },
          account_role_coverage: {
            utilities_count: 1,
            cell_phone_count: 1,
            internet_count: 1,
          },
          import_summary: [
            {
              import_id: "import-1",
              file: "bank.csv",
              status: "ready_for_review",
              row_count: "3",
              parsed_rows: "3",
              error_rows: "0",
              outflow_cents: "3500",
              inflow_cents: "0",
              posted_outflow_cents: "1000",
              posted_inflow_cents: "0",
              mapped_rows: "2",
              posted_rows: "1",
              unposted_rows: "2",
              duplicate_rows: "2",
            },
          ],
          blocking_summary: [
            {
              source_id: "import-1",
              source_name: "bank.csv",
              kind: "import_row",
              status: "ready_for_review",
              issue_count: 2,
              unmapped_rows: 1,
              duplicate_rows: 1,
              not_posted: 0,
              parse_errors: 0,
              first_row_index: "2",
              href: "/imports/import-1#row-2",
            },
            {
              source_id: "receipt-1",
              source_name: "receipt.pdf",
              kind: "receipt",
              status: "uploaded",
              issue_count: 1,
              unmapped_rows: 0,
              duplicate_rows: 0,
              not_posted: 1,
              parse_errors: 0,
              first_row_index: "",
              href: "/receipts/receipt-1",
            },
          ],
          actions: [
            {
              kind: "map_import_rows",
              label: "Map uncategorized rows in bank.csv",
              count: 1,
              href: "/imports/import-1",
              priority: 10,
            },
          ],
          exceptions: [
            {
              source_id: "import-1",
              source_name: "bank.csv",
              kind: "import_row",
              status: "ready_for_review",
              issue: "unmapped import row",
              row_index: "2",
              vendor: "Cafe",
              amount_cents: "1200",
              href: "/imports/import-1#row-2",
            },
            {
              source_id: "receipt-1",
              source_name: "receipt.pdf",
              kind: "receipt",
              status: "uploaded",
              issue: "not posted",
              row_index: "",
              vendor: "",
              amount_cents: "",
              href: "/receipts/receipt-1",
            },
          ],
        });
      }
      return Promise.reject(new Error(`Unexpected call: ${path}`));
    });
    mocks.apiFetchBlob.mockResolvedValue(new Blob());
  });

  it("renders grouped tax readiness blockers and deep links", async () => {
    render(ExportsPage);

    screen.getByRole("button", { name: "Check readiness" }).click();

    await vi.waitFor(() => {
      expect(
        screen.getByText((_, element) => element?.textContent === "3 exception(s) need review"),
      ).toBeInTheDocument();
    });

    expect(screen.getByText("Imports")).toBeInTheDocument();
    expect(screen.getByText("Unmapped")).toBeInTheDocument();
    expect(screen.getByText("Duplicates")).toBeInTheDocument();
    expect(
      screen.getByText(/Home-use allocation:/, {
        selector: ".rounded-xl.border.border-line.px-4.py-3",
      }),
    ).toBeInTheDocument();
    expect(screen.getByText("Tagged accounts: 3 total")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Open row 2/ })).toHaveAttribute(
      "href",
      "/imports/import-1#row-2",
    );
    expect(
      screen.getByRole("link", { name: /Map uncategorized rows in bank\.csv/ }),
    ).toHaveAttribute("href", "/imports/import-1");
    expect(
      screen.getByRole("link", { name: "unmapped import row bank.csv · row 2 · Cafe" }),
    ).toHaveAttribute("href", "/imports/import-1#row-2");
    expect(screen.getByRole("link", { name: "not posted receipt.pdf" })).toHaveAttribute(
      "href",
      "/receipts/receipt-1",
    );
  });

  it("validates tax pack readiness before downloading", async () => {
    render(ExportsPage);

    screen.getByRole("button", { name: "Download tax pack" }).click();

    await vi.waitFor(() => {
      expect(screen.getByText("Resolve the blockers above before downloading the tax pack.")).toBeInTheDocument();
    });

    expect(mocks.apiFetchBlob).not.toHaveBeenCalled();
  });
});
