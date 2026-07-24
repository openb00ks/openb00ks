import { cleanup, render, screen } from "@testing-library/svelte";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => {
  let activeEntityValue: string | null = "entity-1";
  const activeSubscribers = new Set<(value: string | null) => void>();

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
      run([{ id: "entity-1", name: "Main Entity" }]);
      return () => void 0;
    },
  };

  return {
    activeEntity,
    entities,
    apiFetch: vi.fn(),
    apiFetchBlob: vi.fn(),
  };
});

vi.mock("$lib/api/client", () => ({
  apiFetch: mocks.apiFetch,
  apiFetchBlob: mocks.apiFetchBlob,
}));

vi.mock("$lib/stores/entity", () => ({
  activeEntity: mocks.activeEntity,
  entities: mocks.entities,
}));

vi.mock("$lib/utils/date", () => ({
  formatLocalDate: () => "2026-06-01",
  todayLocalDate: () => "2026-06-12",
}));

import MileagePage from "./+page.svelte";

describe("Mileage page", () => {
  afterEach(() => {
    cleanup();
    mocks.apiFetch.mockReset();
    mocks.apiFetchBlob.mockReset();
  });

  it("clones a mileage entry into a new trip form with today's date", async () => {
    mocks.apiFetch.mockImplementation((path: string) => {
      if (path === "/mileage?entity_id=entity-1&start_date=2026-06-01&end_date=2026-06-12") {
        return Promise.resolve({
          rows: [
            {
              id: "trip-1",
              entity_id: "entity-1",
              date: "2026-06-10",
              distance_miles: 18.4,
              start_location: "Office",
              end_location: "Client",
              purpose: "Client meeting",
              suggestion_context: "Follow-up notes",
              created_at: "2026-06-10T12:00:00Z",
              updated_at: "2026-06-10T12:00:00Z",
            },
          ],
        });
      }
      if (path === "/reports/mileage?entity_id=entity-1&start_date=2026-06-01&end_date=2026-06-12") {
        return Promise.resolve({
          rows: [],
        });
      }
      return Promise.reject(new Error(`Unexpected call: ${path}`));
    });

    render(MileagePage);

    await vi.waitFor(() => {
      expect(screen.getByText("Office → Client")).toBeInTheDocument();
    });

    screen.getByRole("button", { name: "Clone" }).click();

    await vi.waitFor(() => {
      expect(screen.getByRole("heading", { name: "New trip" })).toBeInTheDocument();
    });

    expect((screen.getByLabelText("Date") as HTMLInputElement).value).toBe("2026-06-12");
    expect((screen.getByLabelText("From") as HTMLInputElement).value).toBe("Office");
    expect((screen.getByLabelText("To") as HTMLInputElement).value).toBe("Client");
    expect((screen.getByLabelText("Miles") as HTMLInputElement).value).toBe("18.4");
    expect((screen.getByLabelText("Purpose") as HTMLInputElement).value).toBe("Client meeting");
    expect(
      (screen.getByLabelText("Suggestion context (optional)") as HTMLTextAreaElement).value,
    ).toBe("Follow-up notes");
    expect(screen.getByRole("button", { name: "Save trip" })).toBeInTheDocument();
  });
});
