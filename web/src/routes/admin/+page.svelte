<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { formatDateTime as formatDate } from '$lib/utils/date';
  import { onMount } from 'svelte';
  import { apiFetch } from '$lib/api/client';
  import AdminGuard from '$lib/components/AdminGuard.svelte';

  type AdminStats = {
    jobs_by_status: Record<string, number>;
    jobs_by_stage: Record<string, number>;
    unresolved_errors: number;
    receipts_by_status: Record<string, number>;
  };

  type Job = {
    id: string;
    receipt_id: string;
    stage: string;
    status: string;
    attempts: number;
    max_attempts: number;
    last_error: string;
    updated_at: string;
    created_at: string;
  };

  type ProcessingError = {
    id: string;
    entity_id: string;
    receipt_id: string;
    mileage_id: string;
    stage: string;
    error: string;
    created_at: string;
    resolved_at: string | null;
    resolution_note: string;
  };

  let stats = $state<AdminStats | null>(null);
  let jobs = $state<Job[]>([]);
  let processingErrors = $state<ProcessingError[]>([]);

  let statsLoading = $state(false);
  let jobsLoading = $state(false);
  let errorsLoading = $state(false);

  let statsError = $state('');
  let jobsError = $state('');
  let errorsError = $state('');

  let jobStatusFilter = $state('');
  let jobStageFilter = $state('');
  let errorResolvedFilter = $state('false');

  let requeueingId = $state('');
  let resolvingId = $state('');
  let resolveNote = $state('');
  let resolveTargetId = $state('');
  let actionError = $state('');
  let actionSuccess = $state('');

  let activeTab = $state<'queue' | 'errors'>('queue');

  function statusClass(status: string) {
    if (status === 'done') return 'status-pill status-success';
    if (status === 'failed' || status === 'dead') return 'status-pill status-error';
    if (status === 'processing') return 'status-pill status-info';
    return 'status-pill';
  }

  async function loadStats() {
    statsLoading = true;
    statsError = '';
    try {
      stats = await apiFetch<AdminStats>('/admin/stats');
    } catch (err) {
      statsError = errorMessage(err, 'Unable to load stats.');
    } finally {
      statsLoading = false;
    }
  }

  async function loadJobs() {
    jobsLoading = true;
    jobsError = '';
    try {
      const params = new URLSearchParams({ limit: '50', offset: '0' });
      if (jobStatusFilter) params.set('status', jobStatusFilter);
      if (jobStageFilter) params.set('stage', jobStageFilter);
      const res = await apiFetch<{ jobs: Job[] }>(`/admin/queue/jobs?${params}`);
      jobs = res.jobs ?? [];
    } catch (err) {
      jobsError = errorMessage(err, 'Unable to load jobs.');
    } finally {
      jobsLoading = false;
    }
  }

  async function loadErrors() {
    errorsLoading = true;
    errorsError = '';
    try {
      const params = new URLSearchParams({ limit: '50', offset: '0' });
      if (errorResolvedFilter !== '') params.set('resolved', errorResolvedFilter);
      const res = await apiFetch<{ errors: ProcessingError[] }>(`/admin/processing-errors?${params}`);
      processingErrors = res.errors ?? [];
    } catch (err) {
      errorsError = errorMessage(err, 'Unable to load errors.');
    } finally {
      errorsLoading = false;
    }
  }

  async function requeueJob(job: Job) {
    actionError = '';
    actionSuccess = '';
    requeueingId = job.id;
    try {
      await apiFetch(`/admin/queue/jobs/${job.id}/requeue`, { method: 'POST' });
      actionSuccess = `Job ${job.id.slice(0, 8)}… requeued.`;
      await loadJobs();
      await loadStats();
    } catch (err) {
      actionError = errorMessage(err, 'Unable to requeue job.');
    } finally {
      requeueingId = '';
    }
  }

  async function resolveError() {
    if (!resolveTargetId) return;
    actionError = '';
    actionSuccess = '';
    resolvingId = resolveTargetId;
    try {
      await apiFetch(`/admin/processing-errors/${resolveTargetId}/resolve`, {
        method: 'POST',
        body: JSON.stringify({ note: resolveNote })
      });
      actionSuccess = 'Error marked as resolved.';
      resolveTargetId = '';
      resolveNote = '';
      await loadErrors();
      await loadStats();
    } catch (err) {
      actionError = errorMessage(err, 'Unable to resolve error.');
    } finally {
      resolvingId = '';
    }
  }

  onMount(() => {
    void loadStats();
    void loadJobs();
    void loadErrors();
  });
</script>

<AdminGuard>
<section class="grid gap-6">
  <div>
    <h1 class="text-2xl font-semibold tracking-tight">Admin</h1>
    <p class="mt-2 text-sm text-muted">Queue health, processing errors, and system state.</p>
  </div>

  {#if actionError}
    <p class="status-message-sm status-error">{actionError}</p>
  {/if}
  {#if actionSuccess}
    <p class="status-message-sm status-success">{actionSuccess}</p>
  {/if}

  <!-- Stats cards -->
  <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
    {#if statsLoading}
      <p class="col-span-full text-sm text-muted">Loading stats…</p>
    {:else if statsError}
      <p class="col-span-full status-message-sm status-error">{statsError}</p>
    {:else if stats}
      <div class="rounded-2xl border border-line bg-surface p-5">
        <p class="text-xs font-medium uppercase tracking-wide text-muted">Queued</p>
        <p class="mt-2 text-3xl font-semibold">{stats.jobs_by_status['queued'] ?? 0}</p>
      </div>
      <div class="rounded-2xl border border-line bg-surface p-5">
        <p class="text-xs font-medium uppercase tracking-wide text-muted">Processing</p>
        <p class="mt-2 text-3xl font-semibold">{stats.jobs_by_status['processing'] ?? 0}</p>
      </div>
      <div class="rounded-2xl border border-line bg-surface p-5">
        <p class="text-xs font-medium uppercase tracking-wide text-muted">Failed / Dead</p>
        <p class="mt-2 text-3xl font-semibold text-error">
          {(stats.jobs_by_status['failed'] ?? 0) + (stats.jobs_by_status['dead'] ?? 0)}
        </p>
      </div>
      <div class="rounded-2xl border border-line bg-surface p-5">
        <p class="text-xs font-medium uppercase tracking-wide text-muted">Unresolved Errors</p>
        <p class="mt-2 text-3xl font-semibold {stats.unresolved_errors > 0 ? 'text-error' : ''}">
          {stats.unresolved_errors}
        </p>
      </div>
    {/if}
  </div>

  <!-- Tabs -->
  <div class="flex gap-2 border-b border-line">
    <button
      class="px-4 py-2 text-sm font-semibold {activeTab === 'queue' ? 'border-b-2 border-ink text-ink' : 'text-muted hover:text-ink'}"
      type="button"
      onclick={() => (activeTab = 'queue')}
    >
      Queue jobs
    </button>
    <button
      class="px-4 py-2 text-sm font-semibold {activeTab === 'errors' ? 'border-b-2 border-ink text-ink' : 'text-muted hover:text-ink'}"
      type="button"
      onclick={() => (activeTab = 'errors')}
    >
      Processing errors
    </button>
  </div>

  <!-- Queue tab -->
  {#if activeTab === 'queue'}
    <div class="rounded-2xl border border-line bg-surface p-6">
      <div class="flex flex-wrap items-end gap-3">
        <div>
          <label class="mb-1 block text-xs font-medium text-muted" for="job-status">Status</label>
          <select
            id="job-status"
            class="rounded-lg border border-line bg-surface px-3 py-2 text-sm"
            bind:value={jobStatusFilter}
            onchange={loadJobs}
          >
            <option value="">All</option>
            <option value="queued">Queued</option>
            <option value="processing">Processing</option>
            <option value="failed">Failed</option>
            <option value="dead">Dead</option>
            <option value="done">Done</option>
          </select>
        </div>
        <div>
          <label class="mb-1 block text-xs font-medium text-muted" for="job-stage">Stage</label>
          <select
            id="job-stage"
            class="rounded-lg border border-line bg-surface px-3 py-2 text-sm"
            bind:value={jobStageFilter}
            onchange={loadJobs}
          >
            <option value="">All</option>
            <option value="ocr">OCR</option>
            <option value="suggest">Suggest</option>
            <option value="draft">Draft</option>
          </select>
        </div>
        <button
          class="rounded-full border border-line px-4 py-2 text-sm font-semibold"
          type="button"
          onclick={loadJobs}
          disabled={jobsLoading}
        >
          {jobsLoading ? 'Loading…' : 'Refresh'}
        </button>
      </div>

      {#if jobsError}
        <p class="mt-4 status-message-sm status-error">{jobsError}</p>
      {:else if jobsLoading}
        <p class="mt-4 text-sm text-muted">Loading jobs…</p>
      {:else if jobs.length === 0}
        <p class="mt-4 text-sm text-muted">No jobs found.</p>
      {:else}
        <div class="mt-4 grid gap-2">
          {#each jobs as job}
            <div class="grid gap-2 rounded-xl border border-line px-4 py-3 md:grid-cols-[1fr_0.6fr_0.6fr_0.6fr_auto] md:items-center">
              <div>
                <p class="text-xs font-mono text-muted">{job.receipt_id}</p>
                {#if job.last_error}
                  <p class="mt-1 text-xs text-error line-clamp-2">{job.last_error}</p>
                {/if}
              </div>
              <div><span class={statusClass(job.status)}>{job.status}</span></div>
              <div class="text-sm text-muted">{job.stage}</div>
              <div class="text-xs text-muted">{job.attempts}/{job.max_attempts} attempts</div>
              <div class="flex justify-end">
                {#if job.status === 'failed' || job.status === 'dead'}
                  <button
                    class="rounded-full border border-line px-3 py-1.5 text-xs font-semibold disabled:opacity-60"
                    type="button"
                    disabled={requeueingId === job.id}
                    onclick={() => requeueJob(job)}
                  >
                    {requeueingId === job.id ? 'Requeueing…' : 'Requeue'}
                  </button>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}

  <!-- Errors tab -->
  {#if activeTab === 'errors'}
    <div class="rounded-2xl border border-line bg-surface p-6">
      <div class="flex flex-wrap items-end gap-3">
        <div>
          <label class="mb-1 block text-xs font-medium text-muted" for="error-resolved">Show</label>
          <select
            id="error-resolved"
            class="rounded-lg border border-line bg-surface px-3 py-2 text-sm"
            bind:value={errorResolvedFilter}
            onchange={loadErrors}
          >
            <option value="false">Unresolved</option>
            <option value="true">Resolved</option>
            <option value="">All</option>
          </select>
        </div>
        <button
          class="rounded-full border border-line px-4 py-2 text-sm font-semibold"
          type="button"
          onclick={loadErrors}
          disabled={errorsLoading}
        >
          {errorsLoading ? 'Loading…' : 'Refresh'}
        </button>
      </div>

      {#if errorsError}
        <p class="mt-4 status-message-sm status-error">{errorsError}</p>
      {:else if errorsLoading}
        <p class="mt-4 text-sm text-muted">Loading errors…</p>
      {:else if processingErrors.length === 0}
        <p class="mt-4 text-sm text-muted">No errors found.</p>
      {:else}
        <div class="mt-4 grid gap-2">
          {#each processingErrors as pe}
            <div class="rounded-xl border border-line px-4 py-3">
              <div class="flex flex-wrap items-start justify-between gap-3">
                <div class="grid gap-1">
                  <div class="flex flex-wrap gap-2">
                    <span class="status-pill">{pe.stage}</span>
                    {#if pe.resolved_at}
                      <span class="status-pill status-success">Resolved</span>
                    {/if}
                  </div>
                  <p class="text-sm text-error">{pe.error}</p>
                  {#if pe.receipt_id}
                    <p class="text-xs font-mono text-muted">receipt: {pe.receipt_id}</p>
                  {/if}
                  <p class="text-xs text-muted">{formatDate(pe.created_at)}</p>
                  {#if pe.resolved_at && pe.resolution_note}
                    <p class="text-xs text-muted">Note: {pe.resolution_note}</p>
                  {/if}
                </div>
                {#if !pe.resolved_at}
                  <div class="flex shrink-0 flex-col items-end gap-2">
                    {#if resolveTargetId === pe.id}
                      <textarea
                        class="w-48 rounded-lg border border-line bg-surface px-3 py-2 text-sm"
                        rows="2"
                        placeholder="Optional note…"
                        bind:value={resolveNote}
                      ></textarea>
                      <div class="flex gap-2">
                        <button
                          class="rounded-full border border-line px-3 py-1.5 text-xs font-semibold"
                          type="button"
                          onclick={() => { resolveTargetId = ''; resolveNote = ''; }}
                        >
                          Cancel
                        </button>
                        <button
                          class="rounded-full bg-ink px-3 py-1.5 text-xs font-semibold text-white disabled:opacity-60"
                          type="button"
                          disabled={resolvingId === pe.id}
                          onclick={resolveError}
                        >
                          {resolvingId === pe.id ? 'Saving…' : 'Confirm'}
                        </button>
                      </div>
                    {:else}
                      <button
                        class="rounded-full border border-line px-3 py-1.5 text-xs font-semibold"
                        type="button"
                        onclick={() => { resolveTargetId = pe.id; resolveNote = ''; }}
                      >
                        Resolve
                      </button>
                    {/if}
                  </div>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}
</section>
</AdminGuard>
