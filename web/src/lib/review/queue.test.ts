import { describe, expect, it, vi } from "vitest";

import {
  loadReviewQueue,
  queueActionPath,
  queuePrimaryAction,
  queueSecondaryActions,
  queueActionSuccess,
  queueStatusPath,
  runQueueAction,
  shouldShowRequeue,
  type ReviewQueueItem,
} from "./queue";

describe("review queue helpers", () => {
  it("loads and enriches queue items using existing APIs", async () => {
    const api = vi.fn(
      async <T>(path: string, init?: { method?: string; body?: unknown }) => {
        if (path.startsWith("/receipts?")) {
          return {
            rows: [
              {
                id: "receipt-1",
                status: "ready_for_review",
                total_cents: 4200,
                uploaded_at: "2026-02-28T11:00:00Z",
                original_name: "receipt.png",
              },
            ],
          } as T;
        }
        if (path.startsWith("/imports?")) {
          return {
            rows: [
              {
                id: "import-1",
                status: "needs_attention",
                uploaded_at: "2026-02-28T12:00:00Z",
                original_name: "statement.csv",
              },
            ],
          } as T;
        }
        if (path === "/receipts/suggestions/batch") {
          expect(init).toEqual({
            method: "POST",
            body: { receipt_ids: ["import-1", "receipt-1"] },
          });
          return {
            rows: [
              {
                receipt_id: "import-1",
                status: "failed",
                confidence: 0.31,
                cost_cents: 7,
              },
              {
                receipt_id: "receipt-1",
                status: "completed",
                confidence: 0.92,
                cost_cents: 3,
              },
            ],
          } as T;
        }
        if (path === "/receipts/import-1/status") {
          return {
            latest_job: { stage: "suggest", status: "failed" },
            errors: [{ stage: "suggest", error: "parse failed" }],
          } as T;
        }
        if (path === "/receipts/receipt-1/status") {
          return {
            latest_job: { stage: "draft", status: "completed" },
            errors: [],
          } as T;
        }
        throw new Error(`unexpected path ${path}`);
      },
    );

    const items = await loadReviewQueue(
      "entity-1",
      "all",
      "all",
      api as unknown as <T>(
        path: string,
        init?: { method?: string; body?: unknown },
      ) => Promise<T>,
    );

    expect(items).toHaveLength(2);
    expect(items[0]).toMatchObject({
      id: "import-1",
      kind: "import",
      suggestion_status: "failed",
      confidence: 0.31,
      cost_cents: 7,
      latest_job: { stage: "suggest", status: "failed" },
      errors: [{ stage: "suggest", error: "parse failed" }],
    });
    expect(items[1]).toMatchObject({
      id: "receipt-1",
      kind: "receipt",
      suggestion_status: "completed",
      confidence: 0.92,
      latest_job: { stage: "draft", status: "completed" },
    });
    expect(api).toHaveBeenCalledWith(queueStatusPath("import-1"));
    expect(api).toHaveBeenCalledWith(queueStatusPath("receipt-1"));
  });

  it("uses the correct rerun endpoints for receipt and import items", async () => {
    const api = vi.fn(async <T>() => ({}) as T);
    const receiptItem: ReviewQueueItem = {
      id: "receipt-1",
      kind: "receipt",
      status: "ready_for_review",
      uploaded_at: "2026-02-28T12:00:00Z",
      errors: [],
    };
    const importItem: ReviewQueueItem = {
      id: "import-1",
      kind: "import",
      status: "needs_attention",
      uploaded_at: "2026-02-28T12:00:00Z",
      latest_job: { stage: "suggest", status: "failed" },
      errors: [],
    };

    expect(queueActionPath(receiptItem, "suggestion")).toBe(
      "/receipts/receipt-1/suggestion/rerun",
    );
    expect(queueActionPath(importItem, "ocr")).toBe(
      "/imports/import-1/ocr/rerun",
    );
    expect(queueActionPath(importItem, "requeue")).toBe(
      "/imports/import-1/requeue",
    );
    expect(shouldShowRequeue(importItem)).toBe(true);
    expect(queuePrimaryAction(receiptItem)).toEqual({
      kind: "link",
      label: "Open review",
      href: "/receipts/receipt-1",
    });
    expect(queuePrimaryAction(importItem)).toEqual({
      kind: "action",
      label: "Retry processing",
      action: "requeue",
    });
    expect(queueSecondaryActions(importItem)).toEqual(["suggestion", "ocr"]);
    expect(queueActionSuccess("requeue")).toBe("Queue retry requested.");

    await runQueueAction(
      importItem,
      "requeue",
      api as unknown as <T>(
        path: string,
        init?: { method?: string; body?: unknown },
      ) => Promise<T>,
    );

    expect(api).toHaveBeenCalledWith("/imports/import-1/requeue", {
      method: "POST",
    });
  });
});
