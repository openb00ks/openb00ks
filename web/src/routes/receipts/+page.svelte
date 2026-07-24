<script lang="ts">
  import { errorMessage } from "$lib/utils/errors";
  import { formatShortDate as formatDate } from '$lib/utils/date';
  import { formatCents } from '$lib/utils/money';
  import { browser } from "$app/environment";
  import { activeEntity } from "$lib/stores/entity";
  import { apiFetch } from "$lib/api/client";
  import { pushNotification } from "$lib/stores/notifications";
  import {
    readFilterPreference,
    writeFilterPreference,
  } from "$lib/utils/filter-preferences";

  type ReceiptRow = {
    id: string;
    entity_id: string;
    status: string;
    total_cents?: number;
    uploaded_at: string;
    original_name?: string;
  };

  let receipts: ReceiptRow[] = $state([]);
  let loading = $state(false);
  let listError = $state("");
  let statusFilter = $state(
    browser ? readFilterPreference("receipts.status_filter", "") : "",
  );
  let searchQuery = $state(
    browser ? readFilterPreference("receipts.search_query", "") : "",
  );
  let loadPromise: Promise<void> | null = null;

  let filteredReceipts = $derived(
    receipts.filter((receipt) => {
      const search = searchQuery.trim().toLowerCase();
      if (!search) {
        return true;
      }
      const haystack =
        `${receipt.original_name ?? ""} ${receipt.id}`.toLowerCase();
      return haystack.includes(search);
    }),
  );
  let activeFilterLabel = $derived(
    statusFilter
      ? statusFilter === "needs_attention"
        ? "Needs attention"
        : statusFilter === "ready_for_review"
          ? "Ready for review"
          : statusFilter === "posted"
            ? "Posted"
            : statusFilter
      : "All receipts",
  );

  let file: File | null = null;
  let fileInput: HTMLInputElement | null = $state(null);
  let totalCents = $state("");
  let tagInput = $state("");
  let suggestionContext = $state("");
  let uploadError = $state("");
  let uploadSuccess = $state("");
  let uploading = $state(false);

  function handleFileChange(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    file = input.files?.[0] ?? null;
  }

  async function applyStatusFilter(next: string) {
    statusFilter = next;
    writeFilterPreference("receipts.status_filter", statusFilter);
    await loadReceipts();
  }

  function updateSearch(value: string) {
    searchQuery = value;
    writeFilterPreference("receipts.search_query", searchQuery);
  }

  async function loadReceipts() {
    if (loadPromise) {
      return;
    }
    if (!$activeEntity) {
      receipts = [];
      return;
    }
    loading = true;
    listError = "";
    loadPromise = (async () => {
      try {
        const response = await apiFetch<{ rows: ReceiptRow[] }>(
          `/receipts?entity_id=${encodeURIComponent($activeEntity)}${statusFilter ? `&status=${encodeURIComponent(statusFilter)}` : ""}`,
        );
        receipts = response.rows ?? [];
      } catch (err) {
        listError =
          errorMessage(err, "Unable to load receipts.");
      } finally {
        loading = false;
        loadPromise = null;
      }
    })();
    await loadPromise;
  }

  async function submitReceipt() {
    uploadError = "";
    uploadSuccess = "";
    if (!file) {
      uploadError = "Choose a receipt file to upload.";
      return;
    }
    if (!$activeEntity) {
      uploadError = "Select an entity before uploading.";
      return;
    }

    const formData = new FormData();
    formData.append("file", file);
    formData.append("entity_id", $activeEntity);
    if (totalCents.trim()) {
      const parsed = Number.parseInt(totalCents.trim(), 10);
      if (Number.isNaN(parsed) || parsed < 0) {
        uploadError = "Total cents must be a valid number.";
        return;
      }
      formData.append("total_cents", String(parsed));
    }
    if (tagInput.trim()) {
      const tags = tagInput
        .split(",")
        .map((tag) => tag.trim())
        .filter((tag) => tag.length > 0);
      for (const tag of tags) {
        formData.append("tags[]", tag);
      }
    }
    if (suggestionContext.trim()) {
      formData.append("suggestion_context", suggestionContext.trim());
    }

    uploading = true;
    try {
      await apiFetch<{ id?: string; status?: string }>("/receipts", {
        method: "POST",
        body: formData,
      });
      await loadReceipts();
      uploadSuccess = "Receipt uploaded and queued for processing.";
      pushNotification(uploadSuccess, "success");
      file = null;
      totalCents = "";
      tagInput = "";
      suggestionContext = "";
      if (fileInput) {
        fileInput.value = "";
      }
    } catch (err) {
      uploadError = errorMessage(err, "Upload failed.");
      pushNotification(uploadError, "error");
    } finally {
      uploading = false;
    }
  }

  $effect(() => {
    if ($activeEntity) {
      loadReceipts();
    }
  });

  $effect(() => {
    if (!$activeEntity) {
      receipts = [];
      listError = "";
    }
  });
</script>

<section class="grid gap-6">
  <div class="flex items-center justify-between">
    <div>
      <h1 class="text-2xl font-semibold tracking-tight">Receipts</h1>
      <p class="mt-2 text-sm text-muted">
        Monitor captured receipts, then jump into review when items are ready.
      </p>
    </div>
    <a
      class="rounded-full bg-primary px-5 py-2 text-sm font-semibold text-paper"
      href="#receipt-upload"
    >
      Upload receipt
    </a>
  </div>

  <div class="rounded-2xl border border-line bg-surface p-6">
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <p class="text-xs uppercase tracking-[0.2em] text-muted">
          Current scope
        </p>
        <h2 class="mt-2 text-lg font-semibold">{activeFilterLabel}</h2>
        <p class="mt-2 text-sm text-muted">
          Keep the list in view while using filters to work through receipts by
          state.
        </p>
      </div>
      <div class="flex flex-wrap gap-2">
        <button
          class={`rounded-full px-4 py-2 text-sm font-semibold ${statusFilter === "needs_attention" ? "bg-primary text-paper" : "border border-line text-ink"}`}
          type="button"
          onclick={() => applyStatusFilter("needs_attention")}
        >
          Needs attention
        </button>
        <button
          class={`rounded-full px-4 py-2 text-sm font-semibold ${statusFilter === "ready_for_review" ? "bg-primary text-paper" : "border border-line text-ink"}`}
          type="button"
          onclick={() => applyStatusFilter("ready_for_review")}
        >
          Ready for review
        </button>
        <button
          class={`rounded-full px-4 py-2 text-sm font-semibold ${statusFilter === "posted" ? "bg-primary text-paper" : "border border-line text-ink"}`}
          type="button"
          onclick={() => applyStatusFilter("posted")}
        >
          Posted
        </button>
        <button
          class={`rounded-full px-4 py-2 text-sm font-semibold ${statusFilter === "" ? "bg-primary text-paper" : "border border-line text-ink"}`}
          type="button"
          onclick={() => applyStatusFilter("")}
        >
          All
        </button>
      </div>
    </div>
    <div class="mt-4">
      <label class="grid gap-2 text-sm font-medium text-ink">
        Search receipts
        <input
          class="rounded-xl border border-line px-3 py-2 text-sm"
          type="text"
          placeholder="Search by file name"
          bind:value={searchQuery}
          oninput={(event) => updateSearch(event.currentTarget.value)}
        />
      </label>
    </div>
  </div>

  <div class="grid gap-4 lg:grid-cols-[1.55fr_0.95fr]">
    <div class="rounded-2xl border border-line bg-surface p-6">
      <div class="flex items-center justify-between gap-3">
        <div>
          <h2 class="text-lg font-semibold">Recent receipts</h2>
          <p class="mt-1 text-sm text-muted">
            The list stays primary so you can monitor work in progress.
          </p>
        </div>
        <span class="text-xs uppercase tracking-[0.2em] text-muted">Live</span>
      </div>
      {#if listError}
        <p
          class="mt-4 status-message-sm status-error"
        >
          {listError}
        </p>
      {:else if loading}
        <p class="mt-4 text-sm text-muted">Loading receipts…</p>
      {:else if !$activeEntity}
        <div
          class="mt-4 rounded-2xl border border-dashed border-line-strong bg-paper px-4 py-5 text-sm text-muted"
        >
          <p class="font-semibold text-ink">
            Select an entity to view receipts.
          </p>
          <p class="mt-2">
            Receipt work is scoped to the active entity before you can upload or
            review anything.
          </p>
        </div>
      {:else if filteredReceipts.length === 0}
        <div
          class="mt-4 rounded-2xl border border-dashed border-line-strong bg-paper px-4 py-5 text-sm text-muted"
        >
          {#if searchQuery.trim()}
            <p class="font-semibold text-ink">No receipts match this search.</p>
            <p class="mt-2">
              Try a different file name search or clear the search field.
            </p>
          {:else if statusFilter}
            <p class="font-semibold text-ink">No receipts match this filter.</p>
            <p class="mt-2">
              Try a different status filter or switch back to all receipts.
            </p>
          {:else}
            <p class="font-semibold text-ink">No receipts yet.</p>
            <p class="mt-2">
              Upload the first receipt for this entity to start the review and
              posting workflow.
            </p>
          {/if}
        </div>
      {:else}
        <div class="mt-4 grid gap-3">
          {#each filteredReceipts as receipt}
            <a
              class="grid gap-3 rounded-xl border border-line px-4 py-4 hover:border-line-strong sm:grid-cols-[1.2fr_0.45fr_0.45fr] sm:items-center"
              href={`/receipts/${receipt.id}`}
            >
              <div>
                <p class="text-sm font-semibold">
                  {receipt.original_name ?? receipt.id}
                </p>
                <p class="mt-1 text-xs text-muted">
                  {formatDate(receipt.uploaded_at)}
                </p>
                {#if receipt.status === "ready_for_review"}
                  <p
                    class="mt-1 text-[10px] font-semibold uppercase tracking-[0.16em] text-success"
                  >
                    Draft ready
                  </p>
                {/if}
              </div>
              <div class="text-sm">
                <p class="text-[10px] uppercase tracking-[0.2em] text-muted">
                  Status
                </p>
                <p class="mt-1 font-semibold">{receipt.status.replaceAll('_', ' ')}</p>
              </div>
              <div class="text-sm sm:text-right">
                <p class="text-[10px] uppercase tracking-[0.2em] text-muted">
                  Amount
                </p>
                <p class="mt-1 font-semibold">
                  {receipt.total_cents !== undefined
                    ? formatCents(receipt.total_cents)
                    : "—"}
                </p>
              </div>
            </a>
          {/each}
        </div>
      {/if}
    </div>

    <div class="grid gap-4">
      <div
        id="receipt-upload"
        class="rounded-2xl border border-dashed border-line-strong bg-surface p-6"
      >
        <form
          class="grid gap-3"
          onsubmit={(event) => {
            event.preventDefault();
            void submitReceipt();
          }}
        >
          <div>
            <h2 class="text-lg font-semibold">Upload shortcut</h2>
            <p class="mt-2 text-sm text-muted">
              Capture a receipt without leaving the operational queue.
            </p>
          </div>
          <label class="grid gap-2 text-sm font-medium">
            Receipt file
            <input
              class="rounded-xl border border-line px-3 py-2 text-sm"
              type="file"
              accept=".jpg,.jpeg,.png,.pdf,image/*,application/pdf"
              bind:this={fileInput}
              onchange={handleFileChange}
              required
            />
          </label>
          <details class="rounded-xl border border-line px-4 py-3">
            <summary class="cursor-pointer text-sm font-semibold text-ink"
              >Add details</summary
            >
            <div class="mt-3 grid gap-3">
              <label class="grid gap-2 text-sm font-medium">
                Total cents (optional)
                <input
                  class="rounded-xl border border-line px-3 py-2 text-sm"
                  type="number"
                  min="0"
                  step="1"
                  bind:value={totalCents}
                />
              </label>
              <label class="grid gap-2 text-sm font-medium">
                Tags (comma-separated)
                <input
                  class="rounded-xl border border-line px-3 py-2 text-sm"
                  type="text"
                  placeholder="travel, client, meals"
                  bind:value={tagInput}
                />
              </label>
              <label class="grid gap-2 text-sm font-medium">
                Suggestion context (optional)
                <textarea
                  class="min-h-24 rounded-xl border border-line px-3 py-2 text-sm"
                  placeholder="Any details that help categorize this receipt."
                  bind:value={suggestionContext}
                ></textarea>
              </label>
            </div>
          </details>
          {#if uploadError}
            <p
              class="status-message-sm status-error"
            >
              {uploadError}
            </p>
          {/if}
          {#if uploadSuccess}
            <p
              class="status-message-sm status-success"
            >
              {uploadSuccess}
            </p>
          {/if}
          <button
            class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-paper disabled:opacity-60"
            type="submit"
            disabled={uploading || !$activeEntity}
          >
            {uploading ? "Uploading…" : "Upload receipt"}
          </button>
          {#if !$activeEntity}
            <p class="text-xs text-muted">
              Select an entity to enable uploads.
            </p>
          {:else}
            <p class="text-xs text-muted">
              Supports JPG, PNG, and PDF up to 10 MB.
            </p>
          {/if}
        </form>
      </div>

      <div class="rounded-2xl border border-line bg-surface p-6">
        <h2 class="text-lg font-semibold">Working guidance</h2>
        <div class="mt-4 grid gap-3 text-sm text-muted">
          <div class="rounded-xl border border-line px-4 py-3">
            <p class="font-semibold text-ink">1. Capture</p>
            <p class="mt-1">
              Upload a file quickly, then add details only when they help.
            </p>
          </div>
          <div class="rounded-xl border border-line px-4 py-3">
            <p class="font-semibold text-ink">2. Review</p>
            <p class="mt-1">
              Use status filters to work through receipts that are ready or
              blocked.
            </p>
          </div>
          <div class="rounded-xl border border-line px-4 py-3">
            <p class="font-semibold text-ink">3. Post manually</p>
            <p class="mt-1">
              Receipts are not posted automatically. Confirm the draft before
              posting.
            </p>
          </div>
        </div>
      </div>
    </div>
  </div>
</section>

