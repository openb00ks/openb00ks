<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { formatShortDate as formatDate } from '$lib/utils/date';
  import { formatCentsOrDash } from '$lib/utils/money';
  import { browser } from '$app/environment';
  import { activeEntity } from '$lib/stores/entity';
  import { pushNotification } from '$lib/stores/notifications';
  import { readFilterPreference, writeFilterPreference } from '$lib/utils/filter-preferences';
  import {
    loadReviewQueue,
    queueActionLabel,
    queuePrimaryAction,
    queueSecondaryActions,
    runQueueAction,
    shouldShowRequeue,
    type ReviewQueueAction,
    type ReviewQueueItem,
    type ReviewQueueKind,
    type ReviewQueueStatusFilter
  } from '$lib/review/queue';

  let queue: ReviewQueueItem[] = $state([]);
  let loading = $state(false);
  let error = $state('');
  let statusFilter: ReviewQueueStatusFilter = ($state(browser
    ? (readFilterPreference('review.status_filter', 'all') as ReviewQueueStatusFilter)
    : 'all'));
  let kindFilter: ReviewQueueKind | 'all' = $state(browser
    ? ((readFilterPreference('review.kind_filter', 'all') as ReviewQueueKind | 'all'))
    : 'all');
  let searchQuery = $state(browser ? readFilterPreference('review.search_query', '') : '');
  let actionLoading: Record<string, boolean> = $state({});
  let actionMessage: Record<string, string> = $state({});
  let selectedItems: Record<string, boolean> = $state({});
  let bulkActionLoading: ReviewQueueAction | '' = $state('');
  let bulkActionMessage = $state('');
  let filteredQueue = $derived(queue.filter((item) => {
    const search = searchQuery.trim().toLowerCase();
    if (!search) {
      return true;
    }
    const haystack = `${item.original_name ?? ''} ${item.id}`.toLowerCase();
    return haystack.includes(search);
  }));
  let selectedQueueItems = $derived(filteredQueue.filter((item) => selectedItems[item.id]));
  let statusFilterLabel =
    $derived(statusFilter === 'ready_for_review'
      ? 'ready for review'
      : statusFilter === 'needs_attention'
        ? 'needs attention'
        : 'all statuses');
  let kindFilterLabel =
    $derived(kindFilter === 'receipt'
      ? 'receipts'
      : kindFilter === 'import'
        ? 'imports'
        : 'all items');

  function actionKey(itemID: string, action: ReviewQueueAction) {
    return `${itemID}:${action}`;
  }

  function processingState(item: ReviewQueueItem) {
    const stage = item.latest_job?.stage;
    const status = item.latest_job?.status;
    if (stage && status) {
      return `${stage} • ${status}`;
    }
    return item.status.replaceAll('_', ' ');
  }

  function primaryError(item: ReviewQueueItem) {
    if (item.errors.length === 0) {
      return '';
    }
    const first = item.errors[0];
    return first.stage ? `${first.stage}: ${first.error}` : first.error;
  }

  function statusBadgeClass(status: string) {
    if (status === 'needs_attention') {
      return 'status-error';
    }
    if (status === 'ready_for_review') {
      return 'status-success';
    }
    return 'border-line bg-surface text-muted';
  }

  function primaryAction(item: ReviewQueueItem) {
    return queuePrimaryAction(item);
  }

  async function applyStatusFilter(next: ReviewQueueStatusFilter) {
    statusFilter = next;
    writeFilterPreference('review.status_filter', statusFilter);
    await loadQueue();
  }

  async function applyKindFilter(next: ReviewQueueKind | 'all') {
    kindFilter = next;
    writeFilterPreference('review.kind_filter', kindFilter);
    await loadQueue();
  }

  function updateSearch(value: string) {
    searchQuery = value;
    writeFilterPreference('review.search_query', searchQuery);
  }

  function toggleVisibleItems(selected: boolean) {
    const next = { ...selectedItems };
    for (const item of filteredQueue) {
      next[item.id] = selected;
    }
    selectedItems = next;
    bulkActionMessage = '';
  }

  function clearSelectedItems() {
    selectedItems = {};
    bulkActionMessage = '';
  }

  function queueActionAllowed(item: ReviewQueueItem, action: ReviewQueueAction) {
    if (action === 'requeue') {
      return shouldShowRequeue(item);
    }
    return true;
  }

  function primaryActionIsLink(item: ReviewQueueItem) {
    return primaryAction(item).kind === 'link';
  }

  function primaryActionHref(item: ReviewQueueItem) {
    const action = primaryAction(item);
    return action.kind === 'link' ? action.href : '';
  }

  function primaryActionLabel(item: ReviewQueueItem) {
    return primaryAction(item).label;
  }

  function primaryActionMutation(item: ReviewQueueItem): ReviewQueueAction {
    const action = primaryAction(item);
    return action.kind === 'action' ? action.action : 'suggestion';
  }

  function primaryActionLoading(item: ReviewQueueItem) {
    if (primaryActionIsLink(item)) {
      return false;
    }
    return actionLoading[actionKey(item.id, primaryActionMutation(item))];
  }

  function runPrimaryAction(item: ReviewQueueItem) {
    if (primaryActionIsLink(item)) {
      return;
    }
    return runAction(item, primaryActionMutation(item));
  }

  async function loadQueue() {
    if (!$activeEntity) {
      queue = [];
      return;
    }
    loading = true;
    error = '';
    try {
      queue = await loadReviewQueue($activeEntity, statusFilter, kindFilter);
    } catch (err) {
      error = errorMessage(err, 'Unable to load review queue.');
    } finally {
      loading = false;
    }
  }

  async function runAction(item: ReviewQueueItem, action: ReviewQueueAction) {
    const key = actionKey(item.id, action);
    actionLoading = { ...actionLoading, [key]: true };
    actionMessage = { ...actionMessage, [item.id]: '' };
    try {
      const message = await runQueueAction(item, action);
      actionMessage = { ...actionMessage, [item.id]: message };
      pushNotification(message, 'success');
      await loadQueue();
    } catch (err) {
      const message = errorMessage(err, 'Request failed.');
      actionMessage = {
        ...actionMessage,
        [item.id]: message
      };
      pushNotification(message, 'error');
    } finally {
      actionLoading = { ...actionLoading, [key]: false };
    }
  }

  async function runBulkAction(action: ReviewQueueAction) {
    const items = selectedQueueItems.filter((item) => queueActionAllowed(item, action));
    if (items.length === 0) {
      return;
    }
    bulkActionLoading = action;
    bulkActionMessage = '';
    error = '';
    try {
      let succeeded = 0;
      let failed = 0;
      for (const item of items) {
        try {
          await runQueueAction(item, action);
          succeeded += 1;
        } catch (err) {
          failed += 1;
          const message = errorMessage(err, 'Request failed.');
          actionMessage = {
            ...actionMessage,
            [item.id]: message
          };
        }
      }
      await loadQueue();
      clearSelectedItems();
      const parts = [`${queueActionLabel(action)} applied to ${succeeded} item(s).`];
      if (failed > 0) {
        parts.push(`${failed} item(s) failed.`);
      }
      bulkActionMessage = parts.join(' ');
      pushNotification(bulkActionMessage, failed > 0 ? 'error' : 'success');
    } finally {
      bulkActionLoading = '';
    }
  }

  function totalQueueAmount() {
    return filteredQueue.reduce((sum, item) => sum + (item.total_cents ?? 0), 0);
  }

  function totalQueueCost() {
    return filteredQueue.reduce((sum, item) => sum + (item.cost_cents ?? 0), 0);
  }

  $effect(() => {
    if ($activeEntity) {
      loadQueue();
    }
  });

  $effect(() => {
    if (!$activeEntity) {
      queue = [];
      error = '';
      actionLoading = {};
      actionMessage = {};
      selectedItems = {};
      bulkActionLoading = '';
      bulkActionMessage = '';
    }
  });
</script>

<section class="grid gap-6">
  <div class="flex flex-wrap items-center justify-between gap-4">
    <div>
      <h1 class="text-2xl font-semibold tracking-tight">Review queue</h1>
      <p class="mt-2 text-sm text-muted">Receipts and imports that need human review before any posting.</p>
    </div>
    <div class="flex flex-wrap gap-2 text-xs text-muted">
      <button
        class={`rounded-full px-3 py-1 font-semibold ${statusFilter === 'all' ? 'bg-primary text-paper' : 'border border-line text-ink'}`}
        type="button"
        onclick={() => applyStatusFilter('all')}
      >
        All
      </button>
      <button
        class={`rounded-full px-3 py-1 font-semibold ${statusFilter === 'ready_for_review' ? 'bg-primary text-paper' : 'border border-line text-ink'}`}
        type="button"
        onclick={() => applyStatusFilter('ready_for_review')}
      >
        Drafts ready
      </button>
      <button
        class={`rounded-full px-3 py-1 font-semibold ${statusFilter === 'needs_attention' ? 'bg-primary text-paper' : 'border border-line text-ink'}`}
        type="button"
        onclick={() => applyStatusFilter('needs_attention')}
      >
        Needs attention
      </button>
      <span class="mx-2 hidden h-6 w-px bg-line sm:inline-block"></span>
      <button
        class={`rounded-full px-3 py-1 font-semibold ${kindFilter === 'all' ? 'bg-primary text-paper' : 'border border-line text-ink'}`}
        type="button"
        onclick={() => applyKindFilter('all')}
      >
        All items
      </button>
      <button
        class={`rounded-full px-3 py-1 font-semibold ${kindFilter === 'receipt' ? 'bg-primary text-paper' : 'border border-line text-ink'}`}
        type="button"
        onclick={() => applyKindFilter('receipt')}
      >
        Receipts
      </button>
      <button
        class={`rounded-full px-3 py-1 font-semibold ${kindFilter === 'import' ? 'bg-primary text-paper' : 'border border-line text-ink'}`}
        type="button"
        onclick={() => applyKindFilter('import')}
      >
        Imports
      </button>
    </div>
  </div>

  <div class="rounded-2xl border border-line bg-surface p-6">
    <label class="grid gap-2 text-sm font-medium text-ink">
      Search queue
      <input
        class="rounded-xl border border-line px-3 py-2 text-sm"
        type="text"
        placeholder="Search by file name"
        bind:value={searchQuery}
        oninput={(event) => updateSearch(event.currentTarget.value)}
      />
    </label>
  </div>

  <div class="rounded-2xl border border-line bg-surface p-6">
    <div class="flex items-center justify-between">
      <h2 class="text-lg font-semibold">Queue</h2>
      <span class="text-xs uppercase tracking-[0.2em] text-muted">Live</span>
    </div>
    {#if filteredQueue.length > 0}
      <div class="mt-3 flex flex-wrap gap-2">
        <button
          class="rounded-full border border-line px-3 py-1 text-xs font-semibold"
          type="button"
          onclick={() => toggleVisibleItems(true)}
        >
          Select visible
        </button>
        <button
          class="rounded-full border border-line px-3 py-1 text-xs font-semibold"
          type="button"
          onclick={() => toggleVisibleItems(false)}
        >
          Deselect visible
        </button>
      </div>
    {/if}
    {#if selectedQueueItems.length > 0}
      <div class="mt-4 rounded-xl border border-line bg-paper px-4 py-3">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <p class="text-sm font-semibold text-ink">{selectedQueueItems.length} selected</p>
          <div class="flex flex-wrap gap-2">
            <button
              class="rounded-full border border-ink/20 px-3 py-2 text-xs font-semibold disabled:opacity-60"
              type="button"
              disabled={bulkActionLoading === 'suggestion'}
              onclick={() => runBulkAction('suggestion')}
            >
              {bulkActionLoading === 'suggestion' ? 'Working…' : 'Rerun suggestions'}
            </button>
            <button
              class="rounded-full border border-ink/20 px-3 py-2 text-xs font-semibold disabled:opacity-60"
              type="button"
              disabled={bulkActionLoading === 'ocr'}
              onclick={() => runBulkAction('ocr')}
            >
              {bulkActionLoading === 'ocr' ? 'Working…' : 'Rerun OCR'}
            </button>
            <button
              class="rounded-full border border-ink/20 px-3 py-2 text-xs font-semibold disabled:opacity-60"
              type="button"
              disabled={bulkActionLoading === 'requeue' || selectedQueueItems.every((item) => !shouldShowRequeue(item))}
              onclick={() => runBulkAction('requeue')}
            >
              {bulkActionLoading === 'requeue' ? 'Working…' : 'Retry processing'}
            </button>
            <button
              class="rounded-full border border-line px-3 py-2 text-xs font-semibold"
              type="button"
              onclick={clearSelectedItems}
            >
              Clear
            </button>
          </div>
        </div>
      </div>
    {/if}
    {#if bulkActionMessage}
      <p class="mt-3 text-xs text-muted">{bulkActionMessage}</p>
    {/if}
    {#if error}
      <p class="mt-4 status-message-sm status-error">
        {error}
      </p>
    {:else if loading}
      <p class="mt-4 text-sm text-muted">Loading queue…</p>
    {:else if filteredQueue.length === 0}
      <div class="mt-4 rounded-2xl border border-dashed border-line-strong bg-paper px-4 py-5 text-sm text-muted">
        {#if !$activeEntity}
          <p class="font-semibold text-ink">Select an entity to review items.</p>
          <p class="mt-2">The review queue only shows receipts and imports for the active entity.</p>
        {:else if searchQuery.trim()}
          <p class="font-semibold text-ink">No queue items match this search.</p>
          <p class="mt-2">Try a different file name search or clear the search field.</p>
        {:else if statusFilter !== 'all' || kindFilter !== 'all'}
          <p class="font-semibold text-ink">No {kindFilterLabel} match {statusFilterLabel}.</p>
          <p class="mt-2">Try broadening the filters or switch back to all items to keep work moving.</p>
        {:else}
          <p class="font-semibold text-ink">No items need review right now.</p>
          <p class="mt-2">Capture a receipt or start an import, then come back here when processing finishes.</p>
        {/if}
      </div>
    {:else}
      <div class="mt-4 grid gap-3">
        {#each filteredQueue as item}
          <div class="rounded-xl border border-line px-4 py-4">
            <div class="grid gap-4 md:grid-cols-[auto_minmax(0,1.8fr)_auto] md:items-start">
              <label class="pt-1">
                <input
                  type="checkbox"
                  aria-label={`Select ${item.original_name ?? item.id}`}
                  checked={!!selectedItems[item.id]}
                  onchange={(event) =>
                    (selectedItems = {
                      ...selectedItems,
                      [item.id]: event.currentTarget.checked,
                    })}
                />
              </label>
              <a
                class="block rounded-xl transition-colors hover:text-ink"
                href={item.kind === 'import' ? `/imports/${item.id}` : `/receipts/${item.id}`}
              >
                <div class="flex flex-wrap items-center gap-2">
                  <p class="text-sm font-semibold">{item.original_name ?? item.id}</p>
                  <span
                    class={`rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase tracking-[0.16em] ${statusBadgeClass(item.status)}`}
                  >
                    {item.status.replaceAll('_', ' ')}
                  </span>
                  <span class="text-[10px] uppercase tracking-[0.16em] text-muted">
                    {item.kind === 'import' ? 'Import' : 'Receipt'}
                  </span>
                </div>
                <p class="mt-1 text-xs text-muted">
                  {formatDate(item.uploaded_at)} • Processing: {processingState(item)}
                </p>

                <div class="mt-3 grid gap-3 sm:grid-cols-3">
                  <div class="rounded-xl border border-line px-3 py-2">
                    <p class="text-[10px] uppercase tracking-[0.16em] text-muted">Amount</p>
                    <p class="mt-1 text-sm font-semibold text-ink">{formatCentsOrDash(item.total_cents)}</p>
                  </div>
                  <div class="rounded-xl border border-line px-3 py-2">
                    <p class="text-[10px] uppercase tracking-[0.16em] text-muted">Confidence</p>
                    <p class="mt-1 text-sm font-semibold text-ink">
                    {item.confidence ? `${Math.round(item.confidence * 100)}%` : '—'}
                    </p>
                    {#if item.suggestion_status}
                      <p class="mt-1 text-xs text-muted">Suggestion: {item.suggestion_status}</p>
                    {/if}
                    {#if item.status === 'ready_for_review'}
                      <p class="mt-1 text-[10px] font-semibold uppercase tracking-[0.16em] text-success">
                        Draft ready
                      </p>
                    {/if}
                  </div>
                  <div class="rounded-xl border border-line px-3 py-2">
                    <p class="text-[10px] uppercase tracking-[0.16em] text-muted">AI cost</p>
                    <p class="mt-1 text-sm font-semibold text-ink">{formatCentsOrDash(item.cost_cents)}</p>
                  </div>
                </div>

                {#if primaryError(item)}
                  <p class="mt-3 status-message-xs status-error">
                    {primaryError(item)}
                  </p>
                {/if}
              </a>

              <div class="flex flex-col items-stretch gap-3 md:min-w-44">
                {#if primaryActionIsLink(item)}
                  <a
                    class="rounded-full bg-primary px-3 py-2 text-center text-xs font-semibold text-paper"
                    href={primaryActionHref(item)}
                  >
                    {primaryActionLabel(item)}
                  </a>
                {:else}
                  <button
                    class="rounded-full bg-primary px-3 py-2 text-xs font-semibold text-paper disabled:opacity-60"
                    type="button"
                    disabled={primaryActionLoading(item)}
                    onclick={() => runPrimaryAction(item)}
                  >
                    {primaryActionLoading(item)
                      ? 'Working…'
                      : primaryActionLabel(item)}
                  </button>
                {/if}

                {#if queueSecondaryActions(item).length > 0}
                  <div class="rounded-xl border border-line px-3 py-3">
                    <p class="text-[10px] font-semibold uppercase tracking-[0.16em] text-muted">
                      Other actions
                    </p>
                    <div class="mt-2 grid gap-2">
                      {#each queueSecondaryActions(item) as action}
                        <button
                          class="rounded-full border border-ink/20 px-3 py-2 text-xs font-semibold disabled:opacity-60"
                          type="button"
                          disabled={actionLoading[actionKey(item.id, action)]}
                          onclick={() => runAction(item, action)}
                        >
                          {actionLoading[actionKey(item.id, action)] ? 'Working…' : queueActionLabel(action)}
                        </button>
                      {/each}
                    </div>
                  </div>
                {/if}
                {#if actionMessage[item.id]}
                  <p class="text-xs text-muted">{actionMessage[item.id]}</p>
                {/if}
              </div>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </div>

  <div class="grid gap-4 md:grid-cols-5">
    <div class="rounded-2xl border border-line bg-surface p-5">
      <p class="text-xs uppercase tracking-[0.2em] text-muted">Ready now</p>
      <p class="mt-2 text-2xl font-semibold">
        {filteredQueue.filter((item) => item.status === 'ready_for_review').length}
      </p>
    </div>
    <div class="rounded-2xl border border-line bg-surface p-5">
      <p class="text-xs uppercase tracking-[0.2em] text-muted">Needs attention</p>
      <p class="mt-2 text-2xl font-semibold">
        {filteredQueue.filter((item) => item.status === 'needs_attention').length}
      </p>
    </div>
    <div class="rounded-2xl border border-line bg-surface p-5">
      <p class="text-xs uppercase tracking-[0.2em] text-muted">Total in queue</p>
      <p class="mt-2 text-2xl font-semibold">{filteredQueue.length}</p>
    </div>
    <div class="rounded-2xl border border-line bg-surface p-5">
      <p class="text-xs uppercase tracking-[0.2em] text-muted">Amount in queue</p>
      <p class="mt-2 text-2xl font-semibold">{formatCentsOrDash(totalQueueAmount())}</p>
    </div>
    <div class="rounded-2xl border border-line bg-surface p-5">
      <p class="text-xs uppercase tracking-[0.2em] text-muted">AI cost (est)</p>
      <p class="mt-2 text-2xl font-semibold">{formatCentsOrDash(totalQueueCost())}</p>
    </div>
  </div>
</section>

