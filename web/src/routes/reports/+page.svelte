<script lang="ts">
  import { onMount } from 'svelte';
  import { activeEntity, entities } from '$lib/stores/entity';
  import { formatLocalDate, todayLocalDate } from '$lib/utils/date';

  const reports = [
    { slug: 'profit-loss', name: 'Profit & Loss', description: 'Income and expense summary.' },
    { slug: 'balance-sheet', name: 'Balance Sheet', description: 'Assets, liabilities, and equity.' },
    { slug: 'trial-balance', name: 'Trial Balance', description: 'Every account as a debit or credit balance.' },
    { slug: 'general-ledger', name: 'General Ledger', description: 'All posted entries by account.' },
    { slug: 'vendor-payments', name: 'Vendor Payments (1099)', description: 'Vendor totals with 1099-NEC candidates flagged.' }
  ];

  let startDate = $state('');
  let endDate = $state('');

  let activeEntityName =
    $derived($entities.find((entity) => entity.id === $activeEntity)?.name ?? '');

  onMount(() => {
    const now = new Date();
    const todayStr = todayLocalDate();
    const monthStart = formatLocalDate(new Date(now.getFullYear(), now.getMonth(), 1));
    if (!startDate) {
      startDate = monthStart;
    }
    if (!endDate) {
      endDate = todayStr;
    }
  });

  function reportHref(slug: string) {
    const entityId = $activeEntity ?? '';
    const params = new URLSearchParams({
      entity_id: entityId,
      start_date: startDate,
      end_date: endDate
    });
    return `/reports/${slug}?${params.toString()}`;
  }
</script>

<section class="grid gap-6">
  <div class="flex flex-wrap items-center justify-between gap-4">
    <div>
      <h1 class="text-2xl font-semibold tracking-tight">Reports</h1>
      <p class="mt-2 text-sm text-muted">Review standard financial views using the current reporting scope.</p>
    </div>
  </div>

  <div class="grid gap-4 md:grid-cols-3">
    <div class="rounded-2xl border border-line bg-surface p-5">
      <p class="text-xs uppercase tracking-[0.2em] text-muted">Current entity</p>
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

  <div class="grid gap-4 md:grid-cols-[1.2fr_1fr]">
    <div class="rounded-2xl border border-line bg-surface p-6">
      <h2 class="text-lg font-semibold">Report scope</h2>
      <div class="mt-4 grid gap-3 text-sm text-muted">
        <label class="grid gap-2 text-sm font-medium text-ink">
          Entity
          <select
            class="rounded-xl border border-line px-3 py-2 text-base"
            value={$activeEntity ?? ''}
            onchange={(event) =>
              activeEntity.set(event.currentTarget.value || null)}
          >
            <option value="">Select entity</option>
            {#each $entities as entity}
              <option value={entity.id}>{entity.name}</option>
            {/each}
          </select>
        </label>
        <label class="grid gap-2 text-sm font-medium text-ink">
          Start date
          <input
            class="rounded-xl border border-line px-3 py-2 text-base"
            type="date"
            bind:value={startDate}
          />
        </label>
        <label class="grid gap-2 text-sm font-medium text-ink">
          End date
          <input
            class="rounded-xl border border-line px-3 py-2 text-base"
            type="date"
            bind:value={endDate}
          />
        </label>
      </div>
    </div>

    <div class="rounded-2xl border border-line bg-surface p-6">
      <h2 class="text-lg font-semibold">Report summary</h2>
      <div class="mt-4 grid gap-3 text-sm text-muted">
        <div class="rounded-xl border border-line px-4 py-3">
          Reports use posted transactions only.
        </div>
        <div class="rounded-xl border border-line px-4 py-3">
          {#if activeEntityName}
            Active entity: <span class="font-semibold text-ink">{activeEntityName}</span>
          {:else}
            Select an entity to run reports.
          {/if}
        </div>
        <div class="rounded-xl border border-line px-4 py-3">
          Adjust the scope first, then open the report that matches the question you need answered.
        </div>
        <a class="rounded-xl border border-line px-4 py-3 text-ink hover:border-line-strong" href="/exports">
          <p class="font-semibold">Need the tax bundle?</p>
          <p class="mt-1 text-sm text-muted">Use exports for the filing pack, mileage output, and review checklist.</p>
        </a>
        <a class="rounded-xl border border-line px-4 py-3 text-ink hover:border-line-strong" href="/tax-prep">
          <p class="font-semibold">Need the checklist?</p>
          <p class="mt-1 text-sm text-muted">Use tax prep for blockers, mileage gaps, and prep status.</p>
        </a>
      </div>
    </div>
  </div>

  <div class="rounded-2xl border border-line bg-surface p-6">
    <div class="flex items-center justify-between">
      <h2 class="text-lg font-semibold">Standard reports</h2>
      <span class="text-xs uppercase tracking-[0.2em] text-muted">Live</span>
    </div>
    {#snippet reportBody(report: { name: string; description: string })}
      <div>
        <p class="text-sm font-semibold">{report.name}</p>
        <p class="text-xs text-muted">{report.description}</p>
      </div>
      <div class="text-sm text-muted">
        {#if activeEntityName}
          {activeEntityName}
        {:else}
          Select entity
        {/if}
      </div>
      <div class="text-right text-sm font-semibold">{$activeEntity ? 'View' : '—'}</div>
    {/snippet}
    <div class="mt-4 grid gap-3">
      {#each reports as report}
        {#if $activeEntity}
          <a
            class="grid gap-2 rounded-xl border border-line px-4 py-3 hover:border-line-strong md:grid-cols-[1.6fr_1.2fr_0.8fr] md:items-center"
            href={reportHref(report.slug)}
          >
            {@render reportBody(report)}
          </a>
        {:else}
          <div
            class="grid cursor-not-allowed gap-2 rounded-xl border border-dashed border-line px-4 py-3 opacity-60 md:grid-cols-[1.6fr_1.2fr_0.8fr] md:items-center"
            aria-disabled="true"
          >
            {@render reportBody(report)}
          </div>
        {/if}
      {/each}
    </div>
  </div>
</section>
