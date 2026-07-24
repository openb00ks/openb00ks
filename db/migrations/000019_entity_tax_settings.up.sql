CREATE TABLE entity_tax_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id UUID NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    tax_year INT NOT NULL,
    home_office_sqft INT,
    home_total_sqft INT,
    cell_phone_business_use_percent INT,
    home_internet_business_use_percent INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (entity_id, tax_year),
    CHECK (tax_year >= 1900),
    CHECK (home_office_sqft IS NULL OR home_office_sqft >= 0),
    CHECK (home_total_sqft IS NULL OR home_total_sqft >= 0),
    CHECK (
        cell_phone_business_use_percent IS NULL
        OR (cell_phone_business_use_percent >= 0 AND cell_phone_business_use_percent <= 100)
    ),
    CHECK (
        home_internet_business_use_percent IS NULL
        OR (home_internet_business_use_percent >= 0 AND home_internet_business_use_percent <= 100)
    ),
    CHECK (
        home_office_sqft IS NULL
        OR home_total_sqft IS NULL
        OR home_office_sqft <= home_total_sqft
    )
);
