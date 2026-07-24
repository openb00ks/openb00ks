# Mobile strategy (SvelteKit + Tailwind + Capacitor)

## Why this combo

- SvelteKit keeps the web app simple and fast to iterate.
- Capacitor wraps the web UI into native shells for iOS/Android later.
- Tailwind provides consistent design tokens across web and mobile shell.

## How Capacitor fits

- The SvelteKit build output is packaged into a native container.
- Native features (camera, filesystem, notifications) are accessed via Capacitor plugins.
- No React Native or separate UI codebase required.

## Structure decisions now (to keep mobile-ready)

- Keep API access in a single web client module (easy to reuse).
- Avoid direct use of `window` in shared components; guard browser-only APIs.
- Store server base URL in app settings (for future mobile config screen).
- Prefer fetch wrappers that can be swapped for Capacitor HTTP plugin if needed.
- Use a single responsive layout with mobile-first styles.

## Community best practice alignment

- SvelteKit app in `/web`.
- Capacitor project created later under `/web` (recommended by Capacitor docs).
- Keep backend (`go.mod`) at repo root for simplicity.
