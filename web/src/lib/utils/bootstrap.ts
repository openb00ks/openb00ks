import { isUnauthenticatedPublicRoute } from "./setup";

// resolveBootstrapTarget decides where the app should send a browser on a fresh load, once setup status
// is known. It runs for EVERY load (authenticated or not), so it MUST respect the session — otherwise it
// bounces logged-in users to /login on every reload.
export function resolveBootstrapTarget(
  setupRequired: boolean,
  pathname: string,
  hasSession: boolean,
  registrationEnabled = false,
) {
  // Nothing works until first-run setup is done — force everyone to /setup.
  if (setupRequired) {
    return pathname === "/setup" ? null : "/setup";
  }
  const isPublic = isUnauthenticatedPublicRoute(pathname, registrationEnabled);
  if (hasSession) {
    // Authenticated: stay on the current page; only pull them off the auth pages to home.
    return isPublic ? "/" : null;
  }
  // Unauthenticated: public routes are fine; anything else needs a login.
  return isPublic ? null : "/login";
}
