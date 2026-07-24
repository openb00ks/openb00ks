import { cleanup, render } from "@testing-library/svelte";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ goto: vi.fn() }));
vi.mock("$app/navigation", () => ({ goto: mocks.goto }));

import PrepSummaryPage from "./+page.svelte";

describe("Preparer summary (redirect)", () => {
  afterEach(() => {
    cleanup();
    mocks.goto.mockReset();
  });

  it("redirects to the merged Tax prep page", async () => {
    render(PrepSummaryPage);
    await vi.waitFor(() => {
      expect(mocks.goto).toHaveBeenCalledWith("/tax-prep", { replaceState: true });
    });
  });
});
