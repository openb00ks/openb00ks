DROP INDEX IF EXISTS idx_accounts_entity_code;
ALTER TABLE accounts DROP COLUMN IF EXISTS code;
