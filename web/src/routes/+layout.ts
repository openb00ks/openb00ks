// Single-page app: render entirely on the client. The UI calls the API from the browser
// (same-origin /api behind Cloudflare Access), so there is no server-render step — this makes
// adapter-static emit the SPA fallback (index.html) instead of trying to prerender routes.
export const ssr = false;
export const prerender = false;
