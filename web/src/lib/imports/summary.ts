export type ImportTopVendor = {
  vendor: string;
  count: number;
  total_cents: number;
};

export type ImportSummaryView = {
  row_count: number;
  parsed_rows: number;
  total_cents: number;
  top_vendor?: string;
  top_vendors: ImportTopVendor[];
};

export type ImportRowHintView = {
  row_index: number;
  date?: string;
  vendor?: string;
  amount_cents: number;
  account_id?: string;
  rule_match_type?: string;
  rule_pattern?: string;
};

type ParsedSuggestion = {
  import_summary?: ImportSummaryView;
  import_rows?: ImportRowHintView[];
};

function decodeMaybeBase64(raw: string): string {
  const trimmed = raw.trim();
  if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
    return trimmed;
  }
  try {
    if (typeof atob === 'function') {
      return atob(trimmed);
    }
  } catch {
    // fall through to Buffer decode fallback
  }
  try {
    return Buffer.from(trimmed, 'base64').toString('utf-8');
  } catch {
    return '';
  }
}

function parseParsedSuggestion(input: unknown): ParsedSuggestion | null {
  if (!input) {
    return null;
  }
  if (typeof input === 'object') {
    return input as ParsedSuggestion;
  }
  if (typeof input !== 'string') {
    return null;
  }

  const decoded = decodeMaybeBase64(input);
  if (!decoded) {
    return null;
  }
  try {
    return JSON.parse(decoded) as ParsedSuggestion;
  } catch {
    return null;
  }
}

export function readImportSummary(parsedJSON: unknown): ImportSummaryView | null {
  const parsed = parseParsedSuggestion(parsedJSON);
  const summary = parsed?.import_summary;
  if (!summary) {
    return null;
  }
  return {
    row_count: Number(summary.row_count || 0),
    parsed_rows: Number(summary.parsed_rows || 0),
    total_cents: Number(summary.total_cents || 0),
    top_vendor: summary.top_vendor || undefined,
    top_vendors: Array.isArray(summary.top_vendors) ? summary.top_vendors : []
  };
}

export function readImportRows(parsedJSON: unknown): ImportRowHintView[] {
  const parsed = parseParsedSuggestion(parsedJSON);
  const rows = parsed?.import_rows;
  if (!Array.isArray(rows)) {
    return [];
  }

  return rows
    .map((row) => ({
      row_index: Number(row.row_index || 0),
      date: row.date || undefined,
      vendor: row.vendor || undefined,
      amount_cents: Number(row.amount_cents || 0),
      account_id: row.account_id || undefined,
      rule_match_type: row.rule_match_type || undefined,
      rule_pattern: row.rule_pattern || undefined
    }))
    .filter((row) => row.row_index > 0);
}
