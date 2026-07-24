<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { formatCents } from '$lib/utils/money';
  import { activeEntity, entities } from '$lib/stores/entity';
  import { apiFetch, apiFetchBlob } from '$lib/api/client';

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
    actions: Array<{
      kind: string;
      label: string;
      count: number;
      href: string;
      priority: number;
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
    exceptions: Array<{
      source_id: string;
      source_name: string;
      kind: string;
      status: string;
      issue: string;
      row_index: string;
      vendor: string;
      amount_cents: string;
      href?: string;
    }>;
  };

  const initialDates = getCurrentMonthDateRange();

  let startDate = $state(initialDates.startDate);
  let endDate = $state(initialDates.endDate);
  let taxYear = $state(String(new Date().getFullYear() - 1));
  let downloadError = $state('');
  let downloading = $state(false);
  let taxPackDownloading = $state(false);
  let readinessLoading = $state(false);
  let readiness = $state<TaxReadiness | null>(null);

  let activeEntityName =
    $derived($entities.find((entity) => entity.id === $activeEntity)?.name ?? '');
  let taxUseProfile = $derived(readiness?.tax_use_profile ?? null);
  let accountRoleCoverage = $derived(readiness?.account_role_coverage ?? null);
  let blockingSummary = $derived(readiness?.blocking_summary ?? []);
  let blockingTotals = $derived({
    sources: blockingSummary.length,
    unmappedRows: blockingSummary.reduce((sum, item) => sum + item.unmapped_rows, 0),
    duplicateRows: blockingSummary.reduce((sum, item) => sum + item.duplicate_rows, 0),
    notPosted: blockingSummary.reduce((sum, item) => sum + item.not_posted, 0),
    parseErrors: blockingSummary.reduce((sum, item) => sum + item.parse_errors, 0)
  });

  function blockerActionLabel(item: (typeof blockingSummary)[number]) {
    if (item.first_row_index) {
      return `Open row ${item.first_row_index}`;
    }
    return item.kind === 'receipt' ? 'Open receipt' : 'Open import';
  }

  function getCurrentMonthDateRange() {
    const now = new Date();
    const year = now.getFullYear();
    const month = String(now.getMonth() + 1).padStart(2, '0');
    const day = String(now.getDate()).padStart(2, '0');
    return {
      startDate: `${year}-${month}-01`,
      endDate: `${year}-${month}-${day}`
    };
  }

  function formatPercent(value?: number | null) {
    return typeof value === 'number' ? `${value}%` : '—';
  }

  async function downloadExport() {
    downloadError = '';
    if (!$activeEntity) {
      downloadError = 'Select an entity to export.';
      return;
    }
    if (!startDate || !endDate) {
      downloadError = 'Select a date range.';
      return;
    }
    downloading = true;
    try {
      const blob = await apiFetchBlob(
        `/exports/transactions.csv?entity_id=${encodeURIComponent($activeEntity)}&start_date=${encodeURIComponent(startDate)}&end_date=${encodeURIComponent(endDate)}`
      );
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `transactions-${startDate}-to-${endDate}.csv`;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
    } catch (err) {
      downloadError = errorMessage(err, 'Unable to download export.');
    } finally {
      downloading = false;
    }
  }

  async function downloadTaxPack() {
    downloadError = '';
    if (!$activeEntity) {
      downloadError = 'Select an entity to export.';
      return;
    }
    if (!taxYear) {
      downloadError = 'Enter a tax year.';
      return;
    }
    const validation = await checkTaxReadiness();
    if (!validation?.ready) {
      downloadError = 'Resolve the blockers above before downloading the tax pack.';
      return;
    }
    taxPackDownloading = true;
    try {
      const blob = await apiFetchBlob(
        `/exports/tax-pack.zip?entity_id=${encodeURIComponent($activeEntity)}&year=${encodeURIComponent(taxYear)}`
      );
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `tax-pack-${taxYear}.zip`;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
    } catch (err) {
      downloadError = errorMessage(err, 'Unable to download tax pack.');
    } finally {
      taxPackDownloading = false;
    }
  }

  async function checkTaxReadiness() {
    downloadError = '';
    if (!$activeEntity) {
      downloadError = 'Select an entity to check.';
      return null;
    }
    if (!taxYear) {
      downloadError = 'Enter a tax year.';
      return null;
    }
    readinessLoading = true;
    try {
      const result = await apiFetch<TaxReadiness>(
        `/reports/tax-readiness?entity_id=${encodeURIComponent($activeEntity)}&year=${encodeURIComponent(taxYear)}`
      );
      readiness = result;
      return result;
    } catch (err) {
      downloadError = errorMessage(err, 'Unable to check tax readiness.');
      return null;
    } finally {
      readinessLoading = false;
    }
  }

</script>

<section class="grid gap-6">
  <div class="flex flex-wrap items-center justify-between gap-4">
    <div>
      <h1 class="text-2xl font-semibold tracking-tight">Exports</h1>
      <p class="mt-2 text-sm text-muted">Generate CSV snapshots of posted journal entries for a specific reporting scope.</p>
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
      <h2 class="text-lg font-semibold">Export scope</h2>
      <div class="mt-4 grid gap-3 text-sm text-muted">
        <div class="rounded-xl border border-line px-4 py-3">
          Entity:
          {#if $activeEntity && activeEntityName}
            {activeEntityName}
          {:else}
            Select an entity
          {/if}
        </div>
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
      {#if downloadError}
        <p class="mt-4 status-message-sm status-error">
          {downloadError}
        </p>
      {/if}
      <div class="mt-4 flex gap-3">
        <button
          class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-paper disabled:opacity-60"
          type="button"
          disabled={downloading || !$activeEntity}
          onclick={downloadExport}
        >
          {downloading ? 'Generating…' : 'Download CSV'}
        </button>
      </div>
    </div>

    <div class="rounded-2xl border border-line bg-surface p-6">
      <h2 class="text-lg font-semibold">Export summary</h2>
      <p class="mt-2 text-sm text-muted">Exports are read-only outputs of the selected reporting scope.</p>
      <div class="mt-4 grid gap-3 text-sm text-muted">
        <div class="rounded-xl border border-line px-4 py-3">Exports include posted journal entry lines and optional receipt URLs.</div>
        <div class="rounded-xl border border-line px-4 py-3">Tax packs also include profit-loss, general ledger, transactions, mileage, import summaries, review actions, blocking summaries, exceptions, and a README checklist.</div>
        <div class="rounded-xl border border-line px-4 py-3">
          Home-use allocation:
          {taxUseProfile?.status === 'complete'
            ? `utilities ${formatPercent(taxUseProfile.home_utilities_business_use_percent)}, cell ${formatPercent(taxUseProfile.cell_phone_business_use_percent)}, internet ${formatPercent(taxUseProfile.home_internet_business_use_percent)}`
            : 'set at the entity level before filing'}
        </div>
        <div class="rounded-xl border border-line px-4 py-3">
          Tagged accounts:
          {((accountRoleCoverage?.utilities_count ?? 0) + (accountRoleCoverage?.cell_phone_count ?? 0) + (accountRoleCoverage?.internet_count ?? 0)).toString()}
          total
        </div>
        <div class="rounded-xl border border-line px-4 py-3">Regenerate the file if you edit entries inside the selected date range.</div>
        <a class="rounded-xl border border-line px-4 py-3 text-ink hover:border-line-strong" href="/tax-prep">
          <p class="font-semibold">Open tax prep</p>
          <p class="mt-1 text-sm text-muted">See the blocker checklist before downloading the pack.</p>
        </a>
      </div>
      <div class="mt-5 grid gap-3">
        <label class="grid gap-2 text-sm font-medium text-ink">
          Tax year
          <input
            class="rounded-xl border border-line px-3 py-2 text-base"
            type="number"
            min="1900"
            max="3000"
            bind:value={taxYear}
          />
        </label>
        <button
          class="rounded-full border border-ink/20 px-4 py-2 text-sm font-semibold disabled:opacity-60"
          type="button"
          disabled={taxPackDownloading || !$activeEntity}
          onclick={downloadTaxPack}
        >
          {taxPackDownloading ? 'Generating…' : 'Download tax pack'}
        </button>
        <button
          class="rounded-full border border-ink/20 px-4 py-2 text-sm font-semibold disabled:opacity-60"
          type="button"
          disabled={readinessLoading || !$activeEntity}
          onclick={checkTaxReadiness}
        >
          {readinessLoading ? 'Checking…' : 'Check readiness'}
        </button>
        {#if readiness}
          <div class={`rounded-xl border px-4 py-3 text-sm ${readiness.ready ? 'status-success' : 'status-warning'}`}>
            <p class="font-semibold">
              {readiness.ready ? 'Ready for tax pack review' : `${readiness.exception_count} exception(s) need review`}
            </p>
            <p class="mt-1">
              Posted entry lines: {readiness.posted_entry_line_count}
            </p>
          </div>
          {#if !readiness.ready}
            <div class="grid gap-3 md:grid-cols-4">
              <div class="rounded-xl border border-line px-4 py-3 text-sm text-muted">
                <p class="text-[10px] font-semibold uppercase tracking-[0.16em] text-muted">Imports</p>
                <p class="mt-1 text-2xl font-semibold text-ink">{blockingTotals.sources}</p>
                <p class="mt-1">Sources with open tax issues</p>
              </div>
              <div class="rounded-xl border border-line px-4 py-3 text-sm text-muted">
                <p class="text-[10px] font-semibold uppercase tracking-[0.16em] text-muted">Unmapped</p>
                <p class="mt-1 text-2xl font-semibold text-ink">{blockingTotals.unmappedRows}</p>
                <p class="mt-1">Rows still missing an account</p>
              </div>
              <div class="rounded-xl border border-line px-4 py-3 text-sm text-muted">
                <p class="text-[10px] font-semibold uppercase tracking-[0.16em] text-muted">Duplicates</p>
                <p class="mt-1 text-2xl font-semibold text-ink">{blockingTotals.duplicateRows}</p>
                <p class="mt-1">Duplicate-suspect rows</p>
              </div>
              <div class="rounded-xl border border-line px-4 py-3 text-sm text-muted">
                <p class="text-[10px] font-semibold uppercase tracking-[0.16em] text-muted">Not posted</p>
                <p class="mt-1 text-2xl font-semibold text-ink">{blockingTotals.notPosted + blockingTotals.parseErrors}</p>
                <p class="mt-1">Receipts or rows that still need work</p>
              </div>
            </div>
          {/if}
          {#if readiness.actions.length > 0}
            <div class="rounded-xl border border-line px-4 py-3 text-xs text-muted">
              <p class="text-[10px] font-semibold uppercase tracking-[0.16em] text-muted">
                Fix first
              </p>
              <div class="mt-2 grid gap-2">
                {#each readiness.actions as item}
                  <a class="flex items-center justify-between gap-3 rounded-lg border border-line/80 px-3 py-2 text-ink hover:border-ink/30" href={item.href}>
                    <span class="font-semibold">{item.label}</span>
                    <span class="rounded-full bg-paper px-2 py-1 text-[10px] text-muted">
                      {item.count}
                    </span>
                  </a>
                {/each}
              </div>
            </div>
          {/if}
          {#if blockingSummary.length > 0}
            <div class="rounded-xl border border-line px-4 py-3 text-xs text-muted">
              <p class="text-[10px] font-semibold uppercase tracking-[0.16em] text-muted">
                Blocking summary
              </p>
              <div class="mt-2 grid gap-2">
                {#each blockingSummary as item}
                  <a class="rounded-lg border border-line/80 px-3 py-2 text-ink hover:border-ink/30" href={item.href}>
                    <div class="flex flex-wrap items-center justify-between gap-2">
                      <p class="font-semibold">{item.source_name || item.source_id}</p>
                      <span class="rounded-full bg-paper px-2 py-1 text-[10px] text-muted">
                        {item.status}
                      </span>
                    </div>
                    <div class="mt-2 grid gap-1 sm:grid-cols-2">
                      <p>Issues: {item.issue_count}</p>
                      <p>Unmapped rows: {item.unmapped_rows}</p>
                      <p>Duplicate rows: {item.duplicate_rows}</p>
                      <p>Not posted: {item.not_posted}</p>
                      {#if item.parse_errors}
                        <p>Parse errors: {item.parse_errors}</p>
                      {/if}
                    </div>
                    <p class="mt-2 text-[10px] font-semibold uppercase tracking-[0.14em] text-muted">
                      {blockerActionLabel(item)}
                    </p>
                  </a>
                {/each}
              </div>
            </div>
          {/if}
          {#if readiness.exceptions.length > 0}
            <div class="max-h-64 overflow-auto rounded-xl border border-line px-4 py-3 text-xs text-muted">
              <p class="text-[10px] font-semibold uppercase tracking-[0.16em] text-muted">
                Exceptions
              </p>
              <div class="mt-2 grid gap-2">
                {#each readiness.exceptions.slice(0, 20) as item}
                  <a class="rounded-lg border border-line/80 px-3 py-2 text-ink hover:border-ink/30" href={item.href || '#'}>
                    <p class="font-semibold">{item.issue}</p>
                    <p class="mt-1 text-muted">
                      {item.source_name || item.source_id}
                      {#if item.row_index}
                        · row {item.row_index}
                      {/if}
                      {#if item.vendor}
                        · {item.vendor}
                      {/if}
                    </p>
                  </a>
                {/each}
              </div>
            </div>
          {/if}
          {#if readiness.import_summary.length > 0}
            <div class="max-h-64 overflow-auto rounded-xl border border-line px-4 py-3 text-xs text-muted">
              <p class="text-[10px] font-semibold uppercase tracking-[0.16em] text-muted">
                Import reconciliation
              </p>
              <div class="mt-2 grid gap-2">
                {#each readiness.import_summary as item}
                  <div class="rounded-lg border border-line/80 px-3 py-2">
                    <div class="flex flex-wrap items-center justify-between gap-2">
                      <p class="font-semibold text-ink">{item.file || item.import_id}</p>
                      <p class="text-[10px] uppercase tracking-[0.12em] text-muted">
                        {item.status}
                      </p>
                    </div>
                    <div class="mt-2 grid gap-1 sm:grid-cols-2">
                      <p>Rows: {item.row_count} · mapped {item.mapped_rows || '0'} · posted {item.posted_rows || '0'}</p>
                      <p>Unposted: {item.unposted_rows || '0'}</p>
                      <p>Imported outflow: {formatCents(item.outflow_cents)}</p>
                      <p>Posted outflow: {formatCents(item.posted_outflow_cents)}</p>
                      <p>Imported inflow: {formatCents(item.inflow_cents)}</p>
                      <p>Posted inflow: {formatCents(item.posted_inflow_cents)}</p>
                      {#if item.duplicate_rows}
                        <p class="text-warning">Duplicate rows: {item.duplicate_rows}</p>
                      {/if}
                    </div>
                  </div>
                {/each}
              </div>
            </div>
          {/if}
        {/if}
      </div>
    </div>
  </div>

</section>

