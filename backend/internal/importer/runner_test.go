package importer

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestSourceOnlyDoesNotAcceptMachineAuthor(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"source":         map[string]any{"kind": "url", "url": "https://example.com/source.txt"},
		"ingestion_mode": IngestionModeSourceOnly, "author_actor_id": uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeSourceImportConfig(raw); err == nil {
		t.Fatal("source-only archival accepted a Proposal author")
	}
}

func TestDecodeSourceImportConfigIngestionMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		want    string
		wantErr bool
	}{
		{name: "default", want: IngestionModeWiki},
		{name: "source only", mode: IngestionModeSourceOnly, want: IngestionModeSourceOnly},
		{name: "unknown", mode: "raw", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := map[string]any{
				"source": map[string]any{
					"kind": "url",
					"url":  "https://www.rfc-editor.org/rfc/rfc1.txt",
				},
			}
			if test.mode != "" {
				config["ingestion_mode"] = test.mode
			}
			raw, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := decodeSourceImportConfig(raw)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected invalid ingestion mode")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if decoded.IngestionMode != test.want {
				t.Fatalf("ingestion mode=%q, want %q", decoded.IngestionMode, test.want)
			}
		})
	}
}
