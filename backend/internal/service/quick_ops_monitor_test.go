package service

import "testing"

func TestValidateQuickOpsMonitorSuffix(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantSuffix string
		wantOK     bool
	}{
		{name: "empty is allowed while disabled", input: "", wantSuffix: "", wantOK: true},
		{name: "valid custom suffix", input: "/opsview/", wantSuffix: "opsview", wantOK: true},
		{name: "existing batch image route", input: "batch-image", wantSuffix: "batch-image", wantOK: false},
		{name: "existing batch image route case insensitive", input: "BATCH-IMAGE", wantSuffix: "BATCH-IMAGE", wantOK: false},
		{name: "existing docs namespace", input: "docs", wantSuffix: "docs", wantOK: false},
		{name: "existing admin route", input: "admin", wantSuffix: "admin", wantOK: false},
		{name: "too short", input: "abc", wantSuffix: "abc", wantOK: false},
		{name: "multiple path segments", input: "bad/suffix", wantSuffix: "bad/suffix", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSuffix, gotOK := ValidateQuickOpsMonitorSuffix(tt.input)
			if gotSuffix != tt.wantSuffix {
				t.Fatalf("suffix mismatch: got %q want %q", gotSuffix, tt.wantSuffix)
			}
			if gotOK != tt.wantOK {
				t.Fatalf("validity mismatch: got %v want %v", gotOK, tt.wantOK)
			}
		})
	}
}
