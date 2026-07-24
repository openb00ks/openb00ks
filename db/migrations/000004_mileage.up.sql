CREATE TABLE mileage_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  entity_id UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
  user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  receipt_id UUID REFERENCES receipts(id) ON DELETE SET NULL,
  date DATE NOT NULL,
  distance_miles NUMERIC(10,3) NOT NULL,
  start_location TEXT,
  end_location TEXT,
  purpose TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_mileage_logs_entity_date ON mileage_logs(entity_id, date);
CREATE INDEX idx_mileage_logs_user_id ON mileage_logs(user_id);
CREATE INDEX idx_mileage_logs_receipt_id ON mileage_logs(receipt_id);
