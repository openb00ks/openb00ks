import { cleanup, render, screen } from "@testing-library/svelte";
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
  entities: {
    subscribe(run: (value: Array<{ id: string; name: string }>) => void) {
      run([{ id: "entity-1", name: "Northwind LLC" }]);
      return () => {};
    },
  },
}));

import TaxPrepPage from "./+page.svelte";

describe("Tax prep page", () => {
  afterEach(() => {
    cleanup();
    mocks.apiFetch.mockReset();
  });

  beforeEach(() => {
    mocks.apiFetch.mockImplementation((path: string) => {
      if (path.startsWith("/reports/tax-readiness")) {
        return Promise.resolve({
          ready: false,
          exception_count: 2,
          posted_entry_line_count: 14,
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
            internet_count: 0,
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
              duplicate_rows: "1",
            },
          ],
          blocking_summary: [
            {
              source_id: "import-1",
              source_name: "bank.csv",
              kind: "import_row",
              status: "ready_for_review",
              issue_count: 1,
              unmapped_rows: 1,
              duplicate_rows: 0,
              not_posted: 0,
              parse_errors: 0,
              first_row_index: "2",
              href: "/imports/import-1#row-2",
            },
          ],
          actions: [],
        });
      }
      if (path.startsWith("/reports/mileage")) {
        return Promise.resolve({
          entity_id: "entity-1",
          start_date: "2025-01-01",
          end_date: "2025-12-31",
          rows: [
            {
              month: "2025-01",
              total_miles: 42.5,
              trip_count: 3,
              rate_cents_per_mile: 70,
              reimbursed_cents: 2975,
            },
            {
              month: "2025-02",
              total_miles: 11.2,
              trip_count: 1,
              rate_missing: true,
            },
          ],
        });
      }
      return Promise.reject(new Error(`Unexpected call: ${path}`));
    });
  });

  it("renders a tax prep checklist with mileage gaps", async () => {
    render(TaxPrepPage);

    await vi.waitFor(() => {
      expect(screen.getByText("Checklist summary")).toBeInTheDocument();
    });

    expect(screen.getByText("2 exception(s) still need attention.")).toBeInTheDocument();
    expect(screen.getByText("Utilities 25% · Cell 75% · Internet 60%")).toBeInTheDocument();
    expect(screen.getAllByText("Tagged accounts")).toHaveLength(2);
    expect(screen.getByText("3 rows in scope")).toBeInTheDocument();
    expect(screen.getByText("53.7 miles in scope")).toBeInTheDocument();
    expect(screen.getByText("Rate gaps")).toBeInTheDocument();
    expect(screen.getByText("Prepared package")).toBeInTheDocument();
    expect(screen.getByText("Blocked")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /bank\.csv/ })).toHaveAttribute(
      "href",
      "/imports/import-1#row-2",
    );
  });
});
