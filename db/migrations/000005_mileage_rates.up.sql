CREATE TABLE mileage_rates (
  year INT PRIMARY KEY,
  rate_cents_per_mile INT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
