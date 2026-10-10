package permits

import "testing"

func TestUserVerbs_ReturnsKnownVerbs(t *testing.T) {
	verbs, err := UserVerbs()
	if err != nil {
		t.Fatalf("UserVerbs: %v", err)
	}
	if len(verbs) == 0 {
		t.Fatal("expected a non-empty verb list")
	}
	want := map[string]bool{"gov": false, "doctor": false, "verify": false, "hook": false}
	for _, v := range verbs {
		if _, ok := want[v]; ok {
			want[v] = true
		}
	}
	for v, seen := range want {
		if !seen {
			t.Errorf("expected verb %q in the permit list, not found", v)
		}
	}
}
