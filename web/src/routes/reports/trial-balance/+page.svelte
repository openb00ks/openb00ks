<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { formatCents } from '$lib/utils/money';
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { activeEntity, entities } from '$lib/stores/entity';
  import { apiFetch } from '$lib/api/client';
  import { todayLocalDate } from '$lib/utils/date';

  type Line = {
    account_id: string;
    account_name: string;
    account_type: string;
    debit_cents: number;
    credit_cents: number;
  };
  type TrialBalanceResponse = {
    lines: Line[];
    total_debit_cents: number;
    total_credit_cents: number;
    balanced: boolean;
  };

  // A trial balance is point-in-time: balances as of a date, cumulative from inception.
  const INCEPTION = '1970-01-01';

  let entityId = $state('');
  let endDate = $state('');
  let loading = $state(false);
  let error = $state('');
  let result = $state<TrialBalanceResponse | null>(null);

  let activeEntityName = $derived($entities.find((entity) => entity.id === entityId)?.name ?? '');

  onMount(() => {
    if (!endDate) endDate = todayLocalDate();
    const qEntity = $page.url.searchParams.get('entity_id');
    const qEnd = $page.url.searchParams.get('as_of') ?? $page.url.searchParams.get('end_date');
    if (!entityId && qEntity) entityId = qEntity;
    if (qEnd) endDate = qEnd;
    if (!entityId && $activeEntity) entityId = $activeEntity;
    void loadReport();
  });

  $effect(() => {
    if (!entityId && $activeEntity) entityId = $activeEntity;
  });

  async function loadReport() {
    if (!entityId || !endDate) {
      error = 'Select an entity and an "as of" date.';
      return;
    }
    loading = true;
    error = '';
    try {
      const params = new URLSearchParams({ entity_id: entityId, start_date: INCEPTION, end_date: endDate });
      result = await apiFetch<TrialBalanceResponse>(`/reports/trial-balance?${params.toString()}`);
    } catch (err) {
      error = errorMessage(err, 'Unable to load trial balance.');
      result = null;
    } finally {
      loading = false;
    }
  }
</script>

<section class="grid gap-6">
  <a class="text-sm text-muted hover:text-ink" href="/reports">← Back to reports</a>

  <div class="flex flex-wrap items-end justify-between gap-4">
    <div>
      <p class="text-xs uppercase tracking-[0.2em] text-muted">Report</p>
      <h1 class="mt-2 text-2xl font-semibold tracking-tight">Trial Balance</h1>
      <p class="mt-2 text-sm text-muted">
        {activeEntityName || 'Select an entity'} • As of {endDate || '—'}
      </p>
    </div>
  </div>

  <div class="rounded-2xl border border-line bg-surface p-6">
    <div class="grid gap-3 text-sm text-muted md:grid-cols-[1fr_220px_auto] md:items-end">
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
      <button class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-paper" type="button" onclick={loadReport}>
        Refresh
      </button>
    </div>
  </div>

  {#if error}
    <p class="status-message-sm status-error">{error}</p>
  {:else if loading || !result}
    <p class="text-sm text-muted">Loading report…</p>
  {:else}
    <div class="rounded-2xl border border-line bg-surface p-6">
      <div class="flex items-center justify-between">
        <h2 class="text-lg font-semibold">Accounts</h2>
        <span class={result.balanced ? 'font-semibold text-success' : 'font-semibold text-error'}>
          {result.balanced ? 'Balanced' : 'Out of balance'}
        </span>
      </div>
      {#if result.lines.length === 0}
        <div class="mt-4 rounded-2xl border border-dashed border-line-strong bg-paper px-4 py-5 text-sm text-muted">
          No posted balances as of this date.
        </div>
      {:else}
        <div class="mt-4 overflow-x-auto">
          <table class="w-full min-w-[36rem] text-sm">
            <thead>
              <tr class="border-b border-line text-left text-xs uppercase tracking-[0.14em] text-muted">
                <th class="py-2 pr-4 font-semibold">Account</th>
                <th class="py-2 pl-4 text-right font-semibold">Debit</th>
                <th class="py-2 pl-4 text-right font-semibold">Credit</th>
              </tr>
            </thead>
            <tbody>
              {#each result.lines as line}
                <tr class="border-b border-line/60">
                  <td class="py-2 pr-4 align-top">
                    <a class="font-medium hover:underline" href="/accounts/{line.account_id}">{line.account_name}</a>
                    <span class="ml-2 text-xs capitalize text-muted">{line.account_type}</span>
                  </td>
                  <td class="py-2 pl-4 text-right align-top">{line.debit_cents ? formatCents(line.debit_cents) : ''}</td>
                  <td class="py-2 pl-4 text-right align-top">{line.credit_cents ? formatCents(line.credit_cents) : ''}</td>
                </tr>
              {/each}
            </tbody>
            <tfoot>
              <tr class="border-t-2 border-line font-semibold text-ink">
                <td class="py-2 pr-4">Total</td>
                <td class="py-2 pl-4 text-right">{formatCents(result.total_debit_cents)}</td>
                <td class="py-2 pl-4 text-right">{formatCents(result.total_credit_cents)}</td>
              </tr>
            </tfoot>
          </table>
        </div>
      {/if}
    </div>
  {/if}
</section>
