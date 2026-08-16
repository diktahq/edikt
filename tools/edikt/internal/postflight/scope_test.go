package postflight

import (
	"reflect"
	"testing"
)

func TestEffectiveSet(t *testing.T) {
	tests := []struct {
		name          string
		autoDetected  []string
		required      []string
		never         []string
		want          []string
	}{
		{
			name:         "empty inputs",
			autoDetected: nil,
			required:     nil,
			never:        nil,
			want:         []string{},
		},
		{
			name:         "no overlap",
			autoDetected: []string{"api", "backend"},
			required:     []string{"security", "dba"},
			never:        nil,
			want:         []string{"api", "backend", "dba", "security"},
		},
		{
			name:         "full overlap dedupes",
			autoDetected: []string{"api", "security"},
			required:     []string{"api", "security"},
			never:        nil,
			want:         []string{"api", "security"},
		},
		{
			name:         "never overrides required",
			autoDetected: nil,
			required:     []string{"security"},
			never:        []string{"security"},
			want:         []string{},
		},
		{
			name:         "never overrides auto",
			autoDetected: []string{"security", "dba"},
			required:     nil,
			never:        []string{"security"},
			want:         []string{"dba"},
		},
		{
			name:         "stable lexicographic sort",
			autoDetected: []string{"dba", "api"},
			required:     []string{"security"},
			never:        nil,
			want:         []string{"api", "dba", "security"},
		},
		{
			name:         "empty strings are dropped",
			autoDetected: []string{"", "api", ""},
			required:     []string{"", "security"},
			never:        []string{""},
			want:         []string{"api", "security"},
		},
		{
			name:         "never with multiple entries",
			autoDetected: []string{"security", "dba", "api"},
			required:     []string{"sre"},
			never:        []string{"security", "dba"},
			want:         []string{"api", "sre"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveSet(tt.autoDetected, tt.required, tt.never)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("EffectiveSet() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestEffectiveSet_DoesNotMutate guards against accidental mutation of the
// caller's slices. The function should be pure — input slices remain
// byte-identical after the call.
func TestEffectiveSet_DoesNotMutate(t *testing.T) {
	auto := []string{"dba", "api"}
	req := []string{"security"}
	nev := []string{"frontend"}

	autoSnap := append([]string(nil), auto...)
	reqSnap := append([]string(nil), req...)
	nevSnap := append([]string(nil), nev...)

	_ = EffectiveSet(auto, req, nev)

	if !reflect.DeepEqual(auto, autoSnap) {
		t.Errorf("autoDetected mutated: got %v, want %v", auto, autoSnap)
	}
	if !reflect.DeepEqual(req, reqSnap) {
		t.Errorf("required mutated: got %v, want %v", req, reqSnap)
	}
	if !reflect.DeepEqual(nev, nevSnap) {
		t.Errorf("never mutated: got %v, want %v", nev, nevSnap)
	}
}
