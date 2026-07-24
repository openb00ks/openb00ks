<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { formatShortDateYear as formatDate } from '$lib/utils/date';
  import { formatCentsOrDash } from '$lib/utils/money';
  import { onMount, onDestroy } from 'svelte';
  import { page } from '$app/stores';
  import { apiFetch } from '$lib/api/client';
  import type { Account, OcrRun as OCRRow } from '$lib/api/types';
  import { pushNotification } from '$lib/stores/notifications';
  import { errorRecoveryHint } from '$lib/utils/error-hints';

  type AISummary = {
    vendor?: { name?: string; confidence?: number; reason?: string; is_new?: boolean };
    account?: { account_id?: string; confidence?: number; reason?: string };
  };

  type ReceiptResponse = {
    id: string;
    entity_id: string;
    status: string;
    content_type: string;
    size_bytes: number;
    total_cents: number;
    uploaded_at: string;
    attached_at?: string | null;
    original_name?: string;
    url?: string;
    suggestion_context?: string;
    resolved_vendor_id?: string;
    ai_summary?: AISummary;
    tags?: Array<{ id: string; name: string }>;
    errors?: Array<{ id: string; stage: string; error: string; created_at: string }>;
    draft?: DraftResponse;
  };

  type SuggestionRow = {
    id: string;
    status: string;
    confidence?: number;
    parsed_json?: unknown;
    cost_cents?: number;
    total_tokens?: number;
    prompt_tokens?: number;
    completion_tokens?: number;
    created_at: string;
  };

  type DraftEntry = {
    id: string;
    account_id: string;
    debit_cents: number;
    credit_cents: number;
  };

  type DraftResponse = {
    id: string;
    receipt_id: string;
    entity_id: string;
    date: string;
    memo?: string;
    entries: DraftEntry[];
  };

  type DraftEditEntry = {
    account_id: string;
    debit: string;
    credit: string;
  };

  type AccountRow = Pick<Account, 'id' | 'name' | 'type' | 'code'>;

  // accountLabel prefixes the chart-of-accounts code when present ("1000 · Cash").
  function accountLabel(account: { code?: string; name: string }): string {
    return account.code ? `${account.code} · ${account.name}` : account.name;
  }

  let receipt = $state<ReceiptResponse | null>(null);
  // A receipt is mid-pipeline while `processing` — used to keep stage buttons disabled + show progress.
  let isProcessing = $derived(receipt?.status === 'processing');
  let draft: DraftResponse | null = $state(null);
  let suggestions: SuggestionRow[] = $state([]);
  let ocrHistory: OCRRow[] = $state([]);
  let accounts: Record<string, AccountRow> = $state({});
  let vendorOptions: Array<{ id: string; name: string }> = $state([]);
  let vendorSaving = $state(false);
  let vendorError = $state('');
  let loading = $state(true);
  let error = $state('');
  let postError = $state('');
  let postSuccess = $state('');
  let posting = $state(false);
  let draftDate = $state('');
  let draftMemo = $state('');
  let draftEntries: DraftEditEntry[] = $state([]);
  let draftSaving = $state(false);
  let draftError = $state('');
  let draftSuccess = $state('');
  let editableTags: string[] = $state([]);
  let tagInput = $state('');
  let tagError = $state('');
  let tagSaving = $state(false);

  let rerunError = $state('');
  let rerunSuccess = $state('');
  let rerunning = $state(false);
  let ocrRerunLoading = $state(false);
  let ocrMessage = $state('');

  function latestSuggestion() {
    return suggestions[0];
  }

  function parseSuggestionFields(row: SuggestionRow | undefined) {
    if (!row?.parsed_json || typeof row.parsed_json !== 'object') {
      return null;
    }
    return row.parsed_json as Record<string, unknown>;
  }

  function formatCost(cents?: number) {
    if (!cents) {
      return '—';
    }
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD'
    }).format(cents / 100);
  }

  function centsToInput(cents: number) {
    if (!cents) {
      return '';
    }
    return (cents / 100).toFixed(2);
  }

  function parseAmount(value: string) {
    if (!value.trim()) {
      return 0;
    }
    const parsed = Number.parseFloat(value);
    if (Number.isNaN(parsed)) {
      return null;
    }
    return Math.round(parsed * 100);
  }

  function initDraftEdits(next: DraftResponse | null) {
    if (!next) {
      draftDate = '';
      draftMemo = '';
      draftEntries = [];
      return;
    }
    draftDate = next.date;
    draftMemo = next.memo ?? '';
    draftEntries = next.entries.map((entry) => ({
      account_id: entry.account_id,
      debit: centsToInput(entry.debit_cents),
      credit: centsToInput(entry.credit_cents)
    }));
  }

  const draftTotals = $derived.by(() => {
    let debit = 0;
    let credit = 0;
    let invalidCount = 0;
    for (const entry of draftEntries) {
      if (!entry.account_id) {
        invalidCount += 1;
      }
      const debitVal = parseAmount(entry.debit);
      const creditVal = parseAmount(entry.credit);
      if (debitVal === null || creditVal === null) {
        invalidCount += 1;
        continue;
      }
      if ((debitVal === 0) === (creditVal === 0)) {
        invalidCount += 1;
      }
      debit += debitVal;
      credit += creditVal;
    }
    return { debit, credit, invalidCount };
  });

  // draftEffects translates the debit/credit legs into plain-language balance movements, because the
  // accounting convention is counterintuitive: for assets & expenses a DEBIT increases the balance,
  // while for liabilities, equity & income a CREDIT increases it. (So a credit to Cash means money OUT.)
  const draftEffects = $derived.by((): Array<{ name: string; delta: number }> => {
    const out: Array<{ name: string; delta: number }> = [];
    for (const entry of draftEntries) {
      const account = accounts[entry.account_id];
      if (!account) {
        continue;
      }
      const debit = parseAmount(entry.debit) ?? 0;
      const credit = parseAmount(entry.credit) ?? 0;
      const netDebit = debit - credit;
      if (netDebit === 0) {
        continue;
      }
      const debitIncreases = account.type === 'asset' || account.type === 'expense';
      // delta in cents; positive = the account's balance goes up.
      out.push({ name: account.name, delta: debitIncreases ? netDebit : -netDebit });
    }
    return out;
  });

  function addDraftLine() {
    draftEntries = [...draftEntries, { account_id: '', debit: '', credit: '' }];
  }

  function removeDraftLine(index: number) {
    draftEntries = draftEntries.filter((_, idx) => idx !== index);
  }

  function resetDraftEdits() {
    initDraftEdits(draft);
    draftError = '';
    draftSuccess = '';
  }

  function validateDraft() {
    if (!draftDate) {
      return 'Draft date is required.';
    }
    if (draftEntries.length === 0) {
      return 'Add at least one entry.';
    }
    const totals = draftTotals;
    if (totals.invalidCount > 0) {
      return 'Fix invalid entries before saving.';
    }
    if (totals.debit !== totals.credit) {
      return 'Draft is not balanced.';
    }
    return '';
  }

  function entryHasAmountError(entry: DraftEditEntry) {
    const debitVal = parseAmount(entry.debit);
    const creditVal = parseAmount(entry.credit);
    if (debitVal === null || creditVal === null) {
      return true;
    }
    return (debitVal === 0) === (creditVal === 0);
  }

  function draftIsValid() {
    return !validateDraft();
  }

  let accountsError = $state('');

  async function loadReceipt(silent = false) {
    if (!silent) {
      loading = true;
    }
    error = '';
    accountsError = '';
    try {
      const receiptId = $page.params.id;
      const data = await apiFetch<ReceiptResponse>(`/receipts/${receiptId}`);
      receipt = data;
      draft = data.draft ?? null;
      initDraftEdits(draft);
      editableTags = (data.tags ?? []).map((tag) => tag.name);
      const [suggestionHistory, accountsResponse, ocrResponse, vendorsResponse] = await Promise.all([
        apiFetch<{ rows: SuggestionRow[] }>(`/receipts/${receiptId}/suggestion`).catch(() => ({ rows: [] })),
        // The accounts endpoint returns a bare array, not { rows }.
        apiFetch<AccountRow[]>(`/entities/${data.entity_id}/accounts?limit=1000`).catch(() => {
          accountsError = 'Could not load the account list; dropdowns may be incomplete. Reload to try again.';
          return null;
        }),
        apiFetch<{ rows: OCRRow[] }>(`/receipts/${receiptId}/ocr`).catch(() => ({ rows: [] })),
        apiFetch<{ rows: Array<{ id: string; name: string }> }>(
          `/vendors?entity_id=${encodeURIComponent(data.entity_id)}`
        ).catch(() => ({ rows: [] }))
      ]);
      suggestions = suggestionHistory.rows ?? [];
      ocrHistory = ocrResponse.rows ?? [];
      if (accountsResponse) {
        accounts = {};
        for (const row of accountsResponse) {
          accounts[row.id] = row;
        }
      }
      vendorOptions = vendorsResponse.rows ?? [];
      if (!draft) {
        draft = await apiFetch<DraftResponse>(`/receipts/${receiptId}/draft`).catch(() => null);
        initDraftEdits(draft);
      }
    } catch (err) {
      error = errorMessage(err, 'Unable to load receipt.');
    } finally {
      if (!silent) {
        loading = false;
      }
    }
  }

  // waitForSettled polls (quietly) after a stage rerun until the receipt has passed through
  // `processing` and reached a resting state, so the stage button stays disabled — and the page
  // refreshes with the result — for the whole run, not just the enqueue request. Bounded so a stuck
  // job can't disable the button forever.
  async function waitForSettled() {
    let sawProcessing = false;
    for (let i = 0; i < 60; i++) {
      await new Promise((resolve) => setTimeout(resolve, 2000));
      if (cancelled) {
        return;
      }
      await loadReceipt(true);
      if (receipt?.status === 'processing') {
        sawProcessing = true;
      } else if (sawProcessing) {
        return;
      }
    }
  }

  async function saveVendor(vendorId: string) {
    if (!receipt) {
      return;
    }
    vendorError = '';
    vendorSaving = true;
    try {
      await apiFetch(`/receipts/${receipt.id}/vendor`, {
        method: 'PATCH',
        body: { vendor_id: vendorId }
      });
      receipt = { ...receipt, resolved_vendor_id: vendorId || undefined };
    } catch (err) {
      vendorError = errorMessage(err, 'Unable to update vendor.');
    } finally {
      vendorSaving = false;
    }
  }

  function confidencePct(value?: number) {
    return value ? `${Math.round(value * 100)}%` : '—';
  }

  async function postDraft() {
    if (!receipt) {
      return;
    }
    postError = '';
    postSuccess = '';
    posting = true;
    try {
      await apiFetch(`/receipts/${receipt.id}/post`, { method: 'POST' });
      postSuccess = 'Transaction posted. Receipt marked as posted.';
      pushNotification(postSuccess, 'success');
      receipt = { ...receipt, status: 'posted' };
    } catch (err) {
      postError = errorMessage(err, 'Unable to post entry.');
      pushNotification(postError, 'error');
    } finally {
      posting = false;
    }
  }

  async function saveDraft() {
    if (!receipt) {
      return;
    }
    draftError = '';
    draftSuccess = '';
    const validationError = validateDraft();
    if (validationError) {
      draftError = validationError;
      return;
    }
    const entries = draftEntries.map((entry) => ({
      account_id: entry.account_id,
      debit_cents: parseAmount(entry.debit) ?? 0,
      credit_cents: parseAmount(entry.credit) ?? 0
    }));
    draftSaving = true;
    try {
      const updated = await apiFetch<DraftResponse>(`/receipts/${receipt.id}/draft`, {
        method: 'PATCH',
        body: {
          date: draftDate,
          memo: draftMemo,
          entries
        }
      });
      draft = updated;
      initDraftEdits(updated);
      draftSuccess = 'Draft updated.';
    } catch (err) {
      draftError = errorMessage(err, 'Unable to update draft.');
    } finally {
      draftSaving = false;
    }
  }

  async function rerunSuggestion() {
    if (!receipt || rerunning) {
      return;
    }
    rerunError = '';
    rerunSuccess = '';
    rerunning = true;
    try {
      await apiFetch(`/receipts/${receipt.id}/suggestion/rerun`, {
        method: 'POST'
      });
      rerunSuccess = 'Suggestion rerun queued.';
      pushNotification(rerunSuccess, 'success');
      await loadReceipt();
      await waitForSettled();
    } catch (err) {
      rerunError = errorMessage(err, 'Unable to rerun suggestion.');
      pushNotification(rerunError, 'error');
    } finally {
      rerunning = false;
    }
  }

  async function rerunOCR() {
    if (!receipt || ocrRerunLoading) {
      return;
    }
    ocrMessage = '';
    ocrRerunLoading = true;
    try {
      await apiFetch(`/receipts/${receipt.id}/ocr/rerun`, { method: 'POST' });
      ocrMessage = 'OCR rerun queued.';
      pushNotification(ocrMessage, 'success');
      await loadReceipt();
      await waitForSettled();
    } catch (err) {
      ocrMessage = errorMessage(err, 'Unable to rerun OCR.');
      pushNotification(ocrMessage, 'error');
    } finally {
      ocrRerunLoading = false;
    }
  }

  function addTag() {
    tagError = '';
    const next = tagInput
      .split(',')
      .map((tag) => tag.trim())
      .filter((tag) => tag.length > 0);
    if (next.length === 0) {
      tagError = 'Enter a tag name.';
      return;
    }
    const set = new Set(editableTags.map((tag) => tag.toLowerCase()));
    for (const tag of next) {
      if (!set.has(tag.toLowerCase())) {
        editableTags = [...editableTags, tag];
        set.add(tag.toLowerCase());
      }
    }
    tagInput = '';
  }

  function removeTag(name: string) {
    editableTags = editableTags.filter((tag) => tag !== name);
  }

  async function saveTags() {
    if (!receipt) {
      return;
    }
    tagError = '';
    tagSaving = true;
    try {
      const response = await apiFetch<{ tags: Array<{ id: string; name: string }> }>(
        `/receipts/${receipt.id}/tags`,
        {
          method: 'PATCH',
          body: { tags: editableTags }
        }
      );
      receipt = {
        ...receipt,
        tags: response.tags ?? []
      };
      editableTags = (response.tags ?? []).map((tag) => tag.name);
    } catch (err) {
      tagError = errorMessage(err, 'Unable to save tags.');
    } finally {
      tagSaving = false;
    }
  }

  let cancelled = false;
  onMount(() => {
    loadReceipt();
  });
  onDestroy(() => {
    cancelled = true;
  });
  let suggestion = $derived(latestSuggestion());
  let suggestionFields = $derived(suggestion ? parseSuggestionFields(suggestion) : null);
</script>

{#if loading}
  <p class="text-sm text-muted">Loading receipt…</p>
{:else if error}
  <div class="rounded-2xl border status-error p-4 text-sm text-error">
    {error}
  </div>
{:else if receipt}
  <section class="grid gap-6">
    <a class="text-sm text-muted hover:text-ink" href="/receipts">← Back to receipts</a>
    {#if accountsError}
      <div class="rounded-2xl border status-error p-4 text-sm text-error">{accountsError}</div>
    {/if}
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <p class="text-xs uppercase tracking-[0.2em] text-muted">Receipt detail</p>
        <h1 class="mt-2 text-2xl font-semibold tracking-tight">
          {receipt.original_name ?? 'Receipt'}
        </h1>
        <p class="mt-2 text-sm text-muted">
          {formatDate(receipt.uploaded_at)} • {receipt.status}
        </p>
      </div>
      <div class="text-right">
        <p class="text-xs uppercase tracking-[0.2em] text-muted">Total</p>
        <p class="mt-2 text-3xl font-semibold">{formatCentsOrDash(receipt.total_cents)}</p>
      </div>
    </div>

    <div class="grid gap-4 md:grid-cols-[1.2fr_1fr]">
      <div class="rounded-2xl border border-line bg-surface p-6">
        <h2 class="text-lg font-semibold">Receipt image</h2>
        {#if receipt.url && receipt.content_type?.startsWith('image/')}
          <div class="mt-4 overflow-hidden rounded-2xl border border-line bg-paper">
            <img class="h-auto w-full" src={receipt.url} alt="Receipt preview" />
          </div>
        {:else if receipt.url}
          <div class="mt-4 flex h-64 items-center justify-center rounded-2xl border border-dashed border-line-strong bg-paper text-sm text-muted">
            <a class="text-ink underline" href={receipt.url} target="_blank" rel="noreferrer">
              Open receipt file
            </a>
          </div>
        {:else}
          <div class="mt-4 flex h-64 items-center justify-center rounded-2xl border border-dashed border-line-strong bg-paper text-sm text-muted">
            Receipt preview unavailable
          </div>
        {/if}
      </div>

      <div class="rounded-2xl border border-line bg-surface p-6">
        <h2 class="text-lg font-semibold">Review summary</h2>
        <p class="mt-2 text-sm text-muted">
          Confirm the draft, then post manually when the receipt looks correct.
        </p>
        <div class="mt-4 grid gap-3 text-sm">
          <div class="grid gap-3 sm:grid-cols-2">
            <div class="rounded-xl border border-line px-4 py-3">
              <p class="text-xs uppercase tracking-[0.2em] text-muted">Receipt status</p>
              <p class="mt-2 font-semibold">{receipt.status}</p>
            </div>
            <div class="rounded-xl border border-line px-4 py-3">
              <p class="text-xs uppercase tracking-[0.2em] text-muted">Uploaded</p>
              <p class="mt-2 font-semibold">{formatDate(receipt.uploaded_at)}</p>
            </div>
          </div>
          <div class="grid gap-3 sm:grid-cols-2">
            <div class="rounded-xl border border-line px-4 py-3">
              <p class="text-xs uppercase tracking-[0.2em] text-muted">Confidence</p>
              <p class="mt-2 font-semibold">
                {suggestion?.confidence
                  ? `${Math.round(suggestion.confidence * 100)}%`
                  : '—'}
              </p>
            </div>
            <div class="rounded-xl border border-line px-4 py-3">
              <p class="text-xs uppercase tracking-[0.2em] text-muted">Suggested state</p>
              <p class="mt-2 font-semibold">{suggestion?.status ?? 'No suggestion yet'}</p>
            </div>
          </div>
          <div class="rounded-xl border border-line px-4 py-3">
            <p class="text-xs uppercase tracking-[0.2em] text-muted">What to check</p>
            <p class="mt-2 text-sm text-ink">
              Make sure the draft uses the right accounts, stays balanced, and matches the receipt
              total before posting.
            </p>
          </div>
          {#if receipt.ai_summary}
            <div class="rounded-xl border border-line px-4 py-3">
              <p class="text-xs uppercase tracking-[0.2em] text-muted">AI suggestion</p>
              {#if receipt.ai_summary.vendor}
                <div class="mt-2">
                  <p class="text-sm">
                    <span class="font-semibold">Vendor:</span>
                    {receipt.ai_summary.vendor.name || '—'}
                    <span class="text-muted"
                      >({confidencePct(receipt.ai_summary.vendor.confidence)}{receipt.ai_summary.vendor.is_new
                        ? ' · new'
                        : ''})</span
                    >
                  </p>
                  {#if receipt.ai_summary.vendor.reason}
                    <p class="text-xs text-muted">{receipt.ai_summary.vendor.reason}</p>
                  {/if}
                </div>
              {/if}
              {#if receipt.ai_summary.account}
                <div class="mt-2">
                  <p class="text-sm">
                    <span class="font-semibold">Account:</span>
                    {accounts[receipt.ai_summary.account.account_id ?? '']
                      ? accountLabel(accounts[receipt.ai_summary.account.account_id ?? ''])
                      : (receipt.ai_summary.account.account_id ?? '—')}
                    <span class="text-muted">({confidencePct(receipt.ai_summary.account.confidence)})</span>
                  </p>
                  {#if receipt.ai_summary.account.reason}
                    <p class="text-xs text-muted">{receipt.ai_summary.account.reason}</p>
                  {/if}
                </div>
              {/if}
              {#if vendorOptions.length > 0}
                <label class="mt-3 grid gap-1 text-xs font-semibold text-muted">
                  Correct the vendor
                  <select
                    class="rounded-xl border border-line px-3 py-2 text-sm"
                    disabled={vendorSaving}
                    value={receipt.resolved_vendor_id ?? ''}
                    onchange={(event) => saveVendor((event.currentTarget as HTMLSelectElement).value)}
                  >
                    <option value="">No vendor</option>
                    {#each vendorOptions as vendorOption}
                      <option value={vendorOption.id}>{vendorOption.name}</option>
                    {/each}
                  </select>
                </label>
                {#if vendorError}
                  <p class="mt-1 status-message-xs status-error">{vendorError}</p>
                {/if}
                <p class="mt-1 text-xs text-muted">Posting trains this vendor on the account you choose.</p>
              {/if}
            </div>
          {/if}
          {#if receipt.errors && receipt.errors.length > 0}
            <div class="status-message-md status-error">
              <p class="text-xs uppercase tracking-[0.2em] text-error">Blocking issues</p>
              <p class="mt-2 font-semibold">{receipt.errors.length} processing issue(s) need review</p>
              <p class="mt-1 text-xs">
                Check the technical details section below before posting.
              </p>
            </div>
          {/if}
        </div>
        <div class="mt-6 flex flex-wrap gap-3">
          <button
            class="rounded-full bg-primary px-5 py-2 text-sm font-semibold text-paper disabled:opacity-60"
            type="button"
            disabled={!draft || posting}
            onclick={postDraft}
          >
            {posting ? 'Posting…' : 'Post entry'}
          </button>
          <button
            class="rounded-full border border-ink/20 px-5 py-2 text-sm font-semibold disabled:opacity-60"
            type="button"
            disabled={rerunning || ocrRerunLoading || isProcessing}
            onclick={rerunSuggestion}
          >
            {rerunning ? (isProcessing ? 'Processing…' : 'Queuing…') : 'Rerun suggestion'}
          </button>
        </div>
        {#if rerunError}
          <p class="mt-3 status-message-sm status-error">
            {rerunError}
          </p>
        {/if}
        {#if rerunSuccess}
          <p class="mt-3 status-message-sm status-success">
            {rerunSuccess}
          </p>
        {/if}
        {#if postError}
          <div class="mt-3 status-message-sm status-error">
            <p>{postError}</p>
            {#if errorRecoveryHint(postError)}
              <p class="mt-1 text-xs">{errorRecoveryHint(postError)}</p>
            {/if}
          </div>
        {/if}
        {#if postSuccess}
          <p class="mt-3 status-message-sm status-success">
            {postSuccess}
          </p>
        {/if}
      </div>
    </div>

    <div class="grid gap-4 md:grid-cols-[1.1fr_0.9fr]">
      <div class="rounded-2xl border border-line bg-surface p-6">
        <h2 class="text-lg font-semibold">Draft confirmation</h2>
        <p class="mt-2 text-sm text-muted">
          This is the primary editable step before you post the transaction.
        </p>
        {#if draft}
          <div class="mt-4 grid gap-3 text-sm">
            <div class="grid gap-3 md:grid-cols-2">
              <label class="grid gap-2 text-sm font-medium text-ink">
                Date
                <input
                  class="rounded-xl border border-line px-3 py-2 text-sm"
                  type="date"
                  bind:value={draftDate}
                />
              </label>
              <label class="grid gap-2 text-sm font-medium text-ink">
                Memo
                <input
                  class="rounded-xl border border-line px-3 py-2 text-sm"
                  type="text"
                  placeholder="Optional memo"
                  bind:value={draftMemo}
                />
              </label>
            </div>

            <div class="grid gap-2">
              <div
                class="hidden gap-2 px-4 text-xs font-semibold uppercase tracking-wide text-muted md:grid md:grid-cols-[1.2fr_0.6fr_0.6fr_0.2fr]"
              >
                <span>Account</span>
                <span>Debit</span>
                <span>Credit</span>
                <span class="sr-only">Remove</span>
              </div>
              {#each draftEntries as entry, index}
                <div class="grid gap-2 rounded-xl border border-line px-4 py-3 md:grid-cols-[1.2fr_0.6fr_0.6fr_0.2fr] md:items-center">
                  <select
                    class={`rounded-xl border px-3 py-2 text-sm ${!entry.account_id ? 'field-error' : 'border-line'}`}
                    bind:value={entry.account_id}
                  >
                    <option value="">Select account</option>
                    {#each Object.values(accounts) as account}
                      <option value={account.id}>{accountLabel(account)}</option>
                    {/each}
                  </select>
                  <label class="grid gap-1">
                    <span class="text-xs font-medium text-muted md:hidden">Debit</span>
                    <input
                      class={`w-full rounded-xl border px-3 py-2 text-sm ${entryHasAmountError(entry) ? 'field-error' : 'border-line'}`}
                      type="number"
                      min="0"
                      step="0.01"
                      placeholder="0.00"
                      bind:value={entry.debit}
                    />
                  </label>
                  <label class="grid gap-1">
                    <span class="text-xs font-medium text-muted md:hidden">Credit</span>
                    <input
                      class={`w-full rounded-xl border px-3 py-2 text-sm ${entryHasAmountError(entry) ? 'field-error' : 'border-line'}`}
                      type="number"
                      min="0"
                      step="0.01"
                      placeholder="0.00"
                      bind:value={entry.credit}
                    />
                  </label>
                  <button
                    class="justify-self-start rounded-full border border-line px-3 py-1 text-xs font-semibold md:justify-self-auto"
                    type="button"
                    onclick={() => removeDraftLine(index)}
                    aria-label="Remove entry"
                  >
                    ×
                  </button>
                </div>
              {/each}
            </div>

            <button
              class="rounded-full border border-ink/20 px-4 py-2 text-sm font-semibold"
              type="button"
              onclick={addDraftLine}
            >
              Add entry
            </button>

            <div class="rounded-xl border border-line px-4 py-3 text-sm text-muted">
              <div class="flex items-center justify-between">
                <span>Total debit</span>
                <span class="font-semibold text-ink">{formatCentsOrDash(draftTotals.debit)}</span>
              </div>
              <div class="mt-2 flex items-center justify-between">
                <span>Total credit</span>
                <span class="font-semibold text-ink">{formatCentsOrDash(draftTotals.credit)}</span>
              </div>
              <div class="mt-2 flex items-center justify-between">
                <span>Status</span>
                <span class="font-semibold text-ink">
                  {draftTotals.invalidCount > 0
                    ? 'Fix highlighted rows'
                    : draftTotals.debit === draftTotals.credit
                      ? 'Balanced'
                      : 'Out of balance'}
                </span>
              </div>
            </div>

            {#if draftEffects.length > 0}
              <div class="rounded-xl border border-line bg-paper px-4 py-3 text-sm">
                <p class="text-xs font-semibold uppercase tracking-wide text-muted">In plain terms</p>
                <ul class="mt-2 grid gap-1">
                  {#each draftEffects as effect}
                    <li class="flex items-center justify-between gap-3">
                      <span>{effect.name}</span>
                      <span class="font-semibold text-ink">
                        {effect.delta >= 0 ? 'increases' : 'decreases'} by {formatCentsOrDash(Math.abs(effect.delta))}
                      </span>
                    </li>
                  {/each}
                </ul>
                <p class="mt-2 text-xs text-muted">
                  Reminder: for cash &amp; assets a credit lowers the balance (money out); for an expense a
                  debit raises it. Debits always equal credits.
                </p>
              </div>
            {/if}

            {#if draftError}
              <div class="status-message-xs status-error">
                <p>{draftError}</p>
                {#if errorRecoveryHint(draftError)}
                  <p class="mt-1">{errorRecoveryHint(draftError)}</p>
                {/if}
              </div>
            {/if}
            {#if draftSuccess}
              <p class="status-message-xs status-success">
                {draftSuccess}
              </p>
            {/if}

            <div class="flex flex-wrap gap-3">
              <button
                class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-paper disabled:opacity-60"
                type="button"
                disabled={draftSaving || !draftIsValid()}
                onclick={saveDraft}
              >
                {draftSaving ? 'Saving…' : 'Save draft'}
              </button>
              <button
                class="rounded-full border border-line px-4 py-2 text-sm font-semibold"
                type="button"
                onclick={resetDraftEdits}
              >
                Reset
              </button>
            </div>
          </div>
        {:else}
          <p class="mt-4 text-sm text-muted">No draft yet.</p>
        {/if}
      </div>

      <div class="grid gap-4">
        <div class="rounded-2xl border border-line bg-surface p-6">
          <h2 class="text-lg font-semibold">Before posting</h2>
          <p class="mt-2 text-sm text-muted">Use these checkpoints to keep posting deliberate.</p>
          <div class="mt-4 grid gap-3 text-sm text-muted">
            <div class="flex items-center justify-between rounded-xl border border-line px-4 py-3">
              <span>Draft status</span>
              <span class="font-semibold text-ink">{draft ? 'Ready to review' : 'No draft yet'}</span>
            </div>
            <div class="flex items-center justify-between rounded-xl border border-line px-4 py-3">
              <span>Balance check</span>
              <span class="font-semibold text-ink">
                {#if draft}
                  {draftTotals.invalidCount > 0
                    ? 'Fix highlighted rows'
                    : draftTotals.debit === draftTotals.credit
                      ? 'Balanced'
                      : 'Out of balance'}
                {:else}
                  No draft
                {/if}
              </span>
            </div>
            <div class="flex items-center justify-between rounded-xl border border-line px-4 py-3">
              <span>Estimated AI cost</span>
              <span class="font-semibold text-ink">{formatCost(suggestion?.cost_cents)}</span>
            </div>
          </div>
          <div class="mt-4">
            <p class="text-xs uppercase tracking-[0.2em] text-muted">Tags</p>
            {#if editableTags.length === 0}
              <p class="mt-2 text-xs text-muted">No tags yet.</p>
            {:else}
              <div class="mt-2 flex flex-wrap gap-2">
                {#each editableTags as tag}
                  <span class="flex items-center gap-2 rounded-full border border-line px-3 py-1 text-xs font-semibold text-muted">
                    {tag}
                    <button
                      class="text-xs text-muted hover:text-ink"
                      type="button"
                      onclick={() => removeTag(tag)}
                      aria-label={`Remove tag ${tag}`}
                    >
                      ×
                    </button>
                  </span>
                {/each}
              </div>
            {/if}
            <div class="mt-3 grid gap-2 text-sm">
              <div class="flex flex-wrap gap-2">
                <input
                  class="flex-1 rounded-xl border border-line px-3 py-2 text-sm"
                  type="text"
                  placeholder="Add tags"
                  bind:value={tagInput}
                />
                <button
                  class="rounded-full border border-ink/20 px-4 py-2 text-sm font-semibold"
                  type="button"
                  onclick={addTag}
                >
                  Add
                </button>
              </div>
              <button
                class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-paper disabled:opacity-60"
                type="button"
                disabled={tagSaving}
                onclick={saveTags}
              >
                {tagSaving ? 'Saving…' : 'Save tags'}
              </button>
              {#if tagError}
                <p class="status-message-xs status-error">
                  {tagError}
                </p>
              {/if}
            </div>
          </div>
        </div>
      </div>
    </div>

    <div class="rounded-2xl border border-line bg-surface p-6">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 class="text-lg font-semibold">Technical detail</h2>
          <p class="mt-2 text-sm text-muted">
            Diagnostics are available here when you need to investigate OCR, suggestion, or
            processing issues.
          </p>
        </div>
        <p class="text-xs uppercase tracking-[0.2em] text-muted">Collapsed by default</p>
      </div>
      <div class="mt-4 grid gap-3">
        <details class="rounded-2xl border border-line px-4 py-3">
          <summary class="cursor-pointer text-sm font-semibold text-ink">OCR detail</summary>
          {#if ocrHistory.length > 0}
            <div class="mt-3 grid gap-3 text-sm">
              <div class="rounded-xl border border-line px-4 py-3">
                <p class="text-xs uppercase tracking-[0.2em] text-muted">Status</p>
                <p class="mt-2 font-semibold">{ocrHistory[0].status}</p>
              </div>
              {#if ocrHistory[0].raw_text}
                <div class="rounded-xl border border-line px-4 py-3 text-xs text-muted">
                  <p class="text-[10px] uppercase tracking-[0.2em] text-muted">Extracted text</p>
                  <pre class="mt-2 whitespace-pre-wrap text-ink">{ocrHistory[0].raw_text}</pre>
                </div>
              {/if}
              {#if ocrHistory[0].error}
                <p class="status-message-xs status-error">
                  {ocrHistory[0].error}
                </p>
              {/if}
            </div>
          {:else}
            <p class="mt-3 text-sm text-muted">No OCR runs yet.</p>
          {/if}
          <div class="mt-4 flex flex-wrap gap-3">
            <button
              class="rounded-full border border-ink/20 px-4 py-2 text-sm font-semibold disabled:opacity-60"
              type="button"
              disabled={ocrRerunLoading || rerunning || isProcessing}
              onclick={rerunOCR}
            >
              {ocrRerunLoading ? (isProcessing ? 'Processing…' : 'Queuing…') : 'Rerun OCR'}
            </button>
          </div>
          {#if ocrMessage}
            <p class="mt-3 text-xs text-muted">{ocrMessage}</p>
          {/if}
        </details>

        <details class="rounded-2xl border border-line px-4 py-3">
          <summary class="cursor-pointer text-sm font-semibold text-ink">Suggestion detail</summary>
          {#if suggestion}
            <div class="mt-3 grid gap-3 text-sm">
              <div class="rounded-xl border border-line px-4 py-3">
                <p class="text-xs uppercase tracking-[0.2em] text-muted">Status</p>
                <p class="mt-2 font-semibold">{suggestion.status}</p>
              </div>
              {#if suggestion.total_tokens}
                <div class="rounded-xl border border-line px-4 py-3">
                  <p class="text-xs uppercase tracking-[0.2em] text-muted">Token usage</p>
                  <p class="mt-2 text-sm text-ink">
                    {suggestion.prompt_tokens ?? 0} prompt + {suggestion.completion_tokens ?? 0}
                    completion ({suggestion.total_tokens} total)
                  </p>
                </div>
              {/if}
              {#if suggestionFields}
                <div class="rounded-xl border border-line px-4 py-3 text-xs text-muted">
                  <p class="text-[10px] uppercase tracking-[0.2em] text-muted">Raw suggestion</p>
                  <pre class="mt-2 whitespace-pre-wrap text-ink">
{JSON.stringify(suggestionFields, null, 2)}
                  </pre>
                </div>
              {/if}
            </div>
          {:else}
            <p class="mt-3 text-sm text-muted">No suggestions yet.</p>
          {/if}
        </details>

        <details class="rounded-2xl border border-line px-4 py-3">
          <summary class="cursor-pointer text-sm font-semibold text-ink">Receipt metadata</summary>
          <div class="mt-3 grid gap-3 text-sm text-muted">
            {#if receipt.suggestion_context}
              <div class="rounded-xl border border-line px-4 py-3">
                <p class="text-xs uppercase tracking-[0.2em] text-muted">Suggestion context</p>
                <p class="mt-2 text-sm text-ink">{receipt.suggestion_context}</p>
              </div>
            {/if}
            <div class="flex items-center justify-between rounded-xl border border-line px-4 py-3">
              <span>Entity ID</span>
              <span class="font-semibold text-ink">{receipt.entity_id}</span>
            </div>
            <div class="flex items-center justify-between rounded-xl border border-line px-4 py-3">
              <span>Content type</span>
              <span class="font-semibold text-ink">{receipt.content_type}</span>
            </div>
            <div class="flex items-center justify-between rounded-xl border border-line px-4 py-3">
              <span>Size</span>
              <span class="font-semibold text-ink">{(receipt.size_bytes / 1024).toFixed(1)} KB</span>
            </div>
          </div>
        </details>

        {#if receipt.errors && receipt.errors.length > 0}
          <details class="rounded-2xl border status-error px-4 py-3">
            <summary class="cursor-pointer text-sm font-semibold text-error">
              Processing errors
            </summary>
            <div class="mt-3 grid gap-3 text-sm text-muted">
              {#each receipt.errors as issue}
                <div class="rounded-xl border status-error px-4 py-3">
                  <p class="text-sm font-semibold text-ink">{issue.stage}</p>
                  <p class="mt-1 text-xs text-muted">{issue.error}</p>
                  <p class="mt-2 text-xs text-muted">{formatDate(issue.created_at)}</p>
                </div>
              {/each}
            </div>
          </details>
        {/if}
      </div>
    </div>
  </section>
{/if}
