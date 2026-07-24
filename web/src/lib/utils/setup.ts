export function decideSetupRoute(required: boolean) {
  return required ? "/setup" : "/login";
}

export function isUnauthenticatedPublicRoute(pathname: string, registrationEnabled: boolean) {
  if (pathname === "/register") {
    return registrationEnabled;
  }
  return pathname === "/login" || pathname === "/setup";
}
