<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { activeEntity, entities } from '$lib/stores/entity';
  import { apiFetch } from '$lib/api/client';
  import { onMount } from 'svelte';

  type TaxReadiness = {
    ready: boolean;
    exception_count: number;
    posted_entry_line_count: number;
    tax_use_profile?: {
      tax_year: number;
      status: string;
      home_office_sqft?: number | null;
      home_total_sqft?: number | null;
      home_utilities_business_use_percent?: number | null;
      cell_phone_business_use_percent?: number | null;
      home_internet_business_use_percent?: number | null;
      href?: string;
    };
    account_role_coverage?: {
      utilities_count: number;
      cell_phone_count: number;
      internet_count: number;
      href?: string;
    };
    import_summary: Array<{
      import_id: string;
      file: string;
      status: string;
      row_count: string;
      parsed_rows: string;
      error_rows: string;
      outflow_cents: string;
      inflow_cents: string;
      posted_outflow_cents: string;
      posted_inflow_cents: string;
      mapped_rows: string;
      posted_rows: string;
      unposted_rows: string;
      duplicate_rows: string;
    }>;
    blocking_summary: Array<{
      source_id: string;
      source_name: string;
      kind: string;
      status: string;
      issue_count: number;
      unmapped_rows: number;
      duplicate_rows: number;
      not_posted: number;
      parse_errors: number;
      first_row_index: string;
      href: string;
    }>;
    actions: Array<{
      kind: string;
      label: string;
      count: number;
      href: string;
      priority: number;
    }>;
  };

  type MileageSummary = {
    entity_id: string;
    start_date: string;
    end_date: string;
    rows: Array<{
      month: string;
      total_miles: number;
      trip_count: number;
      rate_cents_per_mile?: number | null;
      reimbursed_cents?: number | null;
      rate_missing?: boolean;
    }>;
  };

  const currentYear = new Date().getFullYear();
  let taxYear = $state(String(currentYear - 1));
  let loading = $state(false);
  let error = $state('');
  let readiness = $state<TaxReadiness | null>(null);
  let mileageSummary = $state<MileageSummary | null>(null);
  let activeEntityName =
    $derived($entities.find((entity) => entity.id === $activeEntity)?.name ?? '');
  let taxUseProfile = $derived(readiness?.tax_use_profile ?? null);
  let accountRoleCoverage = $derived(readiness?.account_role_coverage ?? null);
  let blockingRows = $derived(readiness?.blocking_summary ?? []);
  let totalImportRows = $derived(
    readiness?.import_summary.reduce(
      (sum, item) => sum + Number(item.row_count || 0),
      0,
    ) ?? 0,
  );
  let unmappedRows = $derived(
    blockingRows.reduce((sum, item) => sum + item.unmapped_rows, 0),
  );
  let duplicateRows = $derived(
    blockingRows.reduce((sum, item) => sum + item.duplicate_rows, 0),
  );
  let unpostedRows = $derived(
    blockingRows.reduce((sum, item) => sum + item.not_posted, 0),
  );
  let missingReceipts = $derived(
    blockingRows.filter((item) => item.kind === 'receipt').length,
  );
  let totalMileageTrips = $derived(
    mileageSummary?.rows.reduce((sum, item) => sum + item.trip_count, 0) ?? 0,
  );
  let totalMileageMiles = $derived(
    mileageSummary?.rows.reduce((sum, item) => sum + item.total_miles, 0) ?? 0,
  );
  let missingMileageRates = $derived(
    mileageSummary?.rows.filter((item) => item.rate_missing).length ?? 0,
  );
  let prepReady = $derived(
    Boolean(readiness?.ready) && missingMileageRates === 0,
  );
  let packageStatus = $derived(prepReady ? 'Ready to hand off' : 'Still blocked');

  function taxYearRange(year: string) {
    const normalized = Number(year || currentYear - 1);
    return {
      start_date: `${normalized}-01-01`,
      end_date: `${normalized}-12-31`,
    };
  }

  function formatMiles(value: number) {
    return new Intl.NumberFormat('en-US', {
      minimumFractionDigits: 1,
      maximumFractionDigits: 1,
    }).format(value);
  }

  async function loadPrep() {
    error = '';
    readiness = null;
    mileageSummary = null;
    if (!$activeEntity) {
      error = 'Select an entity first.';
      return;
    }
    if (!taxYear) {
      error = 'Enter a tax year.';
      return;
    }
    loading = true;
    try {
      const range = taxYearRange(taxYear);
      const [readinessResp, mileageResp] = await Promise.all([
        apiFetch<TaxReadiness>(
          `/reports/tax-readiness?entity_id=${encodeURIComponent($activeEntity)}&year=${encodeURIComponent(taxYear)}`,
        ),
        apiFetch<MileageSummary>(
          `/reports/mileage?entity_id=${encodeURIComponent($activeEntity)}&start_date=${encodeURIComponent(range.start_date)}&end_date=${encodeURIComponent(range.end_date)}`,
        ),
      ]);
      readiness = readinessResp;
      mileageSummary = mileageResp;
    } catch (err) {
      error = errorMessage(err, 'Unable to load tax prep checklist.');
    } finally {
      loading = false;
    }
  }

  function checklistStatusClass(ok: boolean) {
    return ok ? 'status-success' : 'status-warning';
  }

  function formatPercent(value?: number | null) {
    return typeof value === 'number' ? `${value}%` : '—';
  }

  onMount(() => {
    void loadPrep();
  });
</script>

<section class="grid gap-6">
  <div class="flex flex-wrap items-center justify-between gap-4">
    <div>
      <h1 class="text-2xl font-semibold tracking-tight">Tax prep</h1>
      <p class="mt-2 text-sm text-muted">Work through what's blocking filing, then print this page as the preparer handoff.</p>
    </div>
    <div class="flex flex-wrap gap-2">
      <button class="rounded-full bg-primary px-5 py-2 text-sm font-semibold text-paper" type="button" onclick={loadPrep}>
        {loading ? 'Checking…' : 'Check prep'}
      </button>
      <button
        class="rounded-full border border-line px-5 py-2 text-sm font-semibold disabled:opacity-60"
        type="button"
        disabled={!readiness}
        onclick={() => window.print()}
      >
        Print for handoff
      </button>
    </div>
  </div>

  <div class="grid gap-4 md:grid-cols-3">
    <div class="rounded-2xl border border-line bg-surface p-5">
      <p class="text-xs uppercase tracking-[0.2em] text-muted">Current entity</p>
      <p class="mt-2 text-lg font-semibold">{activeEntityName || 'Select entity'}</p>
    </div>
    <label class="rounded-2xl border border-line bg-surface p-5">
      <p class="text-xs uppercase tracking-[0.2em] text-muted">Tax year</p>
      <input
        class="mt-2 w-full rounded-xl border border-line px-3 py-2 text-base"
        type="number"
        min="1900"
        max="3000"
        bind:value={taxYear}
      />
    </label>
    <div class="rounded-2xl border border-line bg-surface p-5">
      <p class="text-xs uppercase tracking-[0.2em] text-muted">Status</p>
      <p class="mt-2 text-lg font-semibold">{prepReady ? 'Ready to file' : 'Needs review'}</p>
    </div>
  </div>

  {#if error}
    <p class="status-message-sm status-error">{error}</p>
  {/if}

  {#if readiness}
    <div class={`rounded-2xl border p-6 ${checklistStatusClass(prepReady)}`}>
      <h2 class="text-lg font-semibold">Checklist summary</h2>
      <p class="mt-2 text-sm">
        {prepReady
          ? 'The current scope is clean enough to package for a preparer.'
          : `${readiness.exception_count} exception(s) still need attention.`}
      </p>
      <div class="mt-4 grid gap-3 md:grid-cols-4">
        <div class="rounded-xl border border-line px-4 py-3">
          <p class="text-xs uppercase tracking-[0.16em] text-muted">Home use</p>
          <p class="mt-1 text-2xl font-semibold text-ink">
            {taxUseProfile?.status === 'complete' ? 'Set' : 'Missing'}
          </p>
          <p class="mt-1 text-sm text-muted">
            Utilities {formatPercent(taxUseProfile?.home_utilities_business_use_percent)} · Cell {formatPercent(taxUseProfile?.cell_phone_business_use_percent)} · Internet {formatPercent(taxUseProfile?.home_internet_business_use_percent)}
          </p>
        </div>
        <div class="rounded-xl border border-line px-4 py-3">
          <p class="text-xs uppercase tracking-[0.16em] text-muted">Tagged accounts</p>
          <p class="mt-1 text-2xl font-semibold text-ink">
            {(accountRoleCoverage?.utilities_count ?? 0) + (accountRoleCoverage?.cell_phone_count ?? 0) + (accountRoleCoverage?.internet_count ?? 0)}
          </p>
          <p class="mt-1 text-sm text-muted">
            Utilities {accountRoleCoverage?.utilities_count ?? 0} · Cell {accountRoleCoverage?.cell_phone_count ?? 0} · Internet {accountRoleCoverage?.internet_count ?? 0}
          </p>
        </div>
        <div class="rounded-xl border border-line px-4 py-3">
          <p class="text-xs uppercase tracking-[0.16em] text-muted">Imports</p>
          <p class="mt-1 text-2xl font-semibold text-ink">{readiness.import_summary.length}</p>
          <p class="mt-1 text-sm text-muted">{totalImportRows} rows in scope</p>
        </div>
        <div class="rounded-xl border border-line px-4 py-3">
          <p class="text-xs uppercase tracking-[0.16em] text-muted">Unmapped</p>
          <p class="mt-1 text-2xl font-semibold text-ink">{unmappedRows}</p>
          <p class="mt-1 text-sm text-muted">Rows still missing an account</p>
        </div>
        <div class="rounded-xl border border-line px-4 py-3">
          <p class="text-xs uppercase tracking-[0.16em] text-muted">Duplicates</p>
          <p class="mt-1 text-2xl font-semibold text-ink">{duplicateRows}</p>
          <p class="mt-1 text-sm text-muted">Duplicate-suspect rows</p>
        </div>
        <div class="rounded-xl border border-line px-4 py-3">
          <p class="text-xs uppercase tracking-[0.16em] text-muted">Unposted</p>
          <p class="mt-1 text-2xl font-semibold text-ink">{unpostedRows}</p>
          <p class="mt-1 text-sm text-muted">Rows still waiting to post</p>
        </div>
        <div class="rounded-xl border border-line px-4 py-3">
          <p class="text-xs uppercase tracking-[0.16em] text-muted">Receipts</p>
          <p class="mt-1 text-2xl font-semibold text-ink">{missingReceipts}</p>
          <p class="mt-1 text-sm text-muted">Posted receipts still needing review</p>
        </div>
        <div class="rounded-xl border border-line px-4 py-3">
          <p class="text-xs uppercase tracking-[0.16em] text-muted">Mileage</p>
          <p class="mt-1 text-2xl font-semibold text-ink">{totalMileageTrips}</p>
          <p class="mt-1 text-sm text-muted">{formatMiles(totalMileageMiles)} miles in scope</p>
        </div>
        <div class="rounded-xl border border-line px-4 py-3">
          <p class="text-xs uppercase tracking-[0.16em] text-muted">Rate gaps</p>
          <p class="mt-1 text-2xl font-semibold text-ink">{missingMileageRates}</p>
          <p class="mt-1 text-sm text-muted">Months missing a mileage rate</p>
        </div>
      </div>
    </div>
  {/if}

  <div class="rounded-2xl border border-line bg-surface p-6">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h2 class="text-lg font-semibold">Prepared package</h2>
        <p class="mt-1 text-sm text-muted">
          {packageStatus} for export once the checklist is clean.
        </p>
      </div>
      <a class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-paper" href="/exports">
        Open exports
      </a>
    </div>
      <div class="mt-4 grid gap-3 md:grid-cols-3">
        <div class="rounded-xl border border-line px-4 py-3">
          <p class="text-xs uppercase tracking-[0.16em] text-muted">Home use</p>
          <p class="mt-1 font-semibold text-ink">
            {taxUseProfile?.status === 'complete' ? 'Configured' : 'Needs settings'}
          </p>
        </div>
        <div class="rounded-xl border border-line px-4 py-3">
          <p class="text-xs uppercase tracking-[0.16em] text-muted">Tagged accounts</p>
          <p class="mt-1 font-semibold text-ink">
            {(accountRoleCoverage?.utilities_count ?? 0) + (accountRoleCoverage?.cell_phone_count ?? 0) + (accountRoleCoverage?.internet_count ?? 0)}
          </p>
        </div>
        <div class="rounded-xl border border-line px-4 py-3">
          <p class="text-xs uppercase tracking-[0.16em] text-muted">Tax pack</p>
          <p class="mt-1 font-semibold text-ink">{prepReady ? 'Ready to download' : 'Blocked'}</p>
        </div>
      <div class="rounded-xl border border-line px-4 py-3">
        <p class="text-xs uppercase tracking-[0.16em] text-muted">Checklist</p>
        <p class="mt-1 font-semibold text-ink">{readiness ? `${readiness.exception_count} issue(s)` : 'Not loaded'}</p>
      </div>
      <div class="rounded-xl border border-line px-4 py-3">
        <p class="text-xs uppercase tracking-[0.16em] text-muted">Mileage</p>
        <p class="mt-1 font-semibold text-ink">{missingMileageRates === 0 ? 'Complete' : 'Missing rates'}</p>
      </div>
    </div>
    <p class="mt-4 text-sm text-muted">
      The export includes `prep-checklist.csv`, `prepared-package.csv`, `blocking-summary.csv`, `review-actions.csv`, and the core accounting exports.
    </p>
  </div>

  <div class="grid gap-4 md:grid-cols-[1.1fr_0.9fr]">
    <div class="rounded-2xl border border-line bg-surface p-6">
      <h2 class="text-lg font-semibold">Fix list</h2>
      <div class="mt-4 grid gap-3">
        <a class="rounded-xl border border-line px-4 py-3 hover:border-line-strong" href="/review">
          <p class="font-semibold text-ink">Review queue</p>
          <p class="mt-1 text-sm text-muted">Clear ready items, rerun OCR, and retry failed processing.</p>
        </a>
        <a class="rounded-xl border border-line px-4 py-3 hover:border-line-strong" href="/exports">
          <p class="font-semibold text-ink">Exports</p>
          <p class="mt-1 text-sm text-muted">Check tax pack blockers and download the final bundle.</p>
        </a>
        <a class="rounded-xl border border-line px-4 py-3 hover:border-line-strong" href="/mileage">
          <p class="font-semibold text-ink">Mileage</p>
          <p class="mt-1 text-sm text-muted">Fill any missing trips or reimbursement rates.</p>
        </a>
      </div>
    </div>

    <div class="rounded-2xl border border-line bg-surface p-6">
      <h2 class="text-lg font-semibold">Open blockers</h2>
      {#if blockingRows.length > 0}
        <div class="mt-4 grid gap-3">
          {#each blockingRows as item}
            <a class="rounded-xl border border-line px-4 py-3 hover:border-line-strong" href={item.href}>
              <div class="flex flex-wrap items-center justify-between gap-2">
                <p class="font-semibold text-ink">{item.source_name || item.source_id}</p>
                <span class="rounded-full bg-paper px-2 py-1 text-[10px] uppercase tracking-[0.14em] text-muted">
                  {item.status}
                </span>
              </div>
              <div class="mt-2 grid gap-1 text-sm text-muted sm:grid-cols-2">
                <p>Unmapped: {item.unmapped_rows}</p>
                <p>Duplicate: {item.duplicate_rows}</p>
                <p>Not posted: {item.not_posted}</p>
                <p>Parse errors: {item.parse_errors}</p>
              </div>
            </a>
          {/each}
        </div>
      {:else}
        <p class="mt-4 text-sm text-muted">No current import blockers in the selected scope.</p>
      {/if}
    </div>
  </div>

  {#if mileageSummary}
    <div class="rounded-2xl border border-line bg-surface p-6">
      <h2 class="text-lg font-semibold">Mileage by month</h2>
      <div class="mt-4 grid gap-3 md:grid-cols-3">
        {#each mileageSummary.rows as row}
          <div class="rounded-xl border border-line px-4 py-3">
            <div class="flex items-center justify-between gap-2">
              <p class="font-semibold text-ink">{row.month}</p>
              <span class="rounded-full bg-paper px-2 py-1 text-[10px] uppercase tracking-[0.14em] text-muted">
                {row.trip_count} trip(s)
              </span>
            </div>
            <p class="mt-2 text-sm text-muted">Miles: {row.total_miles.toFixed(1)}</p>
            {#if row.rate_missing}
              <p class="mt-1 text-xs font-semibold uppercase tracking-[0.14em] text-warning">Rate missing</p>
            {/if}
          </div>
        {/each}
      </div>
    </div>
  {/if}
</section>
