package main

import "testing"

func TestValidateDatabaseIdentity(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, database, marker, environment string
		wantErr                             bool
	}{
		{"performance database", "wiki_perf_m7t05", databaseMarker, "test", false},
		{"shared wiki", "wiki", databaseMarker, "test", true},
		{"production-like name", "wiki_prod", databaseMarker, "test", true},
		{"missing marker", "wiki_perf_m7t05", "", "test", true},
		{"production environment", "wiki_perf_m7t05", databaseMarker, "production", true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validateDatabaseIdentity(test.database, test.marker, test.environment)
			if (err != nil) != test.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, test.wantErr)
			}
		})
	}
}

func TestApplyProfile(t *testing.T) {
	t.Parallel()
	cfg := config{Profile: "full", Workers: 1}
	if err := applyProfile(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Pages != 100000 || cfg.Revisions != 2 || cfg.Iterations != 1000 {
		t.Fatalf("unexpected full profile: %+v", cfg)
	}
}
