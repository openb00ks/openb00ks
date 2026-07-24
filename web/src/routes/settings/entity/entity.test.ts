import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => {
  let activeEntityValue: string | null = "entity-1";
  let entityRows = [
    { id: "entity-1", name: "Main Entity", fiscal_year_start_month: 1, fiscal_year_start_day: 1 },
  ];
  const activeSubscribers = new Set<(value: string | null) => void>();
  const entitySubscribers = new Set<(value: typeof entityRows) => void>();

  const activeEntity = {
    subscribe(run: (value: string | null) => void) {
      run(activeEntityValue);
      activeSubscribers.add(run);
      return () => activeSubscribers.delete(run);
    },
    set(value: string | null) {
      activeEntityValue = value;
      for (const run of activeSubscribers) {
        run(activeEntityValue);
      }
    },
  };

  const entities = {
    subscribe(run: (value: Array<{ id: string; name: string }>) => void) {
      run(entityRows);
      entitySubscribers.add(run as (value: typeof entityRows) => void);
      return () => entitySubscribers.delete(run as (value: typeof entityRows) => void);
    },
    update(updater: (rows: typeof entityRows) => typeof entityRows) {
      entityRows = updater(entityRows);
      for (const run of entitySubscribers) {
        run(entityRows);
      }
    },
  };

  return {
    activeEntity,
    entities,
    apiFetch: vi.fn(),
    resetEntities() {
      entityRows = [
        { id: "entity-1", name: "Main Entity", fiscal_year_start_month: 1, fiscal_year_start_day: 1 },
      ];
      for (const run of entitySubscribers) {
        run(entityRows);
      }
    },
  };
});

vi.mock("$lib/api/client", () => ({
  apiFetch: mocks.apiFetch,
}));

vi.mock("$lib/stores/entity", () => ({
  activeEntity: mocks.activeEntity,
  entities: mocks.entities,
}));

import EntitySettingsPage from "./+page.svelte";

describe("Entity settings page", () => {
  afterEach(() => {
    cleanup();
    mocks.apiFetch.mockReset();
    mocks.resetEntities();
  });

  it("loads and saves home-use allocation settings", async () => {
    mocks.apiFetch.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/me/preferences" && !init?.method) {
        return Promise.resolve({ default_entity_id: "entity-1" });
      }
      if (path === "/entities/entity-1/tax-settings?year=2026" && !init?.method) {
        return Promise.resolve({
          tax_year: 2026,
          home_office_sqft: 250,
          home_total_sqft: 1000,
          cell_phone_business_use_percent: 75,
          home_internet_business_use_percent: 60,
        });
      }
      if (path === "/me/preferences" && init?.method === "PATCH") {
        return Promise.resolve({});
      }
      if (path === "/entities/entity-1/tax-settings" && init?.method === "PATCH") {
        return Promise.resolve({
          tax_year: 2026,
          home_office_sqft: 300,
          home_total_sqft: 1200,
          cell_phone_business_use_percent: 70,
          home_internet_business_use_percent: 65,
          home_utilities_business_use_percent: 25,
        });
      }
      return Promise.reject(new Error(`Unexpected call: ${path}`));
    });

    render(EntitySettingsPage);

    await vi.waitFor(() => {
      expect(screen.getByLabelText("Tax year")).toHaveValue("2026");
    });

    expect(screen.getByLabelText("Home office square feet")).toHaveValue("250");
    expect(screen.getByLabelText("Total home square feet")).toHaveValue("1000");
    expect(screen.getByLabelText("Cell phone business-use percent")).toHaveValue("75");
    expect(screen.getByLabelText("Home internet business-use percent")).toHaveValue("60");
    expect(screen.getByRole("heading", { name: "Home use allocation" })).toBeInTheDocument();
    expect(screen.getByText("25%")).toBeInTheDocument();

    await screen.findByRole("button", { name: "Save home use allocation" });
    await screen.getByRole("button", { name: "Save home use allocation" }).click();

    await vi.waitFor(() => {
      expect(mocks.apiFetch).toHaveBeenCalledWith("/entities/entity-1/tax-settings", {
        method: "PATCH",
        body: {
          tax_year: 2026,
          home_office_sqft: 250,
          home_total_sqft: 1000,
          cell_phone_business_use_percent: 75,
          home_internet_business_use_percent: 60,
        },
      });
    });
  });

  it("saves the entity fiscal year start", async () => {
    mocks.apiFetch.mockImplementation((path: string, init?: RequestInit) => {
      if (path === "/me/preferences" && !init?.method) {
        return Promise.resolve({ default_entity_id: "entity-1" });
      }
      if (path === "/entities/entity-1/tax-settings?year=2026" && !init?.method) {
        return Promise.resolve({ tax_year: 2026 });
      }
      if (path === "/entities/entity-1" && init?.method === "PATCH") {
        return Promise.resolve({
          id: "entity-1",
          name: "Main Entity",
          fiscal_year_start_month: 4,
          fiscal_year_start_day: 1,
        });
      }
      return Promise.reject(new Error(`Unexpected call: ${path}`));
    });

    render(EntitySettingsPage);

    await screen.findByLabelText("Fiscal start month");
    await screen.getByLabelText("Fiscal start month").focus();
    await vi.waitFor(() => {
      expect(screen.getByLabelText("Fiscal start month")).toHaveValue("1");
    });

    await fireEvent.input(screen.getByLabelText("Fiscal start month"), { target: { value: "4" } });
    await fireEvent.input(screen.getByLabelText("Fiscal start day"), { target: { value: "1" } });

    await screen.getByRole("button", { name: "Save fiscal year" }).click();

    await vi.waitFor(() => {
      expect(mocks.apiFetch).toHaveBeenCalledWith("/entities/entity-1", {
        method: "PATCH",
        body: {
          fiscal_year_start_month: 4,
          fiscal_year_start_day: 1,
        },
      });
    });
  });
});
