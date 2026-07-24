DROP TABLE IF EXISTS account_statements;

ALTER TABLE entities
  DROP CONSTRAINT IF EXISTS entities_fiscal_year_start_day_check,
  DROP CONSTRAINT IF EXISTS entities_fiscal_year_start_month_check;

ALTER TABLE entities
  DROP COLUMN IF EXISTS fiscal_year_start_day,
  DROP COLUMN IF EXISTS fiscal_year_start_month;
