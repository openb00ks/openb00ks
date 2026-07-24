const API_ERROR_MESSAGES: Record<string, string> = {
  BAD_REQUEST:
    "The request could not be completed. Check the details and try again.",
  EMAIL_ALREADY_EXISTS: "That email is already registered. Sign in instead.",
  FORBIDDEN: "You do not have permission to perform that action.",
  INVALID_CREDENTIALS: "Incorrect email or password.",
  INVALID_MFA_CHALLENGE: "Your MFA challenge expired or is invalid. Sign in again.",
  INVALID_MFA_CODE: "The MFA code is incorrect. Try again.",
  INVALID_DRAFT:
    "The draft is no longer valid. Refresh the page and review it again.",
  INVALID_TRANSACTION:
    "The transaction is invalid. Check the accounts, entity, and balancing before trying again.",
  MISSING_FIELDS:
    "Required information is missing. Fill in the required fields and try again.",
  MFA_CHALLENGE_EXPIRED:
    "Your MFA challenge expired. Sign in again to continue.",
  MFA_SETUP_REQUIRED:
    "MFA is required for this account. Set it up from user settings before signing in.",
  RECEIPT_ALREADY_ATTACHED:
    "This receipt is already attached to a posted transaction and cannot be attached again.",
  SETUP_REQUIRED: "Setup is still required before you can continue.",
};

export class ApiError extends Error {
  code?: string;
  status?: number;

  constructor(
    message: string,
    options: { code?: string; status?: number } = {},
  ) {
    super(message);
    this.name = "ApiError";
    this.code = options.code;
    this.status = options.status;
  }
}

export function extractApiErrorCode(raw: string) {
  const trimmed = raw.trim();
  if (/^[A-Z0-9_]+$/.test(trimmed)) {
    return trimmed;
  }
  const prefixed = trimmed.match(/^([A-Z0-9_]+):/);
  return prefixed?.[1];
}

export function mapApiErrorMessage(raw: string, fallback?: string) {
  const code = extractApiErrorCode(raw);
  if (code && API_ERROR_MESSAGES[code]) {
    return API_ERROR_MESSAGES[code];
  }
  return raw.trim() || fallback || "Request failed. Please try again.";
}

export function toApiError(raw: string, status?: number, fallback?: string) {
  const code = extractApiErrorCode(raw);
  return new ApiError(mapApiErrorMessage(raw, fallback), { code, status });
}

export function getApiErrorCode(error: unknown) {
  return error instanceof ApiError ? error.code : undefined;
}
