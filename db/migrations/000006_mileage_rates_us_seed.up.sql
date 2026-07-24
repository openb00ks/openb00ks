-- US IRS standard mileage rates (default seed). Insert only if table is empty.
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM mileage_rates) THEN
    INSERT INTO mileage_rates (year, rate_cents_per_mile) VALUES
      (2020, 57),
      (2021, 56),
      (2022, 58),
      (2023, 65),
      (2024, 67),
      (2025, 67);
  END IF;
END $$;
