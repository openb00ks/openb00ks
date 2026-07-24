-- Account code (chart-of-accounts numbering: assets 1xxx, liabilities 2xxx, equity 3xxx, income 4xxx,
-- expenses 5xxx+). Optional/nullable so existing accounts are unaffected; drives canonical ordering.
ALTER TABLE accounts ADD COLUMN code TEXT;
CREATE INDEX idx_accounts_entity_code ON accounts (entity_id, code);
