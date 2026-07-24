package importer

import "testing"

func TestParseCSVCommonShapes(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		raw             string
		wantRows        int
		wantErrors      int
		wantOutflow     int64
		wantInflow      int64
		wantFirstVendor string
		wantDirection   Direction
	}{
		"date description signed amount": {
			raw:             "Date,Description,Amount\n2025-01-02,Office Depot,-12.34\n2025-01-03,Client ACH,100.00\n",
			wantRows:        2,
			wantOutflow:     1234,
			wantInflow:      10000,
			wantFirstVendor: "Office Depot",
			wantDirection:   DirectionOutflow,
		},
		"payee debit credit columns": {
			raw:             "Transaction Date,Payee,Debit,Credit\n1/2/2025,Fuel Stop,45.67,\n1/5/2025,Customer,,250.00\n",
			wantRows:        2,
			wantOutflow:     4567,
			wantInflow:      25000,
			wantFirstVendor: "Fuel Stop",
			wantDirection:   DirectionOutflow,
		},
		"no header fallback": {
			raw:             "2025-01-02,Parking,-8.00\n",
			wantRows:        1,
			wantOutflow:     800,
			wantFirstVendor: "Parking",
			wantDirection:   DirectionOutflow,
		},
		"row errors are retained": {
			raw:        "Date,Merchant,Amount\nbad-date,Vendor,12.00\n2025-01-02,,abc\n",
			wantRows:   0,
			wantErrors: 3,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := ParseCSV(tt.raw)
			if len(got.Rows) != tt.wantRows {
				t.Fatalf("rows = %d, want %d; errors=%v", len(got.Rows), tt.wantRows, got.Errors)
			}
			if len(got.Errors) != tt.wantErrors {
				t.Fatalf("errors = %d, want %d: %#v", len(got.Errors), tt.wantErrors, got.Errors)
			}
			if got.Summary.OutflowCents != tt.wantOutflow {
				t.Fatalf("outflow = %d, want %d", got.Summary.OutflowCents, tt.wantOutflow)
			}
			if got.Summary.InflowCents != tt.wantInflow {
				t.Fatalf("inflow = %d, want %d", got.Summary.InflowCents, tt.wantInflow)
			}
			if tt.wantRows > 0 {
				if got.Rows[0].Vendor != tt.wantFirstVendor {
					t.Fatalf("first vendor = %q, want %q", got.Rows[0].Vendor, tt.wantFirstVendor)
				}
				if got.Rows[0].Direction != tt.wantDirection {
					t.Fatalf("first direction = %q, want %q", got.Rows[0].Direction, tt.wantDirection)
				}
				if got.Rows[0].Fingerprint == "" {
					t.Fatal("expected fingerprint")
				}
			}
		})
	}
}

func TestParseCSVDuplicateFingerprints(t *testing.T) {
	t.Parallel()

	got := ParseCSV("Date,Description,Amount\n2025-01-02,Office Depot,-12.34\n2025-01-02,Office Depot,-12.34\n")
	if len(got.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(got.Rows))
	}
	if len(got.Summary.DuplicateRows) != 2 {
		t.Fatalf("duplicate rows = %#v, want two row indexes", got.Summary.DuplicateRows)
	}
	if got.Summary.DuplicateRows[0] != 1 || got.Summary.DuplicateRows[1] != 2 {
		t.Fatalf("duplicate rows = %#v, want [1 2]", got.Summary.DuplicateRows)
	}
}
