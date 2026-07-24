# Frontend design (SvelteKit + Tailwind)

## Principles

- Mobile-first layouts and navigation.
- Simple flows: capture -> confirm -> post.
- Keep API client modular for future mobile wrapper.

## App structure (web)

- `web/src/lib/api`: API client, auth token handling.
- `web/src/lib/stores`: session + settings (server URL).
- `web/src/routes`: pages and layouts.

## Mobile-ready decisions

- Provide a "Server URL" settings screen concept (feature-flagged).
- Keep upload UX simple and fast (camera or file picker).
- Use Tailwind with a small design token set.
