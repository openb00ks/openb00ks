import { describe, expect, it } from 'vitest';
import { readImportRows, readImportSummary } from './summary';

describe('readImportSummary', () => {
  it('parses direct object payloads', () => {
    const summary = readImportSummary({
      import_summary: {
        row_count: 3,
        parsed_rows: 2,
        total_cents: 1234,
        top_vendor: 'Acme',
        top_vendors: [{ vendor: 'Acme', count: 2, total_cents: 1234 }]
      }
    });

    expect(summary?.row_count).toBe(3);
    expect(summary?.top_vendor).toBe('Acme');
  });

  it('parses base64-encoded json payloads', () => {
    const encoded = Buffer.from(
      JSON.stringify({
        import_summary: {
          row_count: 4,
          parsed_rows: 4,
          total_cents: 900,
          top_vendors: []
        }
      })
    ).toString('base64');

    const summary = readImportSummary(encoded);
    expect(summary?.row_count).toBe(4);
    expect(summary?.total_cents).toBe(900);
  });

  it('returns null for invalid payloads', () => {
    expect(readImportSummary('not-json')).toBeNull();
    expect(readImportSummary({ other: true })).toBeNull();
  });

  it('parses row-level import hints from object payloads', () => {
    const rows = readImportRows({
      import_rows: [
        {
          row_index: 1,
          date: '2026-01-01',
          vendor: 'Acme',
          amount_cents: 1234,
          account_id: 'acct-meals',
          rule_match_type: 'contains',
          rule_pattern: 'acme'
        },
        {
          row_index: 2,
          vendor: 'Books',
          amount_cents: 500
        }
      ]
    });

    expect(rows).toHaveLength(2);
    expect(rows[0]).toMatchObject({
      row_index: 1,
      vendor: 'Acme',
      amount_cents: 1234,
      account_id: 'acct-meals'
    });
    expect(rows[1]).toMatchObject({
      row_index: 2,
      vendor: 'Books',
      amount_cents: 500
    });
  });

  it('parses row-level import hints from base64 payloads', () => {
    const encoded = Buffer.from(
      JSON.stringify({
        import_rows: [{ row_index: 3, vendor: 'Cafe', amount_cents: 980 }]
      })
    ).toString('base64');

    const rows = readImportRows(encoded);
    expect(rows).toEqual([{ row_index: 3, vendor: 'Cafe', amount_cents: 980 }]);
  });

  it('returns empty rows for invalid import_rows payloads', () => {
    expect(readImportRows('not-json')).toEqual([]);
    expect(readImportRows({ import_rows: 'bad' })).toEqual([]);
    expect(readImportRows({ import_rows: [{ row_index: 0, amount_cents: 1 }] })).toEqual([]);
  });
});
