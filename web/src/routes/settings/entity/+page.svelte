<script lang="ts">
  import { errorMessage } from '$lib/utils/errors';
  import { activeEntity, entities } from '$lib/stores/entity';
  import { apiFetch } from '$lib/api/client';

  type EntityTaxSettingsResponse = {
    tax_year: number;
    home_office_sqft?: number | null;
    home_total_sqft?: number | null;
    home_utilities_business_use_percent?: number | null;
    cell_phone_business_use_percent?: number | null;
    home_internet_business_use_percent?: number | null;
    updated_at?: string;
  };

  let taxYear = $state(String(new Date().getFullYear()));
  let homeOfficeSqFt = $state('');
  let homeTotalSqFt = $state('');
  let cellPhoneBusinessUsePercent = $state('');
  let homeInternetBusinessUsePercent = $state('');
  let taxLoading = $state(false);
  let taxError = $state('');
  let taxSaving = $state(false);
  let taxSuccess = $state('');
  let taxSettingsLoadedFor = $state('');
  let fiscalMonth = $state('1');
  let fiscalDay = $state('1');
  let fiscalSaving = $state(false);
  let fiscalError = $state('');
  let fiscalSuccess = $state('');

  let currentEntity = $derived($entities.find((entity) => entity.id === $activeEntity) ?? null);

  function activeEntityName() {
    return currentEntity?.name ?? $activeEntity;
  }

  function parseOptionalInteger(value: string, label: string, min: number, max?: number) {
    const trimmed = value.trim();
    if (trimmed === '') {
      return null;
    }
    const parsed = Number(trimmed);
    if (!Number.isInteger(parsed) || parsed < min || (max !== undefined && parsed > max)) {
      throw new Error(label);
    }
    return parsed;
  }

  function parseRequiredInteger(value: string, label: string, min: number, max?: number) {
    const trimmed = value.trim();
    if (trimmed === '') {
      throw new Error(label);
    }
    const parsed = Number(trimmed);
    if (!Number.isInteger(parsed) || parsed < min || (max !== undefined && parsed > max)) {
      throw new Error(label);
    }
    return parsed;
  }

  function formatUtilitiesPercent() {
    const office = Number.parseInt(homeOfficeSqFt, 10);
    const total = Number.parseInt(homeTotalSqFt, 10);
    if (!Number.isInteger(office) || !Number.isInteger(total) || total <= 0 || office < 0 || office > total) {
      return '—';
    }
    return `${Math.round((office / total) * 100)}%`;
  }

  function syncTaxSettings(response: EntityTaxSettingsResponse) {
    taxYear = String(response.tax_year);
    homeOfficeSqFt = response.home_office_sqft?.toString() ?? '';
    homeTotalSqFt = response.home_total_sqft?.toString() ?? '';
    cellPhoneBusinessUsePercent = response.cell_phone_business_use_percent?.toString() ?? '';
    homeInternetBusinessUsePercent = response.home_internet_business_use_percent?.toString() ?? '';
  }

  async function loadTaxSettings(entityID: string, year: string) {
    if (!entityID) {
      return;
    }
    const trimmedYear = year.trim();
    if (!trimmedYear) {
      taxError = 'Tax year is required.';
      return;
    }
    taxLoading = true;
    taxError = '';
    taxSuccess = '';
    try {
      const response = await apiFetch<EntityTaxSettingsResponse>(
        `/entities/${encodeURIComponent(entityID)}/tax-settings?year=${encodeURIComponent(trimmedYear)}`
      );
      syncTaxSettings(response);
      taxSettingsLoadedFor = entityID;
    } catch (err) {
      taxError = errorMessage(err, 'Unable to load tax settings.');
    } finally {
      taxLoading = false;
    }
  }

  async function saveTaxSettings() {
    const entityID = $activeEntity;
    if (!entityID) {
      taxError = 'Select an entity first.';
      return;
    }
    let parsedTaxYear: number;
    let parsedHomeOfficeSqFt: number | null;
    let parsedHomeTotalSqFt: number | null;
    let parsedCellPhoneBusinessUsePercent: number | null;
    let parsedHomeInternetBusinessUsePercent: number | null;
    try {
      parsedTaxYear = parseRequiredInteger(taxYear, 'Tax year is required.', 1900, 9999);
      parsedHomeOfficeSqFt = parseOptionalInteger(homeOfficeSqFt, 'Home office square feet must be a whole number.', 0);
      parsedHomeTotalSqFt = parseOptionalInteger(homeTotalSqFt, 'Total home square feet must be a whole number.', 0);
      parsedCellPhoneBusinessUsePercent = parseOptionalInteger(
        cellPhoneBusinessUsePercent,
        'Cell phone use must be between 0 and 100.',
        0,
        100
      );
      parsedHomeInternetBusinessUsePercent = parseOptionalInteger(
        homeInternetBusinessUsePercent,
        'Home internet use must be between 0 and 100.',
        0,
        100
      );
      if (
        parsedHomeOfficeSqFt !== null &&
        parsedHomeTotalSqFt !== null &&
        parsedHomeOfficeSqFt > parsedHomeTotalSqFt
      ) {
        throw new Error('Home office square feet cannot exceed total home square feet.');
      }
    } catch (err) {
      taxError = errorMessage(err, 'Unable to update tax settings.');
      return;
    }

    taxSaving = true;
    taxError = '';
    taxSuccess = '';
    try {
      const response = await apiFetch<EntityTaxSettingsResponse>(
        `/entities/${encodeURIComponent(entityID)}/tax-settings`,
        {
          method: 'PATCH',
          body: {
            tax_year: parsedTaxYear,
            home_office_sqft: parsedHomeOfficeSqFt,
            home_total_sqft: parsedHomeTotalSqFt,
            cell_phone_business_use_percent: parsedCellPhoneBusinessUsePercent,
            home_internet_business_use_percent: parsedHomeInternetBusinessUsePercent,
          }
        }
      );
      syncTaxSettings(response);
      taxSuccess = 'Tax settings updated.';
    } catch (err) {
      taxError = errorMessage(err, 'Unable to update tax settings.');
    } finally {
      taxSaving = false;
    }
  }

  async function saveFiscalYear() {
    const entityID = $activeEntity;
    if (!entityID) {
      fiscalError = 'Select an entity first.';
      return;
    }
    let parsedMonth: number;
    let parsedDay: number;
    try {
      parsedMonth = parseRequiredInteger(fiscalMonth, 'Fiscal month must be between 1 and 12.', 1, 12);
      parsedDay = parseRequiredInteger(fiscalDay, 'Fiscal day must be between 1 and 31.', 1, 31);
      const parsedDate = new Date(Date.UTC(2024, parsedMonth - 1, parsedDay));
      if (parsedDate.getUTCMonth() !== parsedMonth - 1 || parsedDate.getUTCDate() !== parsedDay) {
        throw new Error('Fiscal year start is not a valid calendar date.');
      }
    } catch (err) {
      fiscalError = errorMessage(err, 'Unable to update fiscal year.');
      return;
    }

    fiscalSaving = true;
    fiscalError = '';
    fiscalSuccess = '';
    try {
      await apiFetch(`/entities/${encodeURIComponent(entityID)}`, {
        method: 'PATCH',
        body: {
          fiscal_year_start_month: parsedMonth,
          fiscal_year_start_day: parsedDay,
        }
      });
      entities.update((rows) =>
        rows.map((entity) =>
          entity.id === entityID
            ? {
                ...entity,
                fiscal_year_start_month: parsedMonth,
                fiscal_year_start_day: parsedDay,
              }
            : entity
        )
      );
      fiscalSuccess = 'Fiscal year updated.';
    } catch (err) {
      fiscalError = errorMessage(err, 'Unable to update fiscal year.');
    } finally {
      fiscalSaving = false;
    }
  }

  $effect(() => {
    const entityID = $activeEntity;
    if (!entityID) {
      taxSettingsLoadedFor = '';
      return;
    }
    if (taxSettingsLoadedFor === entityID) {
      return;
    }
    void loadTaxSettings(entityID, taxYear);
  });

  $effect(() => {
    fiscalMonth = String(currentEntity?.fiscal_year_start_month ?? 1);
    fiscalDay = String(currentEntity?.fiscal_year_start_day ?? 1);
  });
</script>

<section class="grid gap-6">
  <a class="text-sm text-muted hover:text-ink" href="/settings">← Back to settings</a>

  <div>
    <h1 class="text-2xl font-semibold tracking-tight">Entity settings</h1>
    <p class="mt-2 text-sm text-muted">
      Scoped to {activeEntityName() || 'the active entity'}.
    </p>
  </div>

  <div class="grid gap-4 md:grid-cols-2">
    <div class="rounded-2xl border border-line bg-surface p-6 md:col-span-2">
      <h2 class="text-lg font-semibold">Home use allocation</h2>
      <p class="mt-2 text-sm text-muted">
        Store the square-foot ratio used for utilities and the business-use percentages for cell phone and home internet. Values are kept by tax year.
      </p>

      {#if !activeEntityName()}
        <div class="mt-4 status-message-sm status-info">
          Select an entity to manage home-use allocation settings.
        </div>
      {:else}
        {#if taxError}
          <p class="mt-4 status-message-sm status-error">
            {taxError}
          </p>
        {/if}
        {#if taxSuccess}
          <p class="mt-4 status-message-sm status-success">
            {taxSuccess}
          </p>
        {/if}
        {#if taxLoading}
          <p class="mt-4 text-sm text-muted">Loading tax settings…</p>
        {:else}
          <div class="mt-4 grid gap-4 lg:grid-cols-[0.7fr_1.3fr]">
            <label class="grid gap-2 text-sm font-medium text-ink">
              Tax year
              <div class="flex gap-2">
                <input
                  class="min-w-0 flex-1 rounded-xl border border-line px-3 py-2 text-base"
                  type="text"
                  inputmode="numeric"
                  pattern="[0-9]*"
                  bind:value={taxYear}
                />
                <button
                  class="rounded-full border border-line px-4 py-2 text-sm font-semibold"
                  type="button"
                  onclick={() => loadTaxSettings($activeEntity ?? '', taxYear)}
                >
                  Load
                </button>
              </div>
            </label>

            <div class="rounded-xl border border-line bg-paper px-4 py-3 text-sm text-muted">
              <p class="text-xs font-semibold uppercase tracking-[0.18em] text-muted">Utilities ratio</p>
              <p class="mt-1 text-base font-semibold text-ink">{formatUtilitiesPercent()}</p>
              <p class="mt-1">
                Used for electricity, gas, water, and similar shared home expenses when you allocate business use.
              </p>
            </div>
          </div>

          <div class="mt-4 grid gap-3 md:grid-cols-2">
            <label class="grid gap-2 text-sm font-medium text-ink">
              Home office square feet
              <input
                class="rounded-xl border border-line px-3 py-2 text-base"
                type="text"
                inputmode="numeric"
                pattern="[0-9]*"
                bind:value={homeOfficeSqFt}
              />
            </label>
            <label class="grid gap-2 text-sm font-medium text-ink">
              Total home square feet
              <input
                class="rounded-xl border border-line px-3 py-2 text-base"
                type="text"
                inputmode="numeric"
                pattern="[0-9]*"
                bind:value={homeTotalSqFt}
              />
            </label>
            <label class="grid gap-2 text-sm font-medium text-ink">
              Cell phone business-use percent
              <input
                class="rounded-xl border border-line px-3 py-2 text-base"
                type="text"
                inputmode="numeric"
                pattern="[0-9]*"
                bind:value={cellPhoneBusinessUsePercent}
              />
            </label>
            <label class="grid gap-2 text-sm font-medium text-ink">
              Home internet business-use percent
              <input
                class="rounded-xl border border-line px-3 py-2 text-base"
                type="text"
                inputmode="numeric"
                pattern="[0-9]*"
                bind:value={homeInternetBusinessUsePercent}
              />
            </label>
          </div>

          <div class="mt-4 flex gap-3">
            <button
              class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-paper disabled:opacity-60"
              type="button"
              disabled={taxSaving}
              onclick={saveTaxSettings}
            >
              {taxSaving ? 'Saving…' : 'Save home use allocation'}
            </button>
          </div>
        {/if}
      {/if}
    </div>

    <div class="rounded-2xl border border-line bg-surface p-6">
      <h2 class="text-lg font-semibold">Fiscal year</h2>
      <div class="mt-4 grid gap-3 text-sm text-muted">
        <div class="flex items-center justify-between rounded-xl border border-line px-4 py-3">
          <span>Fiscal year start</span>
          <span class="text-sm font-semibold text-ink">{fiscalMonth.padStart(2, '0')}/{fiscalDay.padStart(2, '0')}</span>
        </div>
      </div>
      <div class="mt-4 grid gap-3 sm:grid-cols-2">
        <label class="grid gap-2 text-sm font-medium text-ink">
          Fiscal start month
          <input
            class="rounded-xl border border-line px-3 py-2 text-base"
            type="text"
            inputmode="numeric"
            pattern="[0-9]*"
            bind:value={fiscalMonth}
          />
        </label>
        <label class="grid gap-2 text-sm font-medium text-ink">
          Fiscal start day
          <input
            class="rounded-xl border border-line px-3 py-2 text-base"
            type="text"
            inputmode="numeric"
            pattern="[0-9]*"
            bind:value={fiscalDay}
          />
        </label>
      </div>
      {#if fiscalError}
        <p class="mt-4 status-message-sm status-error">{fiscalError}</p>
      {/if}
      {#if fiscalSuccess}
        <p class="mt-4 status-message-sm status-success">{fiscalSuccess}</p>
      {/if}
      <div class="mt-4">
        <button
          class="rounded-full bg-primary px-4 py-2 text-sm font-semibold text-paper disabled:opacity-60"
          type="button"
          disabled={fiscalSaving || !$activeEntity}
          onclick={saveFiscalYear}
        >
          {fiscalSaving ? 'Saving…' : 'Save fiscal year'}
        </button>
      </div>
    </div>

  </div>
</section>
