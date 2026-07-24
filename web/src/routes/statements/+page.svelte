<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { formatCents } from '$lib/utils/money';
  import { apiFetch } from '$lib/api/client';
  import type { Account } from '$lib/api/types';
  import { activeEntity } from '$lib/stores/entity';
  import { todayLocalDate } from '$lib/utils/date';

  type AccountRow = Pick<Account, 'id' | 'name' | 'type'>;

  type ImportRow = {
    id: string;
    original_name?: string;
    status: string;
  };

  type AccountStatement = {
    id: string;
    entity_id: string;
    account_id: string;
    account_name: string;
    source_receipt_id?: string;
    source_receipt_name?: string;
    period_start: string;
    period_end: string;
    starting_balance_cents: number;
    ending_balance_cents: number;
    imported_inflow_cents: number;
    imported_outflow_cents: number;
    posted_inflow_cents: number;
    posted_outflow_cents: number;
    expected_ending_balance_cents: number;
    difference_cents: number;
    unposted_rows: number;
    status: string;
    notes?: string;
  };

  let accounts: AccountRow[] = $state([]);
  let imports: ImportRow[] = $state([]);
  let statements: AccountStatement[] = $state([]);
  let loading = $state(false);
  let error = $state('');
  let success = $state('');
  let saving = $state(false);
  let reconcilingId = $state('');

  let accountId = $state('');
  let sourceReceiptId = $state('');
  let periodStart = $state(`${new Date().getFullYear()}-01-01`);
  let periodEnd = $state(todayLocalDate());
  let startingBalance = $state('0.00');
  let endingBalance = $state('0.00');
  let notes = $state('');

  function centsFromDollars(value: string) {
    const normalized = value.trim().replace(/[$,]/g, '');
    if (normalized === '') {
      return 0;
    }
    const parsed = Number(normalized);
    if (!Number.isFinite(parsed)) {
      throw new Error('Balances must be valid amounts.');
    }
    return Math.round(parsed * 100);
  }

  function statusClass(statement: AccountStatement) {
    if (statement.status === 'reconciled' || statement.status === 'locked') {
      return 'status-success';
    }
    if (statement.difference_cents !== 0 || statement.unposted_rows > 0) {
      return 'status-error';
    }
    return 'status-info';
  }

  async function loadWorkspace() {
    if (!$activeEntity) {
      accounts = [];
      imports = [];
      statements = [];
      return;
    }
    loading = true;
    error = '';
    try {
      const [accountResp, importResp, statementResp] = await Promise.all([
        apiFetch<AccountRow[] | { rows: AccountRow[] }>(
          `/entities/${encodeURIComponent($activeEntity)}/accounts`
        ),
        apiFetch<{ rows: ImportRow[] }>(
          `/imports?entity_id=${encodeURIComponent($activeEntity)}`
        ),
        apiFetch<{ rows: AccountStatement[] }>(
          `/account-statements?entity_id=${encodeURIComponent($activeEntity)}`
        ),
      ]);
      accounts = Array.isArray(accountResp) ? accountResp : (accountResp.rows ?? []);
      imports = importResp.rows ?? [];
      statements = statementResp.rows ?? [];
      if (!accountId && accounts.length > 0) {
        accountId = accounts[0].id;
      }
    } catch (err) {
      error = errorMessage(err, 'Unable to load statements.');
    } finally {
      loading = false;
    }
  }

  async function createStatement() {
    if (!$activeEntity) {
      error = 'Select an entity before creating a statement.';
      return;
    }
    if (!accountId || !periodStart || !periodEnd) {
      error = 'Account and statement period are required.';
      return;
    }
    let starting: number;
    let ending: number;
    try {
      starting = centsFromDollars(startingBalance);
      ending = centsFromDollars(endingBalance);
    } catch (err) {
      error = errorMessage(err, 'Unable to create statement.');
      return;
    }
    saving = true;
    error = '';
    success = '';
    try {
      await apiFetch('/account-statements', {
        method: 'POST',
        body: {
          entity_id: $activeEntity,
          account_id: accountId,
          source_receipt_id: sourceReceiptId || '',
          period_start: periodStart,
          period_end: periodEnd,
          starting_balance_cents: starting,
          ending_balance_cents: ending,
          notes,
        },
      });
      success = 'Statement created.';
      notes = '';
      await loadWorkspace();
    } catch (err) {
      error = errorMessage(err, 'Unable to create statement.');
    } finally {
      saving = false;
    }
  }

  async function reconcileStatement(id: string) {
    reconcilingId = id;
    error = '';
    success = '';
    try {
      await apiFetch(`/account-statements/${encodeURIComponent(id)}/reconcile`, {
        method: 'POST',
      });
      success = 'Statement status updated.';
      await loadWorkspace();
    } catch (err) {
      error = errorMessage(err, 'Unable to reconcile statement.');
    } finally {
      reconcilingId = '';
    }
  }

  $effect(() => {
    if ($activeEntity) {
      loadWorkspace();
    }
  });

  $effect(() => {
    if (!$activeEntity) {
      accounts = [];
      imports = [];
      statements = [];
    }
  });
</script>

<section class="grid gap-6">
  <div class="flex flex-wrap items-center justify-between gap-4">
    <div>
      <h1 class="text-2xl font-semibold tracking-tight">Statements</h1>
      <p class="mt-2 text-sm text-muted">
        Track statement periods, starting and ending balances, and reconciliation differences.
      </p>
    </div>
  </div>

  {#if error}
    <p class="status-message-sm status-error">{error}</p>
  {/if}
  {#if success}
    <p class="status-message-sm status-success">{success}</p>
  {/if}

  <div class="rounded-2xl border border-line bg-surface p-6">
    <h2 class="text-lg font-semibold">New statement</h2>
    {#if !$activeEntity}
      <p class="mt-4 status-message-sm status-info">Select an entity to create statements.</p>
    {:else}
      <div class="mt-4 grid gap-3 md:grid-cols-2">
        <label class="grid gap-2 text-sm font-medium text-ink">
          Account
          <select class="rounded-xl border border-line px-3 py-2 text-base" bind:value={accountId}>
            <option value="">Select account</option>
            {#each accounts as account}
              <option value={account.id}>{account.name} ({account.type})</option>
            {/each}
          </select>
        </label>
        <label class="grid gap-2 text-sm font-medium text-ink">
          Source import
          <select class="rounded-xl border border-line px-3 py-2 text-base" bind:value={sourceReceiptId}>
            <option value="">No source import</option>
            {#each imports as item}
              <option value={item.id}>{item.original_name ?? item.id}</option>
            {/each}
          </select>
        </label>
        <label class="grid gap-2 text-sm font-medium text-ink">
          Period start
          <input class="rounded-xl border border-line px-3 py-2 text-base" type="date" bind:value={periodStart} />
        </label>
        <label class="grid gap-2 text-sm font-medium text-ink">
          Period end
          <input class="rounded-xl border border-line px-3 py-2 text-base" type="date" bind:value={periodEnd} />
        </label>
        <label class="grid gap-2 text-sm font-medium text-ink">
          Starting balance
          <input class="rounded-xl border border-line px-3 py-2 text-base" type="text" inputmode="decimal" bind:value={startingBalance} />
        </label>
        <label class="grid gap-2 text-sm font-medium text-ink">
          Ending balance
          <input class="rounded-xl border border-line px-3 py-2 text-base" type="text" inputmode="decimal" bind:value={endingBalance} />
        </label>
      </div>
      <label class="mt-3 grid gap-2 text-sm font-medium text-ink">
        Notes
        <textarea class="min-h-24 rounded-xl border border-line px-3 py-2 text-base" bind:value={notes}></textarea>
      </label>
      <p class="mt-3 text-xs text-muted">
        Use signed book balances: asset accounts are usually positive, credit-card and loan liabilities are usually negative.
      </p>
      <div class="mt-4">
        <button
          class="rounded-full bg-primary px-5 py-2 text-sm font-semibold text-paper disabled:opacity-60"
          type="button"
          disabled={saving}
          onclick={createStatement}
        >
          {saving ? 'Creating…' : 'Create statement'}
        </button>
      </div>
    {/if}
  </div>

  <div class="rounded-2xl border border-line bg-surface p-6">
    <div class="flex items-center justify-between">
      <h2 class="text-lg font-semibold">Statement reconciliation</h2>
      {#if loading}
        <span class="text-sm text-muted">Loading…</span>
      {/if}
    </div>

    {#if !$activeEntity}
      <div class="mt-4 rounded-2xl border border-dashed border-line-strong bg-paper px-4 py-5 text-sm text-muted">
        Select an entity to view its statements.
      </div>
    {:else if statements.length === 0}
      <div class="mt-4 rounded-2xl border border-dashed border-line-strong bg-paper px-4 py-5 text-sm text-muted">
        No statements recorded yet.
      </div>
    {:else}
      <div class="mt-4 grid gap-3">
        {#each statements as statement}
          <div class="grid gap-3 rounded-xl border border-line px-4 py-3 xl:grid-cols-[1.2fr_1fr_1.2fr_0.9fr_auto] xl:items-center">
            <div>
              <p class="font-semibold text-ink">{statement.account_name}</p>
              <p class="text-sm text-muted">{statement.period_start} to {statement.period_end}</p>
              {#if statement.source_receipt_name}
                <p class="mt-1 text-xs text-muted">{statement.source_receipt_name}</p>
              {/if}
            </div>
            <div class="text-sm text-muted">
              <p>Start <span class="font-semibold text-ink">{formatCents(statement.starting_balance_cents)}</span></p>
              <p>End <span class="font-semibold text-ink">{formatCents(statement.ending_balance_cents)}</span></p>
            </div>
            <div class="text-sm text-muted">
              <p>Expected <span class="font-semibold text-ink">{formatCents(statement.expected_ending_balance_cents)}</span></p>
              <p>Difference <span class="font-semibold text-ink">{formatCents(statement.difference_cents)}</span></p>
            </div>
            <div class="text-sm text-muted">
              <p>Imported {formatCents(statement.imported_inflow_cents - statement.imported_outflow_cents)}</p>
              <p>{statement.unposted_rows} unposted row(s)</p>
            </div>
            <div class="flex flex-wrap items-center gap-2">
              <span class={`status-message-xs ${statusClass(statement)}`}>{statement.status}</span>
              <button
                class="rounded-full border border-line px-4 py-2 text-sm font-semibold disabled:opacity-60"
                type="button"
                disabled={reconcilingId === statement.id}
                onclick={() => reconcileStatement(statement.id)}
              >
                {reconcilingId === statement.id ? 'Checking…' : 'Reconcile'}
              </button>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </div>
</section>
