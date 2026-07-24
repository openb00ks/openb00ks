export function errorRecoveryHint(message: string) {
  const normalized = message.toLowerCase();

  if (normalized.includes("already attached")) {
    return "Open the linked transaction or choose a different receipt. Attached receipts cannot be posted twice.";
  }
  if (normalized.includes("permission")) {
    return "Switch to an account with the required access, or ask an admin to complete this action.";
  }
  if (normalized.includes("entity")) {
    return "Check the active entity and make sure the selected records belong to the same workspace.";
  }
  if (normalized.includes("required")) {
    return "Complete the missing fields, then try the action again.";
  }
  if (normalized.includes("invalid")) {
    return "Review the current values carefully, correct the invalid fields, and retry.";
  }

  return "";
}
