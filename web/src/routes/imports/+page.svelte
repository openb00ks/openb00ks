<script lang="ts">
  import { errorMessage } from "$lib/utils/errors";
  import { formatShortDate as formatDate } from '$lib/utils/date';
  import { browser } from "$app/environment";
  import { activeEntity } from "$lib/stores/entity";
  import { apiFetch } from "$lib/api/client";
  import { pushNotification } from "$lib/stores/notifications";
  import {
    readFilterPreference,
    writeFilterPreference,
  } from "$lib/utils/filter-preferences";

  type ImportRow = {
    id: string;
    entity_id: string;
    status: string;
    content_type: string;
    size_bytes: number;
    uploaded_at: string;
    original_name?: string;
  };

  let imports: ImportRow[] = $state([]);
  let loading = $state(false);
  let listError = $state("");
  let statusFilter = $state(
    browser ? readFilterPreference("imports.status_filter", "") : "",
  );
  let searchQuery = $state(
    browser ? readFilterPreference("imports.search_query", "") : "",
  );
  let loadPromise: Promise<void> | null = null;
  let filteredImports = $derived(
    imports.filter((item) => {
      const search = searchQuery.trim().toLowerCase();
      if (!search) {
        return true;
      }
      const haystack = `${item.original_name ?? ""} ${item.id}`.toLowerCase();
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
      : "All imports",
  );

  let uploadMode: "file" | "text" = $state("file");
  let file: File | null = null;
  let fileInput: HTMLInputElement | null = $state(null);
  let importText = $state("");
  let filename = $state("import.csv");
  let contentType = $state("text/csv");
  let suggestionContext = $state("");
  let uploadError = $state("");
  let uploadSuccess = $state("");
  let uploading = $state(false);

  function handleFileChange(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    file = input.files?.[0] ?? null;
  }

  function formatBytes(size: number) {
    if (size < 1024) {
      return `${size} B`;
    }
    if (size < 1024 * 1024) {
      return `${(size / 1024).toFixed(1)} KB`;
    }
    return `${(size / 1024 / 1024).toFixed(1)} MB`;
  }

  async function applyStatusFilter(next: string) {
    statusFilter = next;
    writeFilterPreference("imports.status_filter", statusFilter);
    await loadImports();
  }

  function updateSearch(value: string) {
    searchQuery = value;
    writeFilterPreference("imports.search_query", searchQuery);
  }

  async function loadImports() {
    if (loadPromise) {
      return;
    }
    if (!$activeEntity) {
      imports = [];
      return;
    }
    loading = true;
    listError = "";
    loadPromise = (async () => {
      try {
        const query = new URLSearchParams({
          entity_id: $activeEntity,
          status: statusFilter,
        });
        const response = await apiFetch<{ rows: ImportRow[] }>(
          `/imports?${query.toString()}`,
        );
        imports = response.rows ?? [];
      } catch (err) {
        listError =
          errorMessage(err, "Unable to load imports.");
      } finally {
        loading = false;
        loadPromise = null;
      }
    })();
    await loadPromise;
  }

  async function submitImport() {
    uploadError = "";
    uploadSuccess = "";
    if (!$activeEntity) {
      uploadError = "Select an entity before importing.";
      return;
    }
    const formData = new FormData();
    formData.append("entity_id", $activeEntity);

    if (uploadMode === "file") {
      if (!file) {
        uploadError = "Choose a file to import.";
        return;
      }
      formData.append("file", file);
    } else {
      if (!importText.trim()) {
        uploadError = "Paste CSV data to import.";
        return;
      }
      formData.append("text", importText.trim());
      formData.append("filename", filename || "import.csv");
      formData.append("content_type", contentType || "text/csv");
    }

    if (suggestionContext.trim()) {
      formData.append("suggestion_context", suggestionContext.trim());
    }

    uploading = true;
    try {
      await apiFetch("/imports", {
        method: "POST",
        body: formData,
      });
      uploadSuccess = "Import queued for processing.";
      pushNotification(uploadSuccess, "success");
      file = null;
      importText = "";
      suggestionContext = "";
      if (fileInput) {
        fileInput.value = "";
      }
      await loadImports();
    } catch (err) {
      uploadError = errorMessage(err, "Import failed.");
      pushNotification(uploadError, "error");
    } finally {
      uploading = false;
    }
  }

  $effect(() => {
    if ($activeEntity) {
      loadImports();
    }
  });

  $effect(() => {
    if (!$activeEntity) {
      imports = [];
      listError = "";
    }
  });
</script>

<section class="grid gap-6">
  <div class="flex items-center justify-between">
    <div>
      <h1 class="text-2xl font-semibold tracking-tight">Imports</h1>
      <p class="mt-2 text-sm text-muted">
        Monitor bulk uploads, then step into review when import processing
        finishes.
      </p>
    </div>
    <a
      class="rounded-full bg-primary px-5 py-2 text-sm font-semibold text-paper"
      href="#new-import"
    >
      Start import
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
          Keep the import queue visible while you work through blocked or ready
          items.
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
        Search imports
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
          <h2 class="text-lg font-semibold">Recent imports</h2>
          <p class="mt-1 text-sm text-muted">
            The queue stays primary so status changes are easy to monitor.
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
        <p class="mt-4 text-sm text-muted">Loading imports…</p>
      {:else if !$activeEntity}
        <div
          class="mt-4 rounded-2xl border border-dashed border-line-strong bg-paper px-4 py-5 text-sm text-muted"
        >
          <p class="font-semibold text-ink">
            Select an entity to view imports.
          </p>
          <p class="mt-2">
            Import work is scoped to the active entity before you can queue or
            review anything.
          </p>
        </div>
      {:else if filteredImports.length === 0}
        <div
          class="mt-4 rounded-2xl border border-dashed border-line-strong bg-paper px-4 py-5 text-sm text-muted"
        >
          {#if searchQuery.trim()}
            <p class="font-semibold text-ink">No imports match this search.</p>
            <p class="mt-2">
              Try a different file name search or clear the search field.
            </p>
          {:else if statusFilter}
            <p class="font-semibold text-ink">No imports match this filter.</p>
            <p class="mt-2">
              Try a different status filter or switch back to all imports.
            </p>
          {:else}
            <p class="font-semibold text-ink">No imports yet.</p>
            <p class="mt-2">
              Start the first import for this entity to bring in bulk receipt or
              line-item data.
            </p>
          {/if}
        </div>
      {:else}
        <div class="mt-4 grid gap-3">
          {#each filteredImports as item}
            <a
              class="grid gap-3 rounded-xl border border-line px-4 py-4 hover:border-line-strong sm:grid-cols-[1.2fr_0.5fr_0.45fr] sm:items-center"
              href={`/imports/${item.id}`}
            >
              <div>
                <p class="text-sm font-semibold">
                  {item.original_name ?? item.id}
                </p>
                <p class="mt-1 text-xs text-muted">
                  {formatDate(item.uploaded_at)}
                </p>
              </div>
              <div class="text-sm">
                <p class="text-[10px] uppercase tracking-[0.2em] text-muted">
                  Status
                </p>
                <p class="mt-1 font-semibold">{item.status.replaceAll('_', ' ')}</p>
              </div>
              <div class="text-sm sm:text-right">
                <p class="text-[10px] uppercase tracking-[0.2em] text-muted">
                  Size
                </p>
                <p class="mt-1 font-semibold">{formatBytes(item.size_bytes)}</p>
              </div>
            </a>
          {/each}
        </div>
      {/if}
    </div>

    <div class="grid gap-4">
      <div
        id="new-import"
        class="rounded-2xl border border-dashed border-line-strong bg-surface p-6"
      >
        <div>
          <h2 class="text-lg font-semibold">Import shortcut</h2>
          <p class="mt-2 text-sm text-muted">
            Queue a file quickly without leaving the import queue.
          </p>
        </div>
        <div class="mt-4 flex gap-2 text-sm">
          <button
            class={`rounded-full border px-4 py-2 font-semibold ${uploadMode === "file" ? "border-ink text-ink" : "border-line text-muted"}`}
            type="button"
            onclick={() => (uploadMode = "file")}
          >
            File upload
          </button>
          <button
            class={`rounded-full border px-4 py-2 font-semibold ${uploadMode === "text" ? "border-ink text-ink" : "border-line text-muted"}`}
            type="button"
            onclick={() => (uploadMode = "text")}
          >
            Paste CSV
          </button>
        </div>

        <form
          class="mt-4 grid gap-3"
          onsubmit={(event) => {
            event.preventDefault();
            void submitImport();
          }}
        >
          {#if uploadMode === "file"}
            <label class="grid gap-2 text-sm font-medium">
              Import file
              <input
                class="rounded-xl border border-line px-3 py-2 text-sm"
                type="file"
                accept=".csv,.txt,text/csv,text/plain"
                bind:this={fileInput}
                onchange={handleFileChange}
                required
              />
            </label>
          {:else}
            <label class="grid gap-2 text-sm font-medium">
              CSV data
              <textarea
                class="min-h-35 rounded-xl border border-line px-3 py-2 text-sm"
                placeholder="date,amount,merchant,account"
                bind:value={importText}
              ></textarea>
            </label>
            <details class="rounded-xl border border-line px-4 py-3">
              <summary class="cursor-pointer text-sm font-semibold text-ink">
                Advanced paste options
              </summary>
              <div class="mt-3 grid gap-3 md:grid-cols-2">
                <label class="grid gap-2 text-sm font-medium">
                  Filename
                  <input
                    class="rounded-xl border border-line px-3 py-2 text-sm"
                    type="text"
                    bind:value={filename}
                  />
                </label>
                <label class="grid gap-2 text-sm font-medium">
                  Content type
                  <input
                    class="rounded-xl border border-line px-3 py-2 text-sm"
                    type="text"
                    bind:value={contentType}
                  />
                </label>
              </div>
            </details>
          {/if}

          <details class="rounded-xl border border-line px-4 py-3">
            <summary class="cursor-pointer text-sm font-semibold text-ink"
              >Add context</summary
            >
            <div class="mt-3">
              <label class="grid gap-2 text-sm font-medium">
                Suggestion context (optional)
                <textarea
                  class="min-h-24 rounded-xl border border-line px-3 py-2 text-sm"
                  placeholder="Any details that help categorize the import."
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
            {uploading ? "Uploading…" : "Start import"}
          </button>
          {#if !$activeEntity}
            <p class="text-xs text-muted">
              Select an entity to enable imports.
            </p>
          {:else}
            <p class="text-xs text-muted">
              Use file upload for the fastest path. Paste CSV only when needed.
            </p>
          {/if}
        </form>
      </div>

      <div class="rounded-2xl border border-line bg-surface p-6">
        <h2 class="text-lg font-semibold">Working guidance</h2>
        <div class="mt-4 grid gap-3 text-sm text-muted">
          <div class="rounded-xl border border-line px-4 py-3">
            <p class="font-semibold text-ink">1. Queue the import</p>
            <p class="mt-1">
              Default to file upload and use paste mode only for small ad hoc
              imports.
            </p>
          </div>
          <div class="rounded-xl border border-line px-4 py-3">
            <p class="font-semibold text-ink">2. Watch processing state</p>
            <p class="mt-1">
              Use filters to find imports that are blocked or ready for review.
            </p>
          </div>
          <div class="rounded-xl border border-line px-4 py-3">
            <p class="font-semibold text-ink">3. Review before posting</p>
            <p class="mt-1">
              Imports feed the same manual review and posting workflow as
              individual receipts.
            </p>
          </div>
        </div>
      </div>
    </div>
  </div>
</section>

