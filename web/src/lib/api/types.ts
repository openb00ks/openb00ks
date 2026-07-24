// Shared API response shapes. Routes that use a subset of a type should `Pick`
// the fields they need from the canonical type here rather than re-declaring it.
//
// Note: several routes declare same-named local types (e.g. `ImportRow`,
// `SuggestionRow`) that are genuinely different shapes for different endpoints —
// those are intentionally NOT shared; only true cross-route types live here.

/** A chart-of-accounts entry. Routes Pick the fields they render. */
export interface Account {
	id: string;
	name: string;
	type: string;
	code?: string;
	role_tags?: string[];
}

/** One OCR/transcription run recorded against a receipt or import. */
export interface OcrRun {
	id: string;
	provider: string;
	status: string;
	raw_text?: string;
	error?: string;
	run_version: number;
	created_at: string;
}
