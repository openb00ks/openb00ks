ALTER TABLE vendors DROP COLUMN IF EXISTS last_seen;
ALTER TABLE vendors DROP COLUMN IF EXISTS receipt_count;
DROP TABLE IF EXISTS vendor_aliases;
