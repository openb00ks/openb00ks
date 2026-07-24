<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { onMount } from 'svelte';
  import { activeEntity, entities } from '$lib/stores/entity';
  import { apiFetch, apiFetchBlob } from '$lib/api/client';
  import { formatLocalDate, todayLocalDate } from '$lib/utils/date';

  type MileageRow = {
    id: string;
    entity_id: string;
    date: string;
    distance_miles: number;
    start_location: string;
    end_location: string;
    purpose?: string;
    receipt_id?: string;
    suggestion_context?: string;
    created_at: string;
    updated_at: string;
  };

  type MileageSummaryRow = {
    month: string;
    total_miles: number;
    trip_count: number;
    rate_cents_per_mile?: number | null;
    reimbursed_cents?: number | null;
    rate_missing?: boolean;
  };

  let showForm = $state(false);
  let editingId = $state('');
  let date = $state('');
  let from = $state('');
  let to = $state('');
  let miles = $state('');
  let purpose = $state('');
  let suggestionContext = $state('');

  let startDate = $state('');
  let endDate = $state('');

  let trips: MileageRow[] = $state([]);
  let summaryRows: MileageSummaryRow[] = $state([]);

  let loading = $state(false);
  let error = $state('');
  let saving = $state(false);
  let saveError = $state('');
  let deletingId = $state('');
  let downloadError = $state('');
  let downloading = $state(false);
  let lastLoadedKey = $state('');


  function initDates() {
    const now = new Date();
    const todayStr = todayLocalDate();
    const monthStart = formatLocalDate(new Date(now.getFullYear(), now.getMonth(), 1));
    if (!date) {
      date = todayStr;
    }
    if (!startDate) {
      startDate = monthStart;
    }
    if (!endDate) {
      endDate = todayStr;
    }
  }

  onMount(() => {
    initDates();
    if ($activeEntity) {
      void loadMileage(true);
    }
  });


  function loadKey() {
    return `${$activeEntity ?? ''}|${startDate}|${endDate}`;
  }

  async function loadMileage(force = false) {
    if (!$activeEntity || loading) {
      return;
    }
    if (!startDate || !endDate) {
      error = 'Select a start and end date.';
      return;
    }
    const currentKey = loadKey();
    if (!force && currentKey === lastLoadedKey) {
      return;
    }
    lastLoadedKey = currentKey;
    loading = true;
    error = '';
    try {
      const query = new URLSearchParams({
        entity_id: $activeEntity,
        start_date: startDate,
        end_date: endDate
      });
      const [listResp, summaryResp] = await Promise.all([
        apiFetch<{ rows: MileageRow[] }>(`/mileage?${query.toString()}`),
        apiFetch<{ rows: MileageSummaryRow[] }>(
          `/reports/mileage?${query.toString()}`
        )
      ]);
      trips = listResp.rows ?? [];
      summaryRows = summaryResp.rows ?? [];
    } catch (err) {
      error = errorMessage(err, 'Unable to load mileage logs.');
    } finally {
      loading = false;
    }
  }

  function resetForm() {
    date = todayLocalDate();
    from = '';
    to = '';
    miles = '';
    purpose = '';
    suggestionContext = '';
    editingId = '';
    showForm = false;
  }

  async function saveTrip() {
    saveError = '';
    if (!date || !from || !to || !miles) {
      saveError = 'Date, start, end, and miles are required.';
      return;
    }
    if (!$activeEntity) {
      saveError = 'Select an entity before saving mileage.';
      return;
    }
    const parsedMiles = Number.parseFloat(miles);
    if (Number.isNaN(parsedMiles) || parsedMiles <= 0) {
      saveError = 'Miles must be a positive number.';
      return;
    }
    saving = true;
    try {
      const payload = {
        entity_id: $activeEntity,
        date,
        distance_miles: parsedMiles,
        start_location: from,
        end_location: to,
        purpose,
        suggestion_context: suggestionContext.trim()
      };
      const response = await apiFetch<MileageRow>(
        editingId ? `/mileage/${editingId}` : '/mileage',
        {
          method: editingId ? 'PATCH' : 'POST',
          body: payload
        }
      );
      if (editingId) {
        trips = trips.map((trip) => (trip.id === response.id ? response : trip));
      } else {
        trips = [response, ...trips];
      }
      resetForm();
      lastLoadedKey = '';
      await loadMileage(true);
    } catch (err) {
      saveError = errorMessage(err, 'Unable to save mileage.');
    } finally {
      saving = false;
    }
  }

  async function deleteTrip(id: string) {
    if (!id) {
      return;
    }
    deletingId = id;
    try {
      await apiFetch(`/mileage/${id}`, { method: 'DELETE' });
      trips = trips.filter((trip) => trip.id !== id);
      lastLoadedKey = '';
      await loadMileage(true);
    } catch (err) {
      error = errorMessage(err, 'Unable to delete mileage.');
    } finally {
      deletingId = '';
    }
  }

  function startEdit(trip: MileageRow) {
    editingId = trip.id;
    date = trip.date;
    from = trip.start_location;
    to = trip.end_location;
    miles = String(trip.distance_miles);
    purpose = trip.purpose ?? '';
    suggestionContext = trip.suggestion_context ?? '';
    showForm = true;
  }

  function cloneTrip(trip: MileageRow) {
    editingId = '';
    date = todayLocalDate();
    from = trip.start_location;
    to = trip.end_location;
    miles = String(trip.distance_miles);
    purpose = trip.purpose ?? '';
    suggestionContext = trip.suggestion_context ?? '';
    saveError = '';
    showForm = true;
  }

  function formatMiles(value: number) {
    return value.toFixed(1);
  }

  function formatMonth(value: string) {
    const parsed = new Date(`${value}-01T00:00:00Z`);
    if (Number.isNaN(parsed.getTime())) {
      return value;
    }
    return parsed.toLocaleDateString('en-US', { month: 'short', year: 'numeric' });
  }

  async function downloadExport() {
    downloadError = '';
    if (!$activeEntity) {
      downloadError = 'Select an entity to export mileage.';
      return;
    }
    if (!startDate || !endDate) {
      downloadError = 'Select a date range.';
      return;
    }
    downloading = true;
    try {
      const query = new URLSearchParams({
        entity_id: $activeEntity,
        start_date: startDate,
        end_date: endDate
      });
      const blob = await apiFetchBlob(`/exports/mileage.csv?${query.toString()}`);
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `mileage-${startDate}-to-${endDate}.csv`;
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
  let activeEntityName =
    $derived($entities.find((entity) => entity.id === $activeEntity)?.name ?? '');
  $effect(() => {
    const key = loadKey();
    if ($activeEntity && key !== lastLoadedKey) {
      void loadMileage();
    } else if (!$activeEntity) {
      // Clear the previous entity's trips so they don't linger under the "select an entity" header.
      trips = [];
      summaryRows = [];
      lastLoadedKey = '';
    }
  });
</script>

<section class="grid gap-6">
  <div class="flex flex-wrap items-center justify-between gap-4">
    <div>
      <h1 class="text-2xl font-semibold tracking-tight">Mileage</h1>
      <p class="mt-2 text-sm text-muted">Log and review trips for reimbursement.</p>
    </div>
    <button class="rounded-full bg-primary px-5 py-2 text-sm font-semibold text-paper" onclick={() => (showForm = !showForm)}>
      {showForm ? 'Close form' : 'Log trip'}
    </button>
  </div>

  {#if showForm}
    <div class="rounded-2xl border border-line bg-surface p-6">
      <h2 class="text-lg font-semibold">{editingId ? 'Edit trip' : 'New trip'}</h2>
      <div class="mt-4 grid gap-3 text-sm text-muted md:grid-cols-4">
        <label class="grid gap-2 text-sm font-medium text-ink">
          Date
          <input class="rounded-xl border border-line px-3 py-2 text-base" type="date" bind:value={date} />
        </label>
        <label class="grid gap-2 text-sm font-medium text-ink">
          From
          <input class="rounded-xl border border-line px-3 py-2 text-base" bind:value={from} />
        </label>
        <label class="grid gap-2 text-sm font-medium text-ink">
          To
          <input class="rounded-xl border border-line px-3 py-2 text-base" bind:value={to} />
        </label>
        <label class="grid gap-2 text-sm font-medium text-ink">
          Miles
          <input class="rounded-xl border border-line px-3 py-2 text-base" type="number" step="0.1" bind:value={miles} />
        </label>
        <label class="grid gap-2 text-sm font-medium text-ink md:col-span-2">
          Purpose
          <input class="rounded-xl border border-line px-3 py-2 text-base" bind:value={purpose} />
        </label>
        <label class="grid gap-2 text-sm font-medium text-ink md:col-span-4">
          Suggestion context (optional)
          <textarea
            class="min-h-[96px] rounded-xl border border-line px-3 py-2 text-base"
            placeholder="Notes that help categorize this mileage."
            bind:value={suggestionContext}
          ></textarea>
        </label>
      </div>
      {#if saveError}
        <p class="mt-4 status-message-sm status-error">
          {saveError}
        </p>
      {/if}
      <div class="mt-4 flex gap-3">
        <button class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-paper disabled:opacity-60" onclick={saveTrip} disabled={saving}>
          {saving ? 'Saving…' : editingId ? 'Update trip' : 'Save trip'}
        </button>
        <button class="rounded-full border border-line px-4 py-2 text-sm font-semibold" onclick={resetForm}>Cancel</button>
      </div>
    </div>
  {/if}

  <div class="rounded-2xl border border-line bg-surface p-6">
    <h2 class="text-lg font-semibold">Filters</h2>
    <p class="mt-2 text-sm text-muted">Filter trips by date range.</p>
    <div class="mt-4 grid gap-3 text-sm text-muted md:grid-cols-2">
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
      <button class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-paper" type="button" onclick={() => loadMileage(true)}>
        Refresh
      </button>
      <button
        class="rounded-full border border-line px-4 py-2 text-sm font-semibold disabled:opacity-60"
        type="button"
        onclick={downloadExport}
        disabled={downloading || !$activeEntity}
      >
        {downloading ? 'Exporting…' : 'Download CSV'}
      </button>
    </div>
    {#if downloadError}
      <p class="mt-4 status-message-sm status-error">
        {downloadError}
      </p>
    {/if}
  </div>

  <div class="grid gap-4 md:grid-cols-[1.2fr_1fr]">
    <div class="rounded-2xl border border-line bg-surface p-6">
      <h2 class="text-lg font-semibold">Trip log</h2>
      <p class="mt-2 text-sm text-muted">
        {#if activeEntityName}
          {activeEntityName}
        {:else}
          Select an entity to view mileage.
        {/if}
      </p>
      {#if error}
        <p class="mt-4 status-message-sm status-error">
          {error}
        </p>
      {:else if loading}
        <p class="mt-4 text-sm text-muted">Loading trips…</p>
      {:else if trips.length === 0}
        <p class="mt-4 text-sm text-muted">No mileage logged yet.</p>
      {:else}
        <div class="mt-4 grid gap-3">
          {#each trips as trip}
            <div class="grid gap-2 rounded-xl border border-line px-4 py-3 md:grid-cols-[1.4fr_0.7fr_0.6fr_0.6fr] md:items-center">
              <div>
                <p class="text-sm font-semibold">{trip.start_location} → {trip.end_location}</p>
                <p class="text-xs text-muted">{trip.date}</p>
                {#if trip.purpose}
                  <p class="mt-1 text-xs text-muted">{trip.purpose}</p>
                {/if}
                {#if trip.suggestion_context}
                  <p class="mt-1 text-xs text-muted">{trip.suggestion_context}</p>
                {/if}
              </div>
              <div class="text-sm text-muted">{formatMiles(trip.distance_miles)} miles</div>
              <div class="text-sm text-muted">
                {trip.receipt_id ? 'Receipt attached' : 'No receipt'}
              </div>
              <div class="flex justify-end gap-2">
                <button
                  class="rounded-full border border-line px-3 py-1 text-sm font-semibold"
                  type="button"
                  onclick={() => cloneTrip(trip)}
                >
                  Clone
                </button>
                <button
                  class="rounded-full border border-line px-3 py-1 text-sm font-semibold"
                  type="button"
                  onclick={() => startEdit(trip)}
                >
                  Edit
                </button>
                <button
                  class="rounded-full border border-line px-3 py-1 text-sm font-semibold disabled:opacity-60"
                  type="button"
                  onclick={() => deleteTrip(trip.id)}
                  disabled={deletingId === trip.id}
                >
                  {deletingId === trip.id ? 'Deleting…' : 'Delete'}
                </button>
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>

    <div class="rounded-2xl border border-line bg-surface p-6">
      <h2 class="text-lg font-semibold">Monthly summary</h2>
      <p class="mt-2 text-sm text-muted">Totals and reimbursement estimates.</p>
      {#if summaryRows.length === 0}
        <p class="mt-4 text-sm text-muted">No summary available yet.</p>
      {:else}
        <div class="mt-4 grid gap-3">
          {#each summaryRows as row}
            <div class="rounded-xl border border-line px-4 py-3 text-sm text-muted">
              <p class="text-sm font-semibold text-ink">{formatMonth(row.month)}</p>
              <p class="mt-2">Trips: {row.trip_count} • {formatMiles(row.total_miles)} miles</p>
              {#if row.rate_missing}
                <p class="mt-1 text-xs text-muted">Rate missing for this year.</p>
              {:else if row.reimbursed_cents}
                <p class="mt-1 text-xs text-muted">
                  Reimbursed: ${(row.reimbursed_cents / 100).toFixed(2)}
                </p>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </div>
</section>
