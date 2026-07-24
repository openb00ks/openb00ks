<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { formatCents } from '$lib/utils/money';
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { activeEntity, entities } from '$lib/stores/entity';
  import { apiFetch } from '$lib/api/client';
  import { todayLocalDate } from '$lib/utils/date';

  type ReportLine = {
    account_id: string;
    account_name: string;
    account_type: string;
    amount_cents: number;
  };

  type BalanceSheetResponse = {
    assets: ReportLine[];
    liabilities: ReportLine[];
    equity: ReportLine[];
    total_assets_cents: number;
    total_liabilities_cents: number;
    total_equity_cents: number;
  };

  // A balance sheet is a point-in-time statement: balances as of a date, cumulative from inception. We
  // query the trial balance from an inception floor up to the chosen "as of" date.
  const INCEPTION = '1970-01-01';

  let entityId = $state('');
  let endDate = $state('');
  let loading = $state(false);
  let error = $state('');
  let assets: ReportLine[] = $state([]);
  let liabilities: ReportLine[] = $state([]);
  let equity: ReportLine[] = $state([]);
  let totals = $state({
    assets: 0,
    liabilities: 0,
    equity: 0
  });

  let activeEntityName =
    $derived($entities.find((entity) => entity.id === entityId)?.name ?? '');

  function initDates() {
    if (!endDate) {
      endDate = todayLocalDate();
    }
  }

  function syncFromQuery() {
    const params = $page.url.searchParams;
    const qEntity = params.get('entity_id');
    const qEnd = params.get('as_of') ?? params.get('end_date');
    if (!entityId && qEntity) {
      entityId = qEntity;
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
    if (!entityId || !endDate) {
      error = 'Select an entity and an "as of" date.';
      return;
    }
    loading = true;
    error = '';
    try {
      const params = new URLSearchParams({
        entity_id: entityId,
        start_date: INCEPTION,
        end_date: endDate
      });
      const response = await apiFetch<BalanceSheetResponse>(
        `/reports/balance-sheet?${params.toString()}`
      );
      assets = response.assets ?? [];
      liabilities = response.liabilities ?? [];
      equity = response.equity ?? [];
      totals = {
        assets: response.total_assets_cents ?? 0,
        liabilities: response.total_liabilities_cents ?? 0,
        equity: response.total_equity_cents ?? 0
      };
    } catch (err) {
      error = errorMessage(err, 'Unable to load balance sheet.');
    } finally {
      loading = false;
    }
  }

</script>

<section class="grid gap-6">
  <a class="text-sm text-muted hover:text-ink" href="/reports">← Back to reports</a>
  <div class="flex flex-wrap items-start justify-between gap-4">
    <div>
      <p class="text-xs uppercase tracking-[0.2em] text-muted">Report</p>
      <h1 class="mt-2 text-2xl font-semibold tracking-tight">Balance Sheet</h1>
      <p class="mt-2 text-sm text-muted">
        {#if activeEntityName}
          {activeEntityName}
        {:else}
          Select an entity
        {/if}
        • As of {endDate || '—'}
      </p>
    </div>
  </div>

  <div class="grid gap-4 md:grid-cols-2">
    <div class="rounded-2xl border border-line bg-surface p-5">
      <p class="text-xs uppercase tracking-[0.2em] text-muted">Entity</p>
      <p class="mt-2 text-lg font-semibold">{activeEntityName || 'Select entity'}</p>
    </div>
    <div class="rounded-2xl border border-line bg-surface p-5">
      <p class="text-xs uppercase tracking-[0.2em] text-muted">As of</p>
      <p class="mt-2 text-lg font-semibold">{endDate || '—'}</p>
    </div>
  </div>

  <div class="rounded-2xl border border-line bg-surface p-6">
    <h2 class="text-lg font-semibold">Report scope</h2>
    <div class="mt-4 grid gap-3 text-sm text-muted md:grid-cols-2">
      <label class="grid gap-2 text-sm font-medium text-ink">
        Entity
        <select class="rounded-xl border border-line px-3 py-2 text-base" bind:value={entityId}>
          <option value="">Select entity</option>
          {#each $entities as entity}
            <option value={entity.id}>{entity.name}</option>
          {/each}
        </select>
      </label>
      <label class="grid gap-2 text-sm font-medium text-ink">
        As of date
        <input class="rounded-xl border border-line px-3 py-2 text-base" type="date" bind:value={endDate} />
      </label>
    </div>
    <div class="mt-4 flex flex-wrap gap-3">
      <button class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-paper" onclick={loadReport}>
        Refresh
      </button>
    </div>
  </div>

  {#if error}
    <p class="status-message-sm status-error">
      {error}
    </p>
  {:else if loading}
    <p class="text-sm text-muted">Loading report…</p>
  {:else}
    <div class="rounded-2xl border border-line bg-surface p-6">
      <div class="mt-4 grid gap-4">
        <div class="rounded-xl border border-line p-4">
          <div class="flex items-center justify-between">
            <p class="text-sm font-semibold">Assets</p>
            <p class="text-sm font-semibold">{formatCents(totals.assets)}</p>
          </div>
          {#if assets.length === 0}
            <p class="mt-3 text-sm text-muted">No asset balances match the current scope.</p>
          {:else}
            <div class="mt-3 grid gap-2 text-sm text-muted">
              {#each assets as line}
                <div class="flex items-center justify-between">
                  <span>{line.account_name}</span>
                  <span>{formatCents(line.amount_cents)}</span>
                </div>
              {/each}
            </div>
          {/if}
        </div>

        <div class="rounded-xl border border-line p-4">
          <div class="flex items-center justify-between">
            <p class="text-sm font-semibold">Liabilities</p>
            <p class="text-sm font-semibold">{formatCents(totals.liabilities)}</p>
          </div>
          {#if liabilities.length === 0}
            <p class="mt-3 text-sm text-muted">No liability balances match the current scope.</p>
          {:else}
            <div class="mt-3 grid gap-2 text-sm text-muted">
              {#each liabilities as line}
                <div class="flex items-center justify-between">
                  <span>{line.account_name}</span>
                  <span>{formatCents(line.amount_cents)}</span>
                </div>
              {/each}
            </div>
          {/if}
        </div>

        <div class="rounded-xl border border-line p-4">
          <div class="flex items-center justify-between">
            <p class="text-sm font-semibold">Equity</p>
            <p class="text-sm font-semibold">{formatCents(totals.equity)}</p>
          </div>
          {#if equity.length === 0}
            <p class="mt-3 text-sm text-muted">No equity balances match the current scope.</p>
          {:else}
            <div class="mt-3 grid gap-2 text-sm text-muted">
              {#each equity as line}
                <div class="flex items-center justify-between">
                  <span>{line.account_name}</span>
                  <span>{formatCents(line.amount_cents)}</span>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      </div>
    </div>

    <div class="grid gap-4 md:grid-cols-3">
      <div class="rounded-2xl border border-line bg-surface p-5">
        <p class="text-xs uppercase tracking-[0.2em] text-muted">Total assets</p>
        <p class="mt-2 text-2xl font-semibold">{formatCents(totals.assets)}</p>
      </div>
      <div class="rounded-2xl border border-line bg-surface p-5">
        <p class="text-xs uppercase tracking-[0.2em] text-muted">Total liabilities</p>
        <p class="mt-2 text-2xl font-semibold">{formatCents(totals.liabilities)}</p>
      </div>
      <div class="rounded-2xl border border-line bg-surface p-5">
        <p class="text-xs uppercase tracking-[0.2em] text-muted">Total equity</p>
        <p class="mt-2 text-2xl font-semibold">{formatCents(totals.equity)}</p>
      </div>
    </div>
  {/if}
</section>

