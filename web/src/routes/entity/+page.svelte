<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { formatShortDate as formatDate } from '$lib/utils/date';
  import { formatCents } from '$lib/utils/money';
  import { apiFetch } from '$lib/api/client';
  import { activeEntity, entities } from '$lib/stores/entity';

  type ReceiptRow = {
    id: string;
    status: string;
    uploaded_at: string;
    original_name?: string;
    total_cents?: number;
  };

  type ImportRow = {
    id: string;
    status: string;
    uploaded_at: string;
    original_name?: string;
    size_bytes: number;
  };

  type SuggestionRow = {
    receipt_id: string;
    cost_cents?: number;
  };

  type FeedItem = {
    id: string;
    kind: 'receipt' | 'import';
    title: string;
    status: string;
    uploaded_at: string;
    total_cents?: number;
  };

  let receipts: ReceiptRow[] = $state([]);
  let imports: ImportRow[] = $state([]);
  let feed: FeedItem[] = $state([]);
  let readyCount = $state(0);
  let attentionCount = $state(0);
  let processingCount = $state(0);
  let aiCostCents = $state(0);
  let loading = $state(false);
  let error = $state('');
  let loadPromise: Promise<void> | null = null;

  let currentEntity =
    $derived($entities.find((entity) => entity.id === $activeEntity) ?? null);

  function isProcessing(status: string) {
    return !['ready_for_review', 'needs_attention', 'posted'].includes(status);
  }

  function buildFeed(receiptRows: ReceiptRow[], importRows: ImportRow[]) {
    const receiptFeed = receiptRows.map((item) => ({
      id: item.id,
      kind: 'receipt' as const,
      title: item.original_name ?? item.id,
      status: item.status,
      uploaded_at: item.uploaded_at,
      total_cents: item.total_cents
    }));
    const importFeed = importRows.map((item) => ({
      id: item.id,
      kind: 'import' as const,
      title: item.original_name ?? item.id,
      status: item.status,
      uploaded_at: item.uploaded_at
    }));
    return [...receiptFeed, ...importFeed]
      .sort((a, b) => b.uploaded_at.localeCompare(a.uploaded_at))
      .slice(0, 6);
  }

  async function loadDashboard() {
    if (loadPromise) {
      return;
    }
    if (!$activeEntity) {
      receipts = [];
      imports = [];
      feed = [];
      readyCount = 0;
      attentionCount = 0;
      processingCount = 0;
      aiCostCents = 0;
      return;
    }

    loading = true;
    error = '';
    loadPromise = (async () => {
      try {
        const [receiptResp, importResp] = await Promise.all([
          apiFetch<{ rows: ReceiptRow[] }>(
            `/receipts?entity_id=${encodeURIComponent($activeEntity)}&limit=20`
          ),
          apiFetch<{ rows: ImportRow[] }>(
            `/imports?entity_id=${encodeURIComponent($activeEntity)}&limit=20`
          )
        ]);

        receipts = receiptResp.rows ?? [];
        imports = importResp.rows ?? [];

        const combinedStatuses = [...receipts, ...imports].map((item) => item.status);
        readyCount = combinedStatuses.filter((status) => status === 'ready_for_review').length;
        attentionCount = combinedStatuses.filter((status) => status === 'needs_attention').length;
        processingCount = combinedStatuses.filter((status) => isProcessing(status)).length;
        feed = buildFeed(receipts, imports);

        const receiptIDs = receipts.map((row) => row.id);
        if (receiptIDs.length === 0) {
          aiCostCents = 0;
          return;
        }

        const suggestionResp = await apiFetch<{ rows: SuggestionRow[] }>(
          '/receipts/suggestions/batch',
          {
            method: 'POST',
            body: { receipt_ids: receiptIDs }
          }
        );
        aiCostCents = (suggestionResp.rows ?? []).reduce(
          (sum, row) => sum + (row.cost_cents ?? 0),
          0
        );
      } catch (err) {
        error = errorMessage(err, 'Unable to load entity dashboard.');
      } finally {
        loading = false;
        loadPromise = null;
      }
    })();

    await loadPromise;
  }

  $effect(() => {
    if ($activeEntity) {
      loadDashboard();
    }
  });

  $effect(() => {
    if (!$activeEntity) {
      receipts = [];
      imports = [];
      feed = [];
      readyCount = 0;
      attentionCount = 0;
      processingCount = 0;
      aiCostCents = 0;
      error = '';
    }
  });
</script>

<section class="grid gap-6">
  <div class="grid gap-4 md:grid-cols-[1.25fr_1fr]">
    <div class="rounded-3xl bg-surface p-8 shadow-sm">
      <p class="text-sm uppercase tracking-[0.2em] text-muted">Entity workspace</p>
      {#if currentEntity}
        <p class="mt-2 text-xs font-semibold uppercase tracking-[0.16em] text-muted">
          Active entity: {currentEntity.name}
        </p>
        <h1 class="mt-3 text-3xl font-semibold tracking-tight md:text-4xl">
          Work the inbox, then post when ready.
        </h1>
        <p class="mt-4 max-w-2xl text-muted">
          Start with the review queue for anything waiting on confirmation, capture
          new source documents from receipts or imports, and keep posting manual.
        </p>
        <div class="mt-6 flex flex-wrap gap-3">
          <a class="rounded-full bg-primary px-5 py-2 text-sm font-semibold text-paper" href="/review">
            Open review queue
          </a>
          <a class="rounded-full border border-line px-5 py-2 text-sm font-semibold" href="/receipts">
            Capture receipt
          </a>
          <a class="rounded-full border border-line px-5 py-2 text-sm font-semibold" href="/imports">
            Start import
          </a>
        </div>
      {:else}
        <h1 class="mt-3 text-3xl font-semibold tracking-tight md:text-4xl">
          Select an entity to open its work hub.
        </h1>
        <p class="mt-4 max-w-2xl text-muted">
          This page becomes the day-to-day control center for one entity: review
          its queue, capture new source documents, and move confirmed work into the books.
        </p>
        <div class="mt-6 flex flex-wrap gap-3">
          <a class="rounded-full bg-primary px-5 py-2 text-sm font-semibold text-paper" href="/">
            Choose an entity
          </a>
        </div>
      {/if}
    </div>

    <div class="rounded-3xl border border-line bg-surface p-6">
      <h2 class="text-lg font-semibold">Primary actions</h2>
      <div class="mt-4 grid gap-3 text-sm text-muted">
        <a class="rounded-xl border border-line px-4 py-3 text-left hover:border-line-strong" href="/review">
          <p class="text-sm font-semibold text-ink">Review queue</p>
          <p class="text-xs text-muted">Clear items that are ready or blocked.</p>
        </a>
        <a class="rounded-xl border border-line px-4 py-3 text-left hover:border-line-strong" href="/receipts">
          <p class="text-sm font-semibold text-ink">Receipts</p>
          <p class="text-xs text-muted">Upload files and monitor receipt processing.</p>
        </a>
        <a class="rounded-xl border border-line px-4 py-3 text-left hover:border-line-strong" href="/imports">
          <p class="text-sm font-semibold text-ink">Imports</p>
          <p class="text-xs text-muted">Bring in batches or pasted CSV data.</p>
        </a>
      </div>
    </div>
  </div>

  {#if error}
    <p class="status-message-sm status-error">
      {error}
    </p>
  {/if}

  <div class="grid gap-4 md:grid-cols-4">
    <div class="rounded-2xl border border-line bg-surface p-5">
      <p class="text-xs uppercase tracking-[0.2em] text-muted">Drafts pending post</p>
      <p class="mt-2 text-2xl font-semibold">{readyCount}</p>
      <p class="mt-2 text-xs text-muted">Items already prepared and waiting for confirmation.</p>
    </div>
    <div class="rounded-2xl border border-line bg-surface p-5">
      <p class="text-xs uppercase tracking-[0.2em] text-muted">Needs attention</p>
      <p class="mt-2 text-2xl font-semibold">{attentionCount}</p>
    </div>
    <div class="rounded-2xl border border-line bg-surface p-5">
      <p class="text-xs uppercase tracking-[0.2em] text-muted">Still processing</p>
      <p class="mt-2 text-2xl font-semibold">{processingCount}</p>
    </div>
    <div class="rounded-2xl border border-line bg-surface p-5">
      <p class="text-xs uppercase tracking-[0.2em] text-muted">AI cost (recent)</p>
      {#if loading}
        <p class="mt-2 text-sm text-muted">Loading…</p>
      {:else}
        <p class="mt-2 text-2xl font-semibold">{formatCents(aiCostCents)}</p>
      {/if}
    </div>
  </div>

  <div class="grid gap-4 md:grid-cols-[1.35fr_1fr]">
    <div class="rounded-2xl border border-line bg-surface p-6">
      <div class="flex items-center justify-between">
        <h2 class="text-lg font-semibold">Recent work</h2>
        <span class="text-xs uppercase tracking-[0.16em] text-muted">Latest 6</span>
      </div>
      {#if !currentEntity}
        <p class="mt-4 text-sm text-muted">Select an entity to see recent operational activity.</p>
      {:else if loading && feed.length === 0}
        <p class="mt-4 text-sm text-muted">Loading activity…</p>
      {:else if feed.length === 0}
        <p class="mt-4 text-sm text-muted">No receipts or imports yet for this entity.</p>
      {:else}
        <div class="mt-4 grid gap-3">
          {#each feed as item}
            <a
              class="flex items-center justify-between gap-4 rounded-xl border border-line px-4 py-3 hover:border-line-strong"
              href={item.kind === 'import' ? `/imports/${item.id}` : `/receipts/${item.id}`}
            >
              <div>
                <p class="text-sm font-semibold">{item.title}</p>
                <p class="text-xs text-muted">
                  {item.kind === 'import' ? 'Import' : 'Receipt'} • {item.status}
                </p>
                {#if item.status === 'ready_for_review'}
                  <p class="mt-1 text-[10px] font-semibold uppercase tracking-[0.16em] text-success">
                    Draft ready
                  </p>
                {/if}
              </div>
              <div class="text-right">
                <p class="text-xs text-muted">{formatDate(item.uploaded_at)}</p>
                {#if item.kind === 'receipt'}
                  <p class="mt-1 text-sm font-semibold">{formatCents(item.total_cents)}</p>
                {/if}
              </div>
            </a>
          {/each}
        </div>
      {/if}
    </div>

    <div class="rounded-2xl border border-line bg-surface p-6">
      <h2 class="text-lg font-semibold">How work moves</h2>
      <div class="mt-4 grid gap-3 text-sm text-muted">
        <div class="rounded-xl border border-line px-4 py-3">
          <p class="text-xs uppercase tracking-[0.16em] text-muted">1. Capture</p>
          <p class="mt-2 text-sm text-ink">Upload receipts or start imports for this entity.</p>
        </div>
        <div class="rounded-xl border border-line px-4 py-3">
          <p class="text-xs uppercase tracking-[0.16em] text-muted">2. Review</p>
          <p class="mt-2 text-sm text-ink">Resolve blocked items and confirm drafts in the queue.</p>
        </div>
        <div class="rounded-xl border border-line px-4 py-3">
          <p class="text-xs uppercase tracking-[0.16em] text-muted">3. Post</p>
          <p class="mt-2 text-sm text-ink">Posting stays manual so entries are reviewed before hitting the books.</p>
        </div>
      </div>
    </div>
  </div>
</section>

