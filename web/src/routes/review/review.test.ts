import { cleanup, render, screen } from "@testing-library/svelte";
import { fireEvent } from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  loadReviewQueue: vi.fn(),
  runQueueAction: vi.fn(),
}));

vi.mock("$lib/review/queue", async () => {
  const actual = await vi.importActual<typeof import("$lib/review/queue")>(
    "$lib/review/queue",
  );
  return {
    ...actual,
    loadReviewQueue: mocks.loadReviewQueue,
    runQueueAction: mocks.runQueueAction,
  };
});

vi.mock("$lib/stores/entity", () => ({
  activeEntity: {
    subscribe(run: (value: string | null) => void) {
      run("entity-1");
      return () => {};
    },
  },
}));

vi.mock("$lib/stores/notifications", () => ({
  pushNotification: vi.fn(),
}));

import ReviewPage from "./+page.svelte";

describe("Review queue page", () => {
  afterEach(() => {
    cleanup();
    mocks.loadReviewQueue.mockReset();
    mocks.runQueueAction.mockReset();
  });

  beforeEach(() => {
    mocks.loadReviewQueue.mockResolvedValue([
      {
        id: "receipt-1",
        kind: "receipt",
        status: "ready_for_review",
        uploaded_at: "2026-02-28T11:00:00Z",
        original_name: "receipt.png",
        errors: [],
      },
      {
        id: "import-1",
        kind: "import",
        status: "needs_attention",
        uploaded_at: "2026-02-28T12:00:00Z",
        original_name: "statement.csv",
        latest_job: { stage: "suggest", status: "failed" },
        errors: [],
      },
    ]);
    mocks.runQueueAction.mockResolvedValue("Suggestion rerun queued.");
  });

  it("runs a bulk review action on the selected queue items", async () => {
    render(ReviewPage);

    await screen.findByText("receipt.png");

    await fireEvent.click(screen.getByRole("button", { name: "Select visible" }));
    await fireEvent.click(screen.getByRole("button", { name: "Rerun suggestions" }));

    expect(mocks.runQueueAction).toHaveBeenCalledTimes(2);
    expect(mocks.runQueueAction).toHaveBeenCalledWith(
      expect.objectContaining({ id: "receipt-1" }),
      "suggestion",
    );
    expect(mocks.runQueueAction).toHaveBeenCalledWith(
      expect.objectContaining({ id: "import-1" }),
      "suggestion",
    );
    expect(
      await screen.findByText("Rerun suggestion applied to 2 item(s)."),
    ).toBeInTheDocument();
  });
});
