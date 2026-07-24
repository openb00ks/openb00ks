import { isUnauthenticatedPublicRoute } from "./setup";

export function shouldRenderRouteContent(
  hasSession: boolean,
  pathname: string,
  registrationEnabled: boolean,
) {
  return (
    isUnauthenticatedPublicRoute(pathname, registrationEnabled) ||
    hasSession
  );
}
