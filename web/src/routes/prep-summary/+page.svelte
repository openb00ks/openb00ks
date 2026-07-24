<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

  // The preparer summary merged into Tax prep (interactive checklist + printable handoff on one page).
  onMount(() => {
    void goto('/tax-prep', { replaceState: true });
  });
</script>

<p class="text-sm text-muted">Redirecting to Tax prep…</p>
