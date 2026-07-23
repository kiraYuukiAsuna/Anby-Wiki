package main

import (
	"strings"
	"testing"
)

func TestValidateGate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		current  uint
		dirty    bool
		expected uint
		min      uint
		max      uint
		wantErr  string
	}{
		{name: "pass", current: 15, expected: 15, min: 14, max: 15},
		{name: "dirty", current: 15, dirty: true, expected: 15, min: 15, max: 15, wantErr: "dirty"},
		{name: "version mismatch", current: 14, expected: 15, min: 14, max: 15, wantErr: "不一致"},
		{name: "expected outside window", current: 15, expected: 15, min: 12, max: 14, wantErr: "不在镜像兼容窗口"},
		{name: "reversed window", current: 15, expected: 15, min: 16, max: 15, wantErr: "兼容窗口非法"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateGate(tt.current, tt.dirty, tt.expected, tt.min, tt.max)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateGate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateGate() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestPositiveVersion(t *testing.T) {
	t.Parallel()

	if got, err := positiveVersion("EXPECTED", "15"); err != nil || got != 15 {
		t.Fatalf("positiveVersion() = %d, %v", got, err)
	}
	for _, raw := range []string{"", "0", "-1", "dirty"} {
		if _, err := positiveVersion("EXPECTED", raw); err == nil {
			t.Fatalf("positiveVersion(%q) unexpectedly succeeded", raw)
		}
	}
}
