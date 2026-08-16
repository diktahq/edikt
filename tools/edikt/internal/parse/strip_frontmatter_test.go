package parse

import "testing"

func TestStripFrontmatter(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "agent template frontmatter is removed and body starts at first content line",
			in:   "---\nmodel: opus\ntools: [Read, Grep]\n---\n\nYou are the grader.\n",
			want: "You are the grader.\n",
		},
		{
			name: "no frontmatter returns input unchanged",
			in:   "You are the grader.\nLine two.\n",
			want: "You are the grader.\nLine two.\n",
		},
		{
			name: "CRLF frontmatter is normalized and stripped",
			in:   "---\r\nmodel: opus\r\n---\r\nBody line.\r\n",
			want: "Body line.\n",
		},
		{
			name: "malformed frontmatter (no closing fence) is left intact",
			in:   "---\nmodel: opus\nno closing fence here\n",
			want: "---\nmodel: opus\nno closing fence here\n",
		},
		{
			name: "empty body after frontmatter",
			in:   "---\nmodel: opus\n---\n",
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := StripFrontmatter(tc.in)
			if got != tc.want {
				t.Fatalf("StripFrontmatter()\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

// TestStripFrontmatter_BodyNeverStartsWithDash guards the exact bug that
// broke `gov grade-compile`: a prompt value beginning with `-` (the `---`
// fence) is parsed by the claude CLI as an unknown flag. After stripping,
// the body must not start with a dash for any well-formed template.
func TestStripFrontmatter_BodyNeverStartsWithDash(t *testing.T) {
	tmpl := "---\nmodel: opus\nsubagent_type: compile-quality-grader\n---\n\nYou are the **compile-quality-grader**.\n"
	got := StripFrontmatter(tmpl)
	if len(got) == 0 {
		t.Fatal("expected non-empty body")
	}
	if got[0] == '-' {
		t.Fatalf("stripped body still begins with %q — would be parsed as a CLI flag", got[0])
	}
}
