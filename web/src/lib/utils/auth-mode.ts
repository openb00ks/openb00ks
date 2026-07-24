export function parsePublicRegistrationEnabled(value: string | undefined) {
  if (!value) {
    return false;
  }

  switch (value.trim().toLowerCase()) {
    case "1":
    case "true":
    case "yes":
    case "on":
      return true;
    default:
      return false;
  }
}

export function publicRegistrationEnabled() {
  return parsePublicRegistrationEnabled(import.meta.env.PUBLIC_ENABLE_REGISTRATION);
}
