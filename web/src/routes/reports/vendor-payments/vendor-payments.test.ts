import { cleanup, render, screen } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => {
  const activeEntity = {
    subscribe(run: (value: string | null) => void) {
      run("entity-1");
      return () => void 0;
    },
  };
  const entities = {
    subscribe(run: (value: Array<{ id: string; name: string }>) => void) {
      run([{ id: "entity-1", name: "Acme Co" }]);
      return () => void 0;
    },
  };
  return { activeEntity, entities, apiFetch: vi.fn() };
});

vi.mock("$lib/stores/entity", () => ({ activeEntity: mocks.activeEntity, entities: mocks.entities }));
vi.mock("$lib/api/client", () => ({ apiFetch: mocks.apiFetch }));

import VendorPaymentsPage from "./+page.svelte";

describe("Vendor payments (1099) report", () => {
  beforeEach(() => mocks.apiFetch.mockReset());
  afterEach(() => cleanup());

  it("lists vendor totals and flags 1099 candidates", async () => {
    mocks.apiFetch.mockResolvedValue({
      threshold_cents: 60000,
      rows: [
        { vendor_id: "v1", vendor_name: "Screaming Frog Ltd", tax_id: "12-3456789", total_cents: 70000, needs_1099: true },
        { vendor_id: "v2", vendor_name: "Corner Cafe", tax_id: "", total_cents: 4200, needs_1099: false },
      ],
    });

    render(VendorPaymentsPage);

    await vi.waitFor(() => expect(screen.getByText("Screaming Frog Ltd")).toBeInTheDocument());
    expect(screen.getByText("$700.00")).toBeInTheDocument();
    expect(screen.getByText("12-3456789")).toBeInTheDocument();
    expect(screen.getByText("Review")).toBeInTheDocument(); // the 1099 flag
    expect(screen.getByText("1 flagged for 1099")).toBeInTheDocument();
    expect(mocks.apiFetch).toHaveBeenCalledWith(expect.stringContaining("/reports/vendor-payments?"));
  });
});
