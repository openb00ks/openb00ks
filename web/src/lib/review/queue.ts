import { apiFetch } from "$lib/api/client";

export type ReviewQueueKind = "receipt" | "import";
export type ReviewQueueStatusFilter =
  | "all"
  | "ready_for_review"
  | "needs_attention";
export type ReviewQueueAction = "suggestion" | "ocr" | "requeue";
export type ReviewQueuePrimaryAction =
  | {
      kind: "link";
      label: string;
      href: string;
    }
  | {
      kind: "action";
      label: string;
      action: ReviewQueueAction;
    };

type QueueListItem = {
  id: string;
  status: string;
  total_cents?: number;
  uploaded_at: string;
  original_name?: string;
  size_bytes?: number;
  content_type?: string;
};

type SuggestionBatchRow = {
  receipt_id: string;
  status?: string;
  confidence?: number;
  cost_cents?: number;
};

type StatusJob = {
  id?: string;
  stage?: string;
  status?: string;
  created_at?: string;
  updated_at?: string;
};

type StatusError = {
  id?: string;
  stage: string;
  error: string;
  created_at?: string;
};

type ReceiptStatusResponse = {
  status?: string;
  latest_job?: StatusJob;
  errors?: StatusError[];
};

export type ReviewQueueItem = QueueListItem & {
  kind: ReviewQueueKind;
  suggestion_status?: string;
  confidence?: number;
  cost_cents?: number;
  latest_job?: StatusJob | null;
  errors: StatusError[];
};

type ReviewApiClient = <T>(
  path: string,
  init?: { method?: string; body?: unknown },
) => Promise<T>;

const DEFAULT_LIMIT = 25;

function queueListPath(
  kind: ReviewQueueKind,
  entityID: string,
  statusFilter: ReviewQueueStatusFilter,
) {
  const query = new URLSearchParams({
    entity_id: entityID,
    limit: String(DEFAULT_LIMIT),
  });
  if (statusFilter !== "all") {
    query.set("status", statusFilter);
  }
  return `/${kind === "import" ? "imports" : "receipts"}?${query.toString()}`;
}

export function queueStatusPath(receiptID: string) {
  return `/receipts/${receiptID}/status`;
}

export function queueActionPath(
  item: ReviewQueueItem,
  action: ReviewQueueAction,
) {
  const base = item.kind === "import" ? "/imports" : "/receipts";
  if (action === "requeue") {
    return `${base}/${item.id}/requeue`;
  }
  if (action === "ocr") {
    return `${base}/${item.id}/ocr/rerun`;
  }
  return `${base}/${item.id}/suggestion/rerun`;
}

export function queueItemHref(item: ReviewQueueItem) {
  return item.kind === "import"
    ? `/imports/${item.id}`
    : `/receipts/${item.id}`;
}

export function queueActionSuccess(action: ReviewQueueAction) {
  if (action === "requeue") {
    return "Queue retry requested.";
  }
  if (action === "ocr") {
    return "OCR rerun queued.";
  }
  return "Suggestion rerun queued.";
}

export function queueActionLabel(action: ReviewQueueAction) {
  if (action === "requeue") {
    return "Retry queue";
  }
  if (action === "ocr") {
    return "Rerun OCR";
  }
  return "Rerun suggestion";
}

export function shouldShowRequeue(item: ReviewQueueItem) {
  return (
    item.status === "needs_attention" || item.latest_job?.status === "failed"
  );
}

export function queuePrimaryAction(
  item: ReviewQueueItem,
): ReviewQueuePrimaryAction {
  if (item.status === "ready_for_review") {
    return {
      kind: "link",
      label: "Open review",
      href: queueItemHref(item),
    };
  }
  if (shouldShowRequeue(item)) {
    return {
      kind: "action",
      label: "Retry processing",
      action: "requeue",
    };
  }
  return {
    kind: "link",
    label: "View details",
    href: queueItemHref(item),
  };
}

export function queueSecondaryActions(item: ReviewQueueItem) {
  const primary = queuePrimaryAction(item);
  const actions: ReviewQueueAction[] = ["suggestion", "ocr"];
  if (shouldShowRequeue(item)) {
    actions.push("requeue");
  }
  if (primary.kind === "action") {
    return actions.filter((action) => action !== primary.action);
  }
  return actions;
}

export async function runQueueAction(
  item: ReviewQueueItem,
  action: ReviewQueueAction,
  client: ReviewApiClient = apiFetch,
) {
  await client(queueActionPath(item, action), { method: "POST" });
  return queueActionSuccess(action);
}

export async function loadReviewQueue(
  entityID: string,
  statusFilter: ReviewQueueStatusFilter,
  kindFilter: ReviewQueueKind | "all",
  client: ReviewApiClient = apiFetch,
): Promise<ReviewQueueItem[]> {
  const [receiptResp, importResp] = await Promise.all([
    client<{ rows: QueueListItem[] }>(
      queueListPath("receipt", entityID, statusFilter),
    ),
    client<{ rows: QueueListItem[] }>(
      queueListPath("import", entityID, statusFilter),
    ),
  ]);

  const receiptItems =
    receiptResp.rows?.map((row) => ({
      ...row,
      kind: "receipt" as const,
      errors: [],
    })) ?? [];
  const importItems =
    importResp.rows?.map((row) => ({
      ...row,
      kind: "import" as const,
      errors: [],
    })) ?? [];

  const merged = [...receiptItems, ...importItems].sort((a, b) =>
    b.uploaded_at.localeCompare(a.uploaded_at),
  );
  const queue =
    kindFilter === "all"
      ? merged
      : merged.filter((item) => item.kind === kindFilter);

  if (queue.length === 0) {
    return [];
  }

  const [suggestions, statusResults] = await Promise.all([
    client<{ rows: SuggestionBatchRow[] }>("/receipts/suggestions/batch", {
      method: "POST",
      body: { receipt_ids: queue.map((item) => item.id) },
    }).catch(() => ({ rows: [] })),
    Promise.allSettled(
      queue.map((item) =>
        client<ReceiptStatusResponse>(queueStatusPath(item.id)),
      ),
    ),
  ]);

  const suggestionByReceipt = new Map<string, SuggestionBatchRow>();
  for (const row of suggestions.rows ?? []) {
    suggestionByReceipt.set(row.receipt_id, row);
  }

  return queue.map((item, index) => {
    const suggestion = suggestionByReceipt.get(item.id);
    const statusResult = statusResults[index];
    const status =
      statusResult?.status === "fulfilled" ? statusResult.value : undefined;

    return {
      ...item,
      suggestion_status: suggestion?.status,
      confidence: suggestion?.confidence,
      cost_cents: suggestion?.cost_cents,
      latest_job: status?.latest_job ?? null,
      errors: status?.errors ?? [],
    };
  });
}
