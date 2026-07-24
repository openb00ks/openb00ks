<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { formatCents } from '$lib/utils/money';
  import { onMount } from 'svelte';
  import { activeEntity, entities } from '$lib/stores/entity';
  import { apiFetch } from '$lib/api/client';

  type Row = {
    vendor_id: string;
    vendor_name: string;
    tax_id: string;
    total_cents: number;
    needs_1099: boolean;
  };
  type Response = {
    threshold_cents: number;
    rows: Row[];
  };

  const currentYear = new Date().getFullYear();
  let entityId = $state('');
  let taxYear = $state(String(currentYear - 1));
  let loading = $state(false);
  let error = $state('');
  let result = $state<Response | null>(null);

  let activeEntityName = $derived($entities.find((entity) => entity.id === entityId)?.name ?? '');
  let flaggedCount = $derived(result?.rows.filter((row) => row.needs_1099).length ?? 0);

  onMount(() => {
    if (!entityId && $activeEntity) entityId = $activeEntity;
    void loadReport();
  });

  $effect(() => {
    if (!entityId && $activeEntity) entityId = $activeEntity;
  });

  async function loadReport() {
    if (!entityId || !taxYear) {
      error = 'Select an entity and tax year.';
      return;
    }
    loading = true;
    error = '';
    try {
      const params = new URLSearchParams({
        entity_id: entityId,
        start_date: `${taxYear}-01-01`,
        end_date: `${taxYear}-12-31`
      });
      result = await apiFetch<Response>(`/reports/vendor-payments?${params.toString()}`);
    } catch (err) {
      error = errorMessage(err, 'Unable to load vendor payments.');
      result = null;
    } finally {
      loading = false;
    }
  }
</script>

<section class="grid gap-6">
  <a class="text-sm text-muted hover:text-ink" href="/reports">← Back to reports</a>

  <div>
    <p class="text-xs uppercase tracking-[0.2em] text-muted">Report</p>
    <h1 class="mt-2 text-2xl font-semibold tracking-tight">Vendor payments (1099)</h1>
    <p class="mt-2 text-sm text-muted">
      {activeEntityName || 'Select an entity'} • Tax year {taxYear}
    </p>
  </div>

  <div class="rounded-2xl border border-line bg-surface p-6">
    <div class="grid gap-3 text-sm text-muted md:grid-cols-[1fr_180px_auto] md:items-end">
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
        Tax year
        <input class="rounded-xl border border-line px-3 py-2 text-base" type="number" min="1900" max="3000" bind:value={taxYear} />
      </label>
      <button class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-paper" type="button" onclick={loadReport}>
        Refresh
      </button>
    </div>
  </div>

  <div class="rounded-2xl border border-line bg-paper px-4 py-3 text-sm text-muted">
    Vendors paid <span class="font-semibold text-ink">{formatCents(result?.threshold_cents ?? 60000)}</span> or more for
    expenses are candidates for a <span class="font-semibold text-ink">1099-NEC</span>. This is advisory — confirm
    payment method (cash/check vs. card) and contractor status before filing. Totals come from receipts matched to a vendor.
  </div>

  {#if error}
    <p class="status-message-sm status-error">{error}</p>
  {:else if loading || !result}
    <p class="text-sm text-muted">Loading report…</p>
  {:else}
    <div class="rounded-2xl border border-line bg-surface p-6">
      <div class="flex items-center justify-between">
        <h2 class="text-lg font-semibold">Vendors</h2>
        <span class="text-sm text-muted">{flaggedCount} flagged for 1099</span>
      </div>
      {#if result.rows.length === 0}
        <div class="mt-4 rounded-2xl border border-dashed border-line-strong bg-paper px-4 py-5 text-sm text-muted">
          No vendor-matched expense payments in this year. Vendor totals require receipts resolved to a vendor.
        </div>
      {:else}
        <div class="mt-4 overflow-x-auto">
          <table class="w-full min-w-[36rem] text-sm">
            <thead>
              <tr class="border-b border-line text-left text-xs uppercase tracking-[0.14em] text-muted">
                <th class="py-2 pr-4 font-semibold">Vendor</th>
                <th class="py-2 pr-4 font-semibold">Tax ID</th>
                <th class="py-2 pl-4 text-right font-semibold">Total paid</th>
                <th class="py-2 pl-4 text-right font-semibold">1099?</th>
              </tr>
            </thead>
            <tbody>
              {#each result.rows as row}
                <tr class="border-b border-line/60">
                  <td class="py-2 pr-4 align-top font-medium">
                    <a class="hover:underline" href="/vendors">{row.vendor_name}</a>
                  </td>
                  <td class="py-2 pr-4 align-top text-muted">{row.tax_id || '—'}</td>
                  <td class="py-2 pl-4 text-right align-top font-semibold text-ink">{formatCents(row.total_cents)}</td>
                  <td class="py-2 pl-4 text-right align-top">
                    {#if row.needs_1099}
                      <span class="status-pill status-warning">Review</span>
                    {:else}
                      <span class="text-muted">—</span>
                    {/if}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </div>
  {/if}
</section>
