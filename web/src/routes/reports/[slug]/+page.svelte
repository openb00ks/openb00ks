<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { formatCents } from '$lib/utils/money';
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { activeEntity, entities } from '$lib/stores/entity';
  import { apiFetch, apiFetchBlob } from '$lib/api/client';
  import { formatLocalDate, todayLocalDate } from '$lib/utils/date';

  type ReportLine = {
    account_id: string;
    account_name: string;
    account_type: string;
    amount_cents: number;
  };

  type ProfitLossResponse = {
    income: ReportLine[];
    expenses: ReportLine[];
    net_income_cents: number;
  };

  type LedgerRow = {
    transaction_id: string;
    date: string;
    memo: string;
    account_id: string;
    account_name: string;
    account_type: string;
    debit_cents: number;
    credit_cents: number;
  };

  let entityId = $state('');
  let startDate = $state('');
  let endDate = $state('');
  let loading = $state(false);
  let error = $state('');
  let income: ReportLine[] = $state([]);
  let expenses: ReportLine[] = $state([]);
  let netIncomeCents = $state(0);
  let ledgerRows: LedgerRow[] = $state([]);
  let downloading = $state(false);
  let downloadError = $state('');

  let slug = $derived($page.params.slug);
  let activeEntityName =
    $derived($entities.find((entity) => entity.id === entityId)?.name ?? '');

  function initDates() {
    const now = new Date();
    const todayStr = todayLocalDate();
    const monthStart = formatLocalDate(new Date(now.getFullYear(), now.getMonth(), 1));
    if (!startDate) {
      startDate = monthStart;
    }
    if (!endDate) {
      endDate = todayStr;
    }
  }

  function syncFromQuery() {
    const params = $page.url.searchParams;
    const qEntity = params.get('entity_id');
    const qStart = params.get('start_date');
    const qEnd = params.get('end_date');
    if (!entityId && qEntity) {
      entityId = qEntity;
    }
    if (!startDate && qStart) {
      startDate = qStart;
    }
    if (!endDate && qEnd) {
      endDate = qEnd;
    }
  }

  onMount(() => {
    initDates();
    syncFromQuery();
    if (!entityId && $activeEntity) {
      entityId = $activeEntity;
    }
    loadReport();
  });

  $effect(() => {
    if (!entityId && $activeEntity) {
      entityId = $activeEntity;
    }
  });

  async function loadReport() {
    if (!entityId || !startDate || !endDate) {
      error = 'Select an entity and date range.';
      return;
    }
    loading = true;
    error = '';
    income = [];
    expenses = [];
    ledgerRows = [];
    try {
      const params = new URLSearchParams({
        entity_id: entityId,
        start_date: startDate,
        end_date: endDate
      });
      if (slug === 'profit-loss') {
        const response = await apiFetch<ProfitLossResponse>(
          `/reports/profit-loss?${params.toString()}`
        );
        income = response.income ?? [];
        expenses = response.expenses ?? [];
        netIncomeCents = response.net_income_cents ?? 0;
      } else if (slug === 'general-ledger') {
        const response = await apiFetch<{ rows: LedgerRow[] }>(
          `/reports/general-ledger?${params.toString()}`
        );
        ledgerRows = response.rows ?? [];
      } else {
        error = 'Unknown report.';
      }
    } catch (err) {
      error = errorMessage(err, 'Unable to load report.');
    } finally {
      loading = false;
    }
  }

  function reportTitle() {
    if (slug === 'profit-loss') {
      return 'Profit & Loss';
    }
    if (slug === 'general-ledger') {
      return 'General Ledger';
    }
    return 'Report';
  }

  async function downloadCSV() {
    downloadError = '';
    if (!entityId || !startDate || !endDate) {
      downloadError = 'Select an entity and date range.';
      return;
    }
    downloading = true;
    try {
      const params = new URLSearchParams({
        entity_id: entityId,
        start_date: startDate,
        end_date: endDate
      });
      const blob = await apiFetchBlob(`/exports/transactions.csv?${params.toString()}`);
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `report-${slug}-${startDate}-to-${endDate}.csv`;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
    } catch (err) {
      downloadError = errorMessage(err, 'Unable to download CSV.');
    } finally {
      downloading = false;
    }
  }
</script>

<section class="grid gap-6">
  <a class="text-sm text-muted hover:text-ink" href="/reports">← Back to reports</a>
  <div class="flex flex-wrap items-start justify-between gap-4">
    <div>
      <p class="text-xs uppercase tracking-[0.2em] text-muted">Report</p>
      <h1 class="mt-2 text-2xl font-semibold tracking-tight">{reportTitle()}</h1>
      <p class="mt-2 text-sm text-muted">
        {#if activeEntityName}
          {activeEntityName}
        {:else}
          Select an entity
        {/if}
        • {startDate} → {endDate}
      </p>
    </div>
    <div class="flex gap-3">
      {#if slug === 'general-ledger'}
        <button
          class="rounded-full border border-ink/20 px-4 py-2 text-sm font-semibold disabled:opacity-60"
          onclick={downloadCSV}
          disabled={downloading}
        >
          {downloading ? 'Exporting…' : 'Export CSV'}
        </button>
      {/if}
    </div>
  </div>

  <div class="grid gap-4 md:grid-cols-3">
    <div class="rounded-2xl border border-line bg-surface p-5">
      <p class="text-xs uppercase tracking-[0.2em] text-muted">Entity</p>
      <p class="mt-2 text-lg font-semibold">{activeEntityName || 'Select entity'}</p>
    </div>
    <div class="rounded-2xl border border-line bg-surface p-5">
      <p class="text-xs uppercase tracking-[0.2em] text-muted">Start date</p>
      <p class="mt-2 text-lg font-semibold">{startDate || '—'}</p>
    </div>
    <div class="rounded-2xl border border-line bg-surface p-5">
      <p class="text-xs uppercase tracking-[0.2em] text-muted">End date</p>
      <p class="mt-2 text-lg font-semibold">{endDate || '—'}</p>
    </div>
  </div>

  <div class="rounded-2xl border border-line bg-surface p-6">
    <h2 class="text-lg font-semibold">Report scope</h2>
    <div class="mt-4 grid gap-3 text-sm text-muted md:grid-cols-3">
      <label class="grid gap-2 text-sm font-medium text-ink">
        Entity
        <select
          class="rounded-xl border border-line px-3 py-2 text-base"
          bind:value={entityId}
        >
          <option value="">Select entity</option>
          {#each $entities as entity}
            <option value={entity.id}>{entity.name}</option>
          {/each}
        </select>
      </label>
      <label class="grid gap-2 text-sm font-medium text-ink">
        Start date
        <input class="rounded-xl border border-line px-3 py-2 text-base" type="date" bind:value={startDate} />
      </label>
      <label class="grid gap-2 text-sm font-medium text-ink">
        End date
        <input class="rounded-xl border border-line px-3 py-2 text-base" type="date" bind:value={endDate} />
      </label>
    </div>
    <div class="mt-4 flex flex-wrap gap-3">
      <button class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-paper" onclick={loadReport}>
        Refresh
      </button>
    </div>
    {#if downloadError}
      <p class="mt-4 status-message-sm status-error">
        {downloadError}
      </p>
    {/if}
  </div>

  {#if error}
    <p class="status-message-sm status-error">
      {error}
    </p>
  {:else if loading}
    <p class="text-sm text-muted">Loading report…</p>
  {:else if slug === 'profit-loss'}
    <div class="grid gap-4 md:grid-cols-2">
      <div class="rounded-2xl border border-line bg-surface p-6">
        <h2 class="text-lg font-semibold">Income</h2>
        {#if income.length === 0}
          <p class="mt-4 text-sm text-muted">No income entries were posted in this range.</p>
        {:else}
          <div class="mt-4 grid gap-2 text-sm text-muted">
            {#each income as line}
              <div class="flex items-center justify-between">
                <span>{line.account_name}</span>
                <span class="font-semibold text-ink">{formatCents(line.amount_cents)}</span>
              </div>
            {/each}
          </div>
        {/if}
      </div>

      <div class="rounded-2xl border border-line bg-surface p-6">
        <h2 class="text-lg font-semibold">Expenses</h2>
        {#if expenses.length === 0}
          <p class="mt-4 text-sm text-muted">No expense entries were posted in this range.</p>
        {:else}
          <div class="mt-4 grid gap-2 text-sm text-muted">
            {#each expenses as line}
              <div class="flex items-center justify-between">
                <span>{line.account_name}</span>
                <span class="font-semibold text-ink">{formatCents(line.amount_cents)}</span>
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </div>

    <div class="rounded-2xl border border-line bg-surface p-6">
      <div class="flex items-center justify-between">
        <p class="text-sm text-muted">Net income</p>
        <p class="text-xl font-semibold text-ink">{formatCents(netIncomeCents)}</p>
      </div>
    </div>
  {:else if slug === 'general-ledger'}
    <div class="rounded-2xl border border-line bg-surface p-6">
      <h2 class="text-lg font-semibold">Ledger entries</h2>
      {#if ledgerRows.length === 0}
        <p class="mt-4 text-sm text-muted">No posted ledger entries match the current reporting scope.</p>
      {:else}
        <div class="mt-4 grid gap-3">
          {#each ledgerRows as row}
            <div class="grid gap-2 rounded-xl border border-line px-4 py-3 md:grid-cols-[0.8fr_1.6fr_0.8fr_0.8fr] md:items-center">
              <div class="text-xs text-muted">{row.date}</div>
              <div>
                <p class="text-sm font-semibold">{row.account_name}</p>
                <p class="text-xs text-muted">{row.memo || row.transaction_id}</p>
              </div>
              <div class="text-sm text-muted">Debit {formatCents(row.debit_cents)}</div>
              <div class="text-sm text-muted">Credit {formatCents(row.credit_cents)}</div>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {:else}
    <p class="text-sm text-muted">Select a report from the list.</p>
  {/if}
</section>

