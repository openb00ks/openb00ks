<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { formatCents } from '$lib/utils/money';
  import { apiFetch } from '$lib/api/client';

  type Account = { id: string; name: string; type?: string; code?: string };
  type Line = { account_id: string; debit: string; credit: string };

  interface Props {
    entityId: string;
    accounts: Account[];
    oncreated?: () => void;
  }

  let { entityId, accounts, oncreated }: Props = $props();

  function todayStr() {
    // Local YYYY-MM-DD without pulling in a date lib.
    const now = new Date();
    const m = String(now.getMonth() + 1).padStart(2, '0');
    const d = String(now.getDate()).padStart(2, '0');
    return `${now.getFullYear()}-${m}-${d}`;
  }

  function blankLine(): Line {
    return { account_id: '', debit: '', credit: '' };
  }

  let open = $state(false);
  let date = $state(todayStr());
  let memo = $state('');
  let lines = $state<Line[]>([blankLine(), blankLine()]);
  let saving = $state(false);
  let error = $state('');
  let success = $state('');

  function accountLabel(account: Account) {
    return account.code ? `${account.code} · ${account.name}` : account.name;
  }

  // Dollars string → integer cents (0 for blank/invalid).
  function toCents(value: string): number {
    const parsed = Number.parseFloat(value);
    return Number.isFinite(parsed) ? Math.round(parsed * 100) : 0;
  }

  let totalDebits = $derived(lines.reduce((sum, line) => sum + toCents(line.debit), 0));
  let totalCredits = $derived(lines.reduce((sum, line) => sum + toCents(line.credit), 0));
  let balanced = $derived(totalDebits > 0 && totalDebits === totalCredits);

  function addLine() {
    lines = [...lines, blankLine()];
  }

  function removeLine(index: number) {
    if (lines.length <= 2) return;
    lines = lines.filter((_, i) => i !== index);
  }

  function toggle() {
    open = !open;
    error = '';
    success = '';
    if (!open) {
      reset();
    }
  }

  function reset() {
    date = todayStr();
    memo = '';
    lines = [blankLine(), blankLine()];
  }

  async function submit() {
    error = '';
    success = '';
    // Keep only lines that have an account and exactly one side filled.
    const payloadLines: Array<{ account_id: string; debit_cents: number; credit_cents: number }> = [];
    for (const line of lines) {
      const debit = toCents(line.debit);
      const credit = toCents(line.credit);
      if (!line.account_id && debit === 0 && credit === 0) {
        continue; // untouched row
      }
      if (!line.account_id) {
        error = 'Every line needs an account.';
        return;
      }
      if ((debit === 0) === (credit === 0)) {
        error = 'Each line needs a debit or a credit — not both, not neither.';
        return;
      }
      payloadLines.push({ account_id: line.account_id, debit_cents: debit, credit_cents: credit });
    }
    if (payloadLines.length < 2) {
      error = 'A journal entry needs at least two lines.';
      return;
    }
    if (!balanced) {
      error = 'Debits and credits must balance.';
      return;
    }

    saving = true;
    try {
      await apiFetch('/transactions', {
        method: 'POST',
        body: { entity_id: entityId, date, memo: memo.trim(), lines: payloadLines },
      });
      success = 'Journal entry posted.';
      reset();
      open = false;
      oncreated?.();
    } catch (err) {
      error = errorMessage(err, 'Unable to post entry.');
    } finally {
      saving = false;
    }
  }
</script>

<div class="rounded-2xl border border-line bg-surface p-6">
  <div class="flex items-center justify-between gap-3">
    <div>
      <h2 class="text-lg font-semibold">Manual journal entry</h2>
      <p class="mt-1 text-sm text-muted">Post an adjustment or opening balance by hand.</p>
    </div>
    <button
      class="rounded-full bg-primary px-5 py-2 text-sm font-semibold text-paper disabled:opacity-60"
      type="button"
      disabled={!entityId}
      onclick={toggle}
    >
      {open ? 'Close' : 'New entry'}
    </button>
  </div>

  {#if success && !open}
    <p class="mt-4 status-message-sm status-success">{success}</p>
  {/if}

  {#if open}
    <div class="mt-5 grid gap-4">
      <div class="grid gap-3 sm:grid-cols-[200px_1fr]">
        <label class="grid gap-2 text-sm font-medium text-ink">
          Date
          <input class="rounded-xl border border-line px-3 py-2 text-base" type="date" bind:value={date} />
        </label>
        <label class="grid gap-2 text-sm font-medium text-ink">
          Memo
          <input
            class="rounded-xl border border-line px-3 py-2 text-base"
            type="text"
            placeholder="e.g. Owner opening balance"
            bind:value={memo}
          />
        </label>
      </div>

      <div class="grid gap-2">
        <div class="hidden gap-3 px-1 text-xs font-semibold uppercase tracking-[0.14em] text-muted sm:grid sm:grid-cols-[1fr_140px_140px_40px]">
          <span>Account</span>
          <span class="text-right">Debit</span>
          <span class="text-right">Credit</span>
          <span></span>
        </div>
        {#each lines as line, i}
          <div class="grid gap-2 rounded-xl border border-line px-3 py-3 sm:grid-cols-[1fr_140px_140px_40px] sm:items-center sm:border-0 sm:px-1 sm:py-0">
            <select class="rounded-xl border border-line px-3 py-2 text-sm" bind:value={line.account_id}>
              <option value="">Select account</option>
              {#each accounts as account}
                <option value={account.id}>{accountLabel(account)}</option>
              {/each}
            </select>
            <input
              class="rounded-xl border border-line px-3 py-2 text-right text-sm"
              type="number"
              inputmode="decimal"
              min="0"
              step="0.01"
              placeholder="0.00"
              bind:value={line.debit}
              oninput={() => { if (line.debit) line.credit = ''; }}
            />
            <input
              class="rounded-xl border border-line px-3 py-2 text-right text-sm"
              type="number"
              inputmode="decimal"
              min="0"
              step="0.01"
              placeholder="0.00"
              bind:value={line.credit}
              oninput={() => { if (line.credit) line.debit = ''; }}
            />
            <button
              class="justify-self-end rounded-full border border-line px-3 py-2 text-sm font-semibold text-muted hover:text-ink disabled:opacity-40"
              type="button"
              aria-label="Remove line"
              disabled={lines.length <= 2}
              onclick={() => removeLine(i)}
            >
              ✕
            </button>
          </div>
        {/each}
      </div>

      <div class="flex flex-wrap items-center justify-between gap-3">
        <button class="text-sm font-semibold text-primary hover:opacity-80" type="button" onclick={addLine}>
          + Add line
        </button>
        <div class="flex items-center gap-4 text-sm">
          <span class="text-muted">Debits <span class="font-semibold text-ink">{formatCents(totalDebits)}</span></span>
          <span class="text-muted">Credits <span class="font-semibold text-ink">{formatCents(totalCredits)}</span></span>
          <span class={balanced ? 'font-semibold text-success' : 'font-semibold text-error'}>
            {balanced ? 'Balanced' : 'Out of balance'}
          </span>
        </div>
      </div>

      {#if error}
        <p class="status-message-sm status-error">{error}</p>
      {/if}

      <div class="flex gap-2">
        <button
          class="rounded-full bg-primary px-5 py-2 text-sm font-semibold text-paper disabled:opacity-60"
          type="button"
          disabled={saving || !balanced}
          onclick={submit}
        >
          {saving ? 'Posting…' : 'Post entry'}
        </button>
        <button class="rounded-full border border-line px-5 py-2 text-sm font-semibold" type="button" onclick={toggle}>
          Cancel
        </button>
      </div>
    </div>
  {/if}
</div>
