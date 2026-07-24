// Extract a user-facing message from an unknown thrown value, falling back to a
// caller-supplied default when the value is not an Error.
export function errorMessage(err: unknown, fallback: string): string {
	return err instanceof Error ? err.message : fallback;
}
