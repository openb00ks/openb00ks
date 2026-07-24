<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { get } from 'svelte/store';
  import { apiFetch } from '$lib/api/client';
  import { clearSession, session } from '$lib/stores/session';

  onMount(() => {
    const current = get(session);
    const refreshToken = current?.refreshToken;
    if (refreshToken) {
      apiFetch('/auth/logout', {
        method: 'POST',
        body: { refresh_token: refreshToken }
      }).catch(() => {
        // Ignore logout failures; we still clear local session.
      });
    }
    clearSession();
    goto('/login');
  });
</script>

<p class="text-sm text-muted">Signing out…</p>
