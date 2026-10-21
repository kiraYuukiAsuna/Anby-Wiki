package ast

import "testing"

func TestCanonicalizeJSONSupportsFiniteSafeDecimals(t *testing.T) {
	got, err := CanonicalizeJSON([]byte(
		`{"z":2.1200,"a":8.5,"small":1e-7,"negative":-0.25,"zero":-0}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"a":8.5,"negative":-0.25,"small":0.0000001,"z":2.12,"zero":0}`
	if string(got) != want {
		t.Fatalf("canonical=%s, want %s", got, want)
	}
}

func TestCanonicalizeJSONRejectsUnsafeNumber(t *testing.T) {
	if _, err := CanonicalizeJSON([]byte(`{"value":9007199254740992}`)); err == nil {
		t.Fatal("expected unsafe number to be rejected")
	}
}
