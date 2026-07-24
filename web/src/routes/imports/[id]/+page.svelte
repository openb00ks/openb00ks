<script lang="ts">
  import { errorMessage } from "$lib/utils/errors";
  import { onMount } from "svelte";
  import { page } from "$app/stores";
  import { apiFetch } from "$lib/api/client";
  import type { Account, OcrRun as OCRRow } from "$lib/api/types";
  import { readImportRows, readImportSummary } from "$lib/imports/summary";
  import { pushNotification } from "$lib/stores/notifications";

  type ImportResponse = {
    id: string;
    entity_id: string;
    status: string;
    content_type: string;
    size_bytes: number;
    uploaded_at: string;
    original_name?: string;
    url?: string;
    suggestion_context?: string;
  };

  type SuggestionRow = {
    id: string;
    status: string;
    confidence?: number;
    cost_cents?: number;
    total_tokens?: number;
    prompt_tokens?: number;
    completion_tokens?: number;
    created_at: string;
    source_url?: string;
    parsed_json?: unknown;
  };

  type ImportRow = {
    id: string;
    row_index: number;
    date: string;
    vendor: string;
    memo?: string;
    amount_cents: number;
    direction: string;
    account_id?: string;
    status: string;
    transaction_id?: string;
    fingerprint?: string;
  };

  type AccountRow = Pick<Account, 'id' | 'name' | 'type'>;

  let importItem: ImportResponse | null = $state(null);
  let suggestions: SuggestionRow[] = $state([]);
  let ocrHistory: OCRRow[] = $state([]);
  let importRows: ImportRow[] = $state([]);
  let accounts: AccountRow[] = $state([]);
  let loading = $state(true);
  let error = $state("");
  let rowActionError = $state("");
  let rowActionMessage = $state("");
  let rowActionLoading: Record<number, boolean> = $state({});
  let selectedRows: Record<number, boolean> = $state({});
  let rowFilter: "all" | "unmapped" | "mapped" | "posted" | "unposted" = $state("all");
  let rowSearch = $state("");
  let bulkAccountId = $state("");
  let bulkAssigning = $state(false);
  let bulkPosting = $state(false);
  let rerunning = $state(false);
  let rerunError = $state("");
  let rerunSuccess = $state("");
  let ocrRerunLoading = $state(false);
  let ocrMessage = $state("");
  const latestImportSummary = $derived(
    readImportSummary(suggestions[0]?.parsed_json),
  );
  const latestImportRows = $derived(
    readImportRows(suggestions[0]?.parsed_json),
  );
  const filteredImportRows = $derived(
    importRows.filter((row) => {
      if (rowFilter === "unmapped" && row.account_id) {
        return false;
      }
      if (rowFilter === "mapped" && !row.account_id) {
        return false;
      }
      if (rowFilter === "posted" && row.status !== "posted") {
        return false;
      }
      if (rowFilter === "unposted" && row.status === "posted") {
        return false;
      }
      const search = rowSearch.trim().toLowerCase();
      if (!search) {
        return true;
      }
      return `${row.vendor} ${row.memo ?? ""} ${row.date}`
        .toLowerCase()
        .includes(search);
    }),
  );
  const selectedRowIndexes = $derived(
    Object.entries(selectedRows)
      .filter(([, selected]) => selected)
      .map(([rowIndex]) => Number(rowIndex)),
  );
  const rowCounts = $derived({
    total: importRows.length,
    mapped: importRows.filter((row) => row.account_id).length,
    posted: importRows.filter((row) => row.status === "posted").length,
  });
  const duplicateRows = $derived((() => {
    const counts = new Map<string, number>();
    for (const row of importRows) {
      if (!row.fingerprint) {
        continue;
      }
      counts.set(row.fingerprint, (counts.get(row.fingerprint) ?? 0) + 1);
    }
    return importRows.filter(
      (row) => row.fingerprint && (counts.get(row.fingerprint) ?? 0) > 1,
    );
  })());

  function formatBytes(size: number) {
    if (size < 1024) {
      return `${size} B`;
    }
    if (size < 1024 * 1024) {
      return `${(size / 1024).toFixed(1)} KB`;
    }
    return `${(size / 1024 / 1024).toFixed(1)} MB`;
  }

  function formatCost(cents?: number) {
    if (!cents) {
      return "—";
    }
    return new Intl.NumberFormat("en-US", {
      style: "currency",
      currency: "USD",
    }).format(cents / 100);
  }

  function accountName(accountId?: string) {
    return accounts.find((account) => account.id === accountId)?.name ?? accountId ?? "";
  }

  function toggleVisibleRows(selected: boolean) {
    const next = { ...selectedRows };
    for (const row of filteredImportRows) {
      if (row.status !== "posted") {
        next[row.row_index] = selected;
      }
    }
    selectedRows = next;
  }

  function clearSelectedRows() {
    selectedRows = {};
  }

  async function loadImport() {
    loading = true;
    error = "";
    try {
      const id = $page.params.id;
      const [detail, suggestionHistory, ocrResponse, rowsResponse] = await Promise.all([
        apiFetch<ImportResponse>(`/imports/${id}`),
        apiFetch<{ rows: SuggestionRow[] }>(`/imports/${id}/suggestion`).catch(
          () => ({ rows: [] }),
        ),
        apiFetch<{ rows: OCRRow[] }>(`/imports/${id}/ocr`).catch(() => ({
          rows: [],
        })),
        apiFetch<{ rows: ImportRow[] }>(`/imports/${id}/rows`).catch(() => ({
          rows: [],
        })),
      ]);
      importItem = detail;
      suggestions = suggestionHistory.rows ?? [];
      ocrHistory = ocrResponse.rows ?? [];
      importRows = rowsResponse.rows ?? [];
      // The accounts endpoint returns a bare array, not { rows }.
      accounts = await apiFetch<AccountRow[]>(
        `/entities/${detail.entity_id}/accounts?limit=1000`,
      ).catch(() => []);
    } catch (err) {
      error = errorMessage(err, "Unable to load import.");
    } finally {
      loading = false;
    }
  }

  async function rerunSuggestion() {
    if (!importItem) {
      return;
    }
    rerunning = true;
    rerunError = "";
    rerunSuccess = "";
    try {
      await apiFetch(`/imports/${importItem.id}/suggestion/rerun`, {
        method: "POST",
      });
      rerunSuccess = "Suggestion re-queued.";
      pushNotification(rerunSuccess, "success");
      await loadImport();
    } catch (err) {
      rerunError =
        errorMessage(err, "Unable to rerun suggestion.");
      pushNotification(rerunError, "error");
    } finally {
      rerunning = false;
    }
  }

  async function rerunOCR() {
    if (!importItem) {
      return;
    }
    ocrMessage = "";
    ocrRerunLoading = true;
    try {
      await apiFetch(`/imports/${importItem.id}/ocr/rerun`, { method: "POST" });
      ocrMessage = "OCR rerun queued.";
      pushNotification(ocrMessage, "success");
      await loadImport();
    } catch (err) {
      ocrMessage = errorMessage(err, "Unable to rerun OCR.");
      pushNotification(ocrMessage, "error");
    } finally {
      ocrRerunLoading = false;
    }
  }

  async function assignRowAccount(row: ImportRow, accountId: string) {
    if (!importItem || !accountId) {
      return;
    }
    rowActionError = "";
    rowActionLoading = { ...rowActionLoading, [row.row_index]: true };
    try {
      const updated = await apiFetch<ImportRow>(
        `/imports/${importItem.id}/rows/${row.row_index}`,
        {
          method: "PATCH",
          body: { account_id: accountId },
        },
      );
      importRows = importRows.map((item) =>
        item.row_index === updated.row_index ? updated : item,
      );
    } catch (err) {
      rowActionError =
        errorMessage(err, "Unable to assign account.");
      pushNotification(rowActionError, "error");
    } finally {
      rowActionLoading = { ...rowActionLoading, [row.row_index]: false };
    }
  }

  async function postImportRow(row: ImportRow) {
    if (!importItem) {
      return;
    }
    rowActionError = "";
    rowActionLoading = { ...rowActionLoading, [row.row_index]: true };
    try {
      await apiFetch(`/imports/${importItem.id}/rows/${row.row_index}/post`, {
        method: "POST",
      });
      pushNotification("Import row posted.", "success");
      await loadImport();
    } catch (err) {
      rowActionError =
        errorMessage(err, "Unable to post import row.");
      pushNotification(rowActionError, "error");
    } finally {
      rowActionLoading = { ...rowActionLoading, [row.row_index]: false };
    }
  }

  async function postMappedRows() {
    if (!importItem) {
      return;
    }
    rowActionError = "";
    rowActionMessage = "";
    bulkPosting = true;
    try {
      const response = await apiFetch<{
        posted: number;
        skipped: number;
        failed: number;
      }>(`/imports/${importItem.id}/rows/post-mapped`, {
        method: "POST",
      });
      rowActionMessage = `Posted ${response.posted}, skipped ${response.skipped}, failed ${response.failed}.`;
      pushNotification(rowActionMessage, response.failed > 0 ? "error" : "success");
      await loadImport();
    } catch (err) {
      rowActionError =
        errorMessage(err, "Unable to post mapped rows.");
      pushNotification(rowActionError, "error");
    } finally {
      bulkPosting = false;
    }
  }

  async function bulkAssignAccount() {
    if (!importItem || !bulkAccountId || selectedRowIndexes.length === 0) {
      return;
    }
    rowActionError = "";
    rowActionMessage = "";
    bulkAssigning = true;
    try {
      let updatedCount = 0;
      for (const rowIndex of selectedRowIndexes) {
        const row = importRows.find((item) => item.row_index === rowIndex);
        if (!row || row.status === "posted") {
          continue;
        }
        const updated = await apiFetch<ImportRow>(
          `/imports/${importItem.id}/rows/${row.row_index}`,
          {
            method: "PATCH",
            body: { account_id: bulkAccountId },
          },
        );
        importRows = importRows.map((item) =>
          item.row_index === updated.row_index ? updated : item,
        );
        updatedCount += 1;
      }
      rowActionMessage = `Assigned ${updatedCount} row(s) to ${accountName(bulkAccountId)}.`;
      pushNotification(rowActionMessage, "success");
      clearSelectedRows();
    } catch (err) {
      rowActionError =
        errorMessage(err, "Unable to assign selected rows.");
      pushNotification(rowActionError, "error");
    } finally {
      bulkAssigning = false;
    }
  }

  async function createVendorRule(row: ImportRow) {
    if (!importItem || !row.account_id || !row.vendor) {
      return;
    }
    rowActionError = "";
    rowActionLoading = { ...rowActionLoading, [row.row_index]: true };
    try {
      await apiFetch("/vendor-rules", {
        method: "POST",
        body: {
          entity_id: importItem.entity_id,
          match_type: "contains",
          pattern: row.vendor,
          account_id: row.account_id,
        },
      });
      const message = `Rule created for ${row.vendor}.`;
      rowActionMessage = message;
      pushNotification(message, "success");
    } catch (err) {
      rowActionError =
        errorMessage(err, "Unable to create vendor rule.");
      pushNotification(rowActionError, "error");
    } finally {
      rowActionLoading = { ...rowActionLoading, [row.row_index]: false };
    }
  }

  onMount(() => {
    loadImport();
  });
</script>

<section class="grid gap-6">
  <a class="text-sm text-muted hover:text-ink" href="/imports"
    >← Back to imports</a
  >

  {#if loading}
    <p class="text-sm text-muted">Loading import…</p>
  {:else if error}
    <p
      class="status-message-sm status-error"
    >
      {error}
    </p>
  {:else if importItem}
    <div class="grid gap-4 md:grid-cols-[1.2fr_1fr]">
      <div class="rounded-2xl border border-line bg-surface p-6">
        <h2 class="text-lg font-semibold">Import details</h2>
        <div class="mt-4 grid gap-3 text-sm text-muted">
          <div
            class="flex items-center justify-between rounded-xl border border-line px-4 py-3"
          >
            <span>Status</span>
            <span class="font-semibold text-ink">{importItem.status}</span>
          </div>
          <div
            class="flex items-center justify-between rounded-xl border border-line px-4 py-3"
          >
            <span>Original name</span>
            <span class="font-semibold text-ink"
              >{importItem.original_name ?? importItem.id}</span
            >
          </div>
          <div
            class="flex items-center justify-between rounded-xl border border-line px-4 py-3"
          >
            <span>Size</span>
            <span class="font-semibold text-ink"
              >{formatBytes(importItem.size_bytes)}</span
            >
          </div>
          <div
            class="flex items-center justify-between rounded-xl border border-line px-4 py-3"
          >
            <span>Uploaded</span>
            <span class="font-semibold text-ink"
              >{new Date(importItem.uploaded_at).toLocaleString("en-US")}</span
            >
          </div>
          {#if importItem.suggestion_context}
            <div class="rounded-xl border border-line px-4 py-3">
              <p class="text-xs uppercase tracking-[0.2em] text-muted">
                Suggestion context
              </p>
              <p class="mt-2 text-sm text-ink">
                {importItem.suggestion_context}
              </p>
            </div>
          {/if}
          {#if importItem.url}
            <a
              class="inline-flex items-center gap-2 text-sm font-semibold text-ink underline"
              href={importItem.url}
              target="_blank"
              rel="noreferrer"
            >
              Open import file
            </a>
          {/if}
          <div class="rounded-xl border border-line px-4 py-3">
            <p class="text-xs uppercase tracking-[0.2em] text-muted">Review status</p>
            <div class="mt-2 grid gap-2 text-sm text-muted sm:grid-cols-2">
              <p>Total rows: <span class="font-semibold text-ink">{rowCounts.total}</span></p>
              <p>Mapped rows: <span class="font-semibold text-ink">{rowCounts.mapped}</span></p>
              <p>Posted rows: <span class="font-semibold text-ink">{rowCounts.posted}</span></p>
              <p>Needs posting: <span class="font-semibold text-ink">{Math.max(rowCounts.mapped - rowCounts.posted, 0)}</span></p>
              <p>Duplicate-suspect rows: <span class="font-semibold text-ink">{duplicateRows.length}</span></p>
            </div>
            {#if duplicateRows.length > 0}
              <p class="mt-2 text-xs text-muted">
                Duplicate fingerprints appear on rows {duplicateRows.map((row) => row.row_index).join(', ')}.
              </p>
            {/if}
            <div class="mt-3 flex flex-wrap gap-2">
              <a class="rounded-full border border-line px-3 py-1 text-xs font-semibold" href="/review">
                Open review queue
              </a>
              <a class="rounded-full border border-line px-3 py-1 text-xs font-semibold" href="/exports">
                Open exports
              </a>
            </div>
          </div>
        </div>
      </div>

      <div class="grid gap-4">
        <div class="rounded-2xl border border-line bg-surface p-6">
          <h2 class="text-lg font-semibold">OCR</h2>
          <p class="mt-2 text-sm text-muted">
            Latest OCR results for this import.
          </p>
          {#if ocrHistory.length > 0}
            <div class="mt-4 grid gap-3 text-sm">
              <div class="rounded-xl border border-line px-4 py-3">
                <p class="text-xs uppercase tracking-[0.2em] text-muted">
                  Status
                </p>
                <p class="mt-2 font-semibold">{ocrHistory[0].status}</p>
                <p class="mt-1 text-xs text-muted">
                  {ocrHistory[0].provider === "import"
                    ? "Provided text"
                    : "OCR output"}
                </p>
              </div>
              {#if ocrHistory[0].raw_text}
                <div
                  class="rounded-xl border border-line px-4 py-3 text-xs text-muted"
                >
                  <p class="text-[10px] uppercase tracking-[0.2em] text-muted">
                    Extracted text
                  </p>
                  <pre class="mt-2 whitespace-pre-wrap text-ink">{ocrHistory[0]
                      .raw_text}</pre>
                </div>
              {/if}
              {#if ocrHistory[0].error}
                <p
                  class="status-message-xs status-error"
                >
                  {ocrHistory[0].error}
                </p>
              {/if}
            </div>
          {:else}
            <p class="mt-4 text-sm text-muted">No OCR runs yet.</p>
          {/if}
          <div class="mt-4 flex flex-wrap gap-3">
            <button
              class="rounded-full border border-ink/20 px-5 py-2 text-sm font-semibold disabled:opacity-60"
              type="button"
              disabled={ocrRerunLoading}
              onclick={rerunOCR}
            >
              {ocrRerunLoading ? "Re-running…" : "Rerun OCR"}
            </button>
          </div>
          {#if ocrMessage}
            <p class="mt-3 text-xs text-muted">{ocrMessage}</p>
          {/if}
        </div>

        <div class="rounded-2xl border border-line bg-surface p-6">
          <h2 class="text-lg font-semibold">Suggestion</h2>
          <p class="mt-2 text-sm text-muted">
            Latest suggestion run for this import.
          </p>
          {#if suggestions.length > 0}
            <div class="mt-4 grid gap-3 text-sm">
              <div class="rounded-xl border border-line px-4 py-3">
                <p class="text-xs uppercase tracking-[0.2em] text-muted">
                  Status
                </p>
                <p class="mt-2 font-semibold">{suggestions[0].status}</p>
              </div>
              <div class="rounded-xl border border-line px-4 py-3">
                <p class="text-xs uppercase tracking-[0.2em] text-muted">
                  Confidence
                </p>
                <p class="mt-2 font-semibold">
                  {suggestions[0].confidence
                    ? `${Math.round(suggestions[0].confidence * 100)}%`
                    : "—"}
                </p>
              </div>
              <div class="rounded-xl border border-line px-4 py-3">
                <p class="text-xs uppercase tracking-[0.2em] text-muted">
                  Estimated cost
                </p>
                <p class="mt-2 font-semibold">
                  {formatCost(suggestions[0].cost_cents)}
                </p>
                {#if suggestions[0].total_tokens}
                  <p class="mt-1 text-xs text-muted">
                    {suggestions[0].prompt_tokens ?? 0} prompt + {suggestions[0]
                      .completion_tokens ?? 0} completion ({suggestions[0]
                      .total_tokens} total)
                  </p>
                {/if}
              </div>
              {#if latestImportSummary}
                <div class="rounded-xl border border-line px-4 py-3">
                  <p class="text-xs uppercase tracking-[0.2em] text-muted">
                    Import summary
                  </p>
                  <div class="mt-2 grid gap-1 text-sm text-muted">
                    <p>
                      Rows: <span class="font-semibold text-ink"
                        >{latestImportSummary.row_count}</span
                      >
                    </p>
                    <p>
                      Parsed amounts:
                      <span class="font-semibold text-ink"
                        >{latestImportSummary.parsed_rows}</span
                      >
                    </p>
                    <p>
                      Total:
                      <span class="font-semibold text-ink"
                        >{formatCost(latestImportSummary.total_cents)}</span
                      >
                    </p>
                    {#if latestImportSummary.top_vendor}
                      <p>
                        Top vendor: <span class="font-semibold text-ink"
                          >{latestImportSummary.top_vendor}</span
                        >
                      </p>
                    {/if}
                  </div>
                  {#if latestImportSummary.top_vendors.length > 0}
                    <div class="mt-3 grid gap-2 text-xs text-muted">
                      {#each latestImportSummary.top_vendors as topVendor}
                        <div class="rounded-lg border border-line/80 px-3 py-2">
                          <p class="text-sm font-semibold text-ink">
                            {topVendor.vendor}
                          </p>
                          <p>
                            {topVendor.count} rows • {formatCost(
                              topVendor.total_cents,
                            )}
                          </p>
                        </div>
                      {/each}
                    </div>
                  {/if}
                </div>
              {/if}
              {#if latestImportRows.length > 0}
                <div class="rounded-xl border border-line px-4 py-3">
                  <div class="flex items-center justify-between gap-2">
                    <p class="text-xs uppercase tracking-[0.2em] text-muted">
                      Row mapping preview
                    </p>
                    <p class="text-xs text-muted">
                      {latestImportRows.filter((row) => row.account_id).length} mapped
                      / {latestImportRows.length}
                    </p>
                  </div>
                  <div class="mt-3 overflow-x-auto">
                    <table class="min-w-full text-xs text-muted">
                      <thead>
                        <tr
                          class="border-b border-line text-left text-[10px] uppercase tracking-[0.14em]"
                        >
                          <th class="px-2 py-2">Row</th>
                          <th class="px-2 py-2">Vendor</th>
                          <th class="px-2 py-2">Date</th>
                          <th class="px-2 py-2">Amount</th>
                          <th class="px-2 py-2">Account</th>
                          <th class="px-2 py-2">Rule</th>
                        </tr>
                      </thead>
                      <tbody>
                        {#each latestImportRows as row}
                          <tr class="border-b border-line/70 align-top">
                            <td class="px-2 py-2 text-ink">{row.row_index}</td>
                            <td class="px-2 py-2 text-ink"
                              >{row.vendor ?? "—"}</td
                            >
                            <td class="px-2 py-2">{row.date ?? "—"}</td>
                            <td class="px-2 py-2 text-ink"
                              >{formatCost(row.amount_cents)}</td
                            >
                            <td class="px-2 py-2">
                              {#if row.account_id}
                                <span class="font-semibold text-ink"
                                  >{row.account_id}</span
                                >
                              {:else}
                                <span class="text-warning">Unmapped</span>
                              {/if}
                            </td>
                            <td class="px-2 py-2">
                              {#if row.rule_match_type || row.rule_pattern}
                                <span class="text-ink"
                                  >{row.rule_match_type ?? "match"}: {row.rule_pattern ??
                                    "—"}</span
                                >
                              {:else}
                                —
                              {/if}
                            </td>
                          </tr>
                        {/each}
                      </tbody>
                    </table>
                  </div>
                </div>
              {/if}
            </div>
          {:else}
            <p class="mt-4 text-sm text-muted">No suggestions yet.</p>
          {/if}
          <div class="mt-6 flex flex-wrap gap-3">
            <button
              class="rounded-full border border-ink/20 px-5 py-2 text-sm font-semibold disabled:opacity-60"
              type="button"
              disabled={rerunning}
              onclick={rerunSuggestion}
            >
              {rerunning ? "Re-running…" : "Rerun suggestion"}
            </button>
          </div>
          {#if rerunError}
            <p
              class="mt-3 status-message-sm status-error"
            >
              {rerunError}
            </p>
          {/if}
          {#if rerunSuccess}
            <p
              class="mt-3 status-message-sm status-success"
            >
              {rerunSuccess}
            </p>
          {/if}
        </div>
      </div>
    </div>

    <div class="rounded-2xl border border-line bg-surface p-6">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 class="text-lg font-semibold">Import rows</h2>
          <p class="mt-2 text-sm text-muted">
            Assign accounts and post rows when you want transaction-level books.
          </p>
        </div>
        <button
          class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-paper disabled:opacity-60"
          type="button"
          disabled={bulkPosting || importRows.filter((row) => row.account_id && row.status !== "posted").length === 0}
          onclick={postMappedRows}
        >
          {bulkPosting ? "Posting…" : "Post mapped rows"}
        </button>
      </div>
      {#if rowActionError}
        <p
          class="mt-4 status-message-sm status-error"
        >
          {rowActionError}
        </p>
      {/if}
      {#if rowActionMessage}
        <p
          class="mt-4 status-message-sm status-success"
        >
          {rowActionMessage}
        </p>
      {/if}
      {#if importRows.length > 0}
        <div class="mt-4 grid gap-3 text-sm sm:grid-cols-4">
          <div class="rounded-xl border border-line px-4 py-3">
            <p class="text-[10px] uppercase tracking-[0.16em] text-muted">Rows</p>
            <p class="mt-1 font-semibold text-ink">{importRows.length}</p>
          </div>
          <div class="rounded-xl border border-line px-4 py-3">
            <p class="text-[10px] uppercase tracking-[0.16em] text-muted">Mapped</p>
            <p class="mt-1 font-semibold text-ink">
              {importRows.filter((row) => row.account_id).length}
            </p>
          </div>
          <div class="rounded-xl border border-line px-4 py-3">
            <p class="text-[10px] uppercase tracking-[0.16em] text-muted">Posted</p>
            <p class="mt-1 font-semibold text-ink">
              {importRows.filter((row) => row.status === "posted").length}
            </p>
          </div>
          <div class="rounded-xl border border-line px-4 py-3">
            <p class="text-[10px] uppercase tracking-[0.16em] text-muted">Unposted</p>
            <p class="mt-1 font-semibold text-ink">
              {importRows.filter((row) => row.status !== "posted").length}
            </p>
          </div>
        </div>
        <div class="mt-4 grid gap-3 rounded-xl border border-line px-4 py-3 text-sm md:grid-cols-[0.9fr_1fr_1fr_auto_auto] md:items-end">
          <label class="grid gap-2 font-medium text-ink">
            Search rows
            <input
              class="rounded-xl border border-line px-3 py-2 text-sm"
              type="text"
              placeholder="Vendor, memo, date"
              bind:value={rowSearch}
            />
          </label>
          <label class="grid gap-2 font-medium text-ink">
            Status
            <select class="rounded-xl border border-line px-3 py-2 text-sm" bind:value={rowFilter}>
              <option value="all">All rows</option>
              <option value="unmapped">Unmapped</option>
              <option value="mapped">Mapped</option>
              <option value="unposted">Unposted</option>
              <option value="posted">Posted</option>
            </select>
          </label>
          <label class="grid gap-2 font-medium text-ink">
            Assign selected
            <select class="rounded-xl border border-line px-3 py-2 text-sm" bind:value={bulkAccountId}>
              <option value="">Select account</option>
              {#each accounts as account}
                <option value={account.id}>{account.name}</option>
              {/each}
            </select>
          </label>
          <button
            class="rounded-full border border-ink/20 px-4 py-2 text-sm font-semibold disabled:opacity-60"
            type="button"
            disabled={bulkAssigning || !bulkAccountId || selectedRowIndexes.length === 0}
            onclick={bulkAssignAccount}
          >
            {bulkAssigning ? "Assigning…" : `Assign ${selectedRowIndexes.length}`}
          </button>
          <button
            class="rounded-full border border-ink/20 px-4 py-2 text-sm font-semibold"
            type="button"
            onclick={clearSelectedRows}
          >
            Clear
          </button>
        </div>
      {/if}
      {#if importRows.length === 0}
        <p class="mt-4 text-sm text-muted">
          No persisted rows yet. Rerun suggestion after processing this import.
        </p>
      {:else}
        <div class="mt-4 overflow-x-auto">
          <table class="min-w-full text-xs text-muted">
            <thead>
              <tr class="border-b border-line text-left text-[10px] uppercase tracking-[0.14em]">
                <th class="px-2 py-2">
                  <input
                    type="checkbox"
                    aria-label="Select visible rows"
                    checked={filteredImportRows.length > 0 && filteredImportRows.filter((row) => row.status !== "posted").every((row) => selectedRows[row.row_index])}
                    onchange={(event) => toggleVisibleRows(event.currentTarget.checked)}
                  />
                </th>
                <th class="px-2 py-2">Row</th>
                <th class="px-2 py-2">Date</th>
                <th class="px-2 py-2">Vendor</th>
                <th class="px-2 py-2">Amount</th>
                <th class="px-2 py-2">Account</th>
                <th class="px-2 py-2">Status</th>
                <th class="px-2 py-2"></th>
              </tr>
            </thead>
            <tbody>
              {#each filteredImportRows as row}
                <tr id={`row-${row.row_index}`} class="border-b border-line/70 align-top">
                  <td class="px-2 py-2">
                    <input
                      type="checkbox"
                      aria-label={`Select row ${row.row_index}`}
                      checked={!!selectedRows[row.row_index]}
                      disabled={row.status === "posted"}
                      onchange={(event) =>
                        (selectedRows = {
                          ...selectedRows,
                          [row.row_index]: event.currentTarget.checked,
                        })}
                    />
                  </td>
                  <td class="px-2 py-2 text-ink">{row.row_index}</td>
                  <td class="px-2 py-2">{row.date}</td>
                  <td class="px-2 py-2 text-ink">
                    <p class="font-semibold">{row.vendor}</p>
                    {#if row.memo && row.memo !== row.vendor}
                      <p class="mt-1 text-[11px] text-muted">{row.memo}</p>
                    {/if}
                    {#if duplicateRows.some((item) => item.row_index === row.row_index)}
                      <p class="mt-1 text-[10px] font-semibold uppercase tracking-[0.12em] text-warning">
                        Duplicate-suspect
                      </p>
                    {/if}
                  </td>
                  <td class="px-2 py-2 text-ink">
                    {formatCost(row.amount_cents)}
                    <p class="mt-1 text-[10px] uppercase tracking-[0.12em] text-muted">
                      {row.direction}
                    </p>
                  </td>
                  <td class="px-2 py-2">
                    {#if row.status === "posted"}
                      <span class="font-semibold text-ink">{accountName(row.account_id)}</span>
                    {:else}
                      <select
                        class="min-w-40 rounded-xl border border-line px-2 py-1 text-xs"
                        value={row.account_id ?? ""}
                        onchange={(event) =>
                          assignRowAccount(row, event.currentTarget.value)}
                        disabled={rowActionLoading[row.row_index]}
                      >
                        <option value="">Select account</option>
                        {#each accounts as account}
                          <option value={account.id}>{account.name}</option>
                        {/each}
                      </select>
                    {/if}
                  </td>
                  <td class="px-2 py-2">{row.status}</td>
                  <td class="px-2 py-2 text-right">
                    <div class="flex flex-wrap justify-end gap-2">
                      <button
                        class="rounded-full border border-ink/20 px-3 py-1 text-xs font-semibold disabled:opacity-60"
                        type="button"
                        disabled={!row.account_id || row.status === "posted" || rowActionLoading[row.row_index]}
                        onclick={() => postImportRow(row)}
                      >
                        {rowActionLoading[row.row_index]
                          ? "Working…"
                          : row.status === "posted"
                            ? "Posted"
                            : "Post"}
                      </button>
                      <button
                        class="rounded-full border border-ink/20 px-3 py-1 text-xs font-semibold disabled:opacity-60"
                        type="button"
                        disabled={!row.account_id || !row.vendor || rowActionLoading[row.row_index]}
                        onclick={() => createVendorRule(row)}
                      >
                        Rule
                      </button>
                    </div>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
          {#if filteredImportRows.length === 0}
            <p class="mt-4 text-sm text-muted">No rows match the current filter.</p>
          {/if}
        </div>
      {/if}
    </div>

    <div class="rounded-2xl border border-line bg-surface p-6">
      <h2 class="text-lg font-semibold">Suggestion history</h2>
      {#if suggestions.length === 0}
        <p class="mt-4 text-sm text-muted">No suggestion history yet.</p>
      {:else}
        <div class="mt-4 grid gap-3">
          {#each suggestions as suggestion}
            <div
              class="grid gap-2 rounded-xl border border-line px-4 py-3 md:grid-cols-[1fr_1fr_1fr] md:items-center"
            >
              <div>
                <p class="text-sm font-semibold">{suggestion.status}</p>
                <p class="text-xs text-muted">
                  {new Date(suggestion.created_at).toLocaleString("en-US")}
                </p>
              </div>
              <div class="text-sm text-muted">
                Confidence: {suggestion.confidence
                  ? `${Math.round(suggestion.confidence * 100)}%`
                  : "—"}
              </div>
              <div class="text-sm text-muted">
                Cost: {formatCost(suggestion.cost_cents)}
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}
</section>


