package main

import "testing"

func TestParseResetAdminOptions(t *testing.T) {
	t.Parallel()

	got, err := parseResetAdminOptions([]string{
		"--email", " admin@example.test ",
		"--password", " secret ",
		"--tenant-name", " Books ",
	})
	if err != nil {
		t.Fatalf("parseResetAdminOptions error: %v", err)
	}
	if got.Email != "admin@example.test" {
		t.Fatalf("email = %q, want trimmed email", got.Email)
	}
	if got.Password != "secret" {
		t.Fatalf("password = %q, want trimmed password", got.Password)
	}
	if got.TenantName != "Books" {
		t.Fatalf("tenant name = %q, want Books", got.TenantName)
	}
}

func TestParseResetAdminOptionsRequiresCredentials(t *testing.T) {
	t.Parallel()

	tests := map[string][]string{
		"missing email":    {"--password", "secret"},
		"missing password": {"--email", "admin@example.test"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseResetAdminOptions(args); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestParseResetAdminOptionsDefaultsTenantName(t *testing.T) {
	t.Parallel()

	got, err := parseResetAdminOptions([]string{
		"--email", "admin@example.test",
		"--password", "secret",
		"--tenant-name", " ",
	})
	if err != nil {
		t.Fatalf("parseResetAdminOptions error: %v", err)
	}
	if got.TenantName != "Default Tenant" {
		t.Fatalf("tenant name = %q, want Default Tenant", got.TenantName)
	}
}
