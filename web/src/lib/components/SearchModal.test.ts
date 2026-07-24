import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => {
  let entityValue: string | null = "entity-1";
  const subscribers = new Set<(value: string | null) => void>();
  const activeEntity = {
    subscribe(run: (value: string | null) => void) {
      run(entityValue);
      subscribers.add(run);
      return () => subscribers.delete(run);
    },
    set(value: string | null) {
      entityValue = value;
      for (const run of subscribers) run(entityValue);
    },
  };
  return { activeEntity, apiFetch: vi.fn(), goto: vi.fn() };
});

vi.mock("$app/environment", () => ({ browser: true }));
vi.mock("$app/navigation", () => ({ goto: mocks.goto }));
vi.mock("$lib/api/client", () => ({ apiFetch: mocks.apiFetch }));
vi.mock("$lib/stores/entity", () => ({ activeEntity: mocks.activeEntity }));

import SearchModal from "./SearchModal.svelte";
import { searchOpen, closeSearch } from "$lib/stores/search";

describe("SearchModal", () => {
  beforeEach(() => {
    mocks.apiFetch.mockReset();
    mocks.goto.mockReset();
    mocks.activeEntity.set("entity-1");
    closeSearch();
  });
  afterEach(() => cleanup());

  it("Ctrl+K toggles the modal open and Escape closes it", async () => {
    render(SearchModal);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();

    await fireEvent.keyDown(window, { key: "k", ctrlKey: true });
    await vi.waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());

    await fireEvent.keyDown(window, { key: "Escape" });
    await vi.waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("prompts to select an entity when none is active", async () => {
    mocks.activeEntity.set(null);
    render(SearchModal);
    searchOpen.set(true);
    await vi.waitFor(() => {
      expect(screen.getByText(/Select an active entity/i)).toBeInTheDocument();
    });
    expect(mocks.apiFetch).not.toHaveBeenCalled();
  });

  it("queries /search for the active entity and navigates to a picked hit", async () => {
    mocks.apiFetch.mockResolvedValue({
      rows: [
        {
          id: "r1",
          kind: "transaction",
          object_id: "t1",
          title: "Screaming Frog Ltd",
          subtitle: "Software & SaaS",
          amount_cents: 27900,
          href: "/transactions?focus=t1",
          score: 1,
        },
      ],
    });
    render(SearchModal);
    searchOpen.set(true);

    const input = (await screen.findByPlaceholderText(/Search transactions/i)) as HTMLInputElement;
    input.value = "frog";
    await fireEvent.input(input);

    const hit = await screen.findByText("Screaming Frog Ltd");
    // the request was scoped to the active entity
    expect(mocks.apiFetch).toHaveBeenCalledWith(expect.stringContaining("entity_id=entity-1"));
    expect(mocks.apiFetch).toHaveBeenCalledWith(expect.stringContaining("q=frog"));

    await fireEvent.mouseDown(hit);
    expect(mocks.goto).toHaveBeenCalledWith("/transactions?focus=t1");
  });

  it("Enter with no highlighted hit opens the full search page", async () => {
    mocks.apiFetch.mockResolvedValue({ rows: [] });
    render(SearchModal);
    searchOpen.set(true);

    const input = (await screen.findByPlaceholderText(/Search transactions/i)) as HTMLInputElement;
    input.value = "office";
    await fireEvent.input(input);
    await fireEvent.keyDown(input, { key: "Enter" });

    expect(mocks.goto).toHaveBeenCalledWith("/search?q=office");
  });
});
