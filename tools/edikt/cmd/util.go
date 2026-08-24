package cmd

// util.go — `bin/edikt util {json,yaml,hash,path}`, the verbs that replaced
// inline python3 across the command bodies (PLAN-v080 Phase 24, FR-010).
//
// WHY THESE EXIST AT ALL. Seventy-one python3 invocations were spread over
// fourteen command bodies, and they reduced to six things: emit a small JSON
// object, read one field out of a JSON or YAML file, hash a file, canonicalise
// YAML, check that a file parses, and make a path relative. None of that needs
// an interpreter; all of it needed one as long as tier-1 was bash, because
// INV-003 forbids building JSON by shell concatenation and bash has no other
// way.
//
// THE SHAPE THAT MATTERS IS --field, NOT THE PARSING. A command body that
// builds `{"status":"error","reason":"..."}` from shell variables is one quote
// away from emitting a broken line, and the reason python3 was reached for was
// always json.dumps rather than python itself. `util json emit --field k=v`
// takes every value VERBATIM as a string and encodes it here, so no caller
// ever concatenates.
//
// NOT A GENERAL-PURPOSE TOOLBOX. Each verb exists because a specific call site
// needed it. Adding one because it might be handy is how a CLI becomes a
// scripting language with worse ergonomics than the one it replaced.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
	"gopkg.in/yaml.v3"
)

func init() { rootCmd.AddCommand(newUtilCmd()) }

func newUtilCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "util",
		Short: "Small data verbs used by command bodies (json, yaml, hash, path)",
		Long: `Verbs that command bodies call instead of inlining python3.

Each one replaced a specific inline call. The hook and command tiers no longer
carry a script interpreter, so nothing here depends on one being installed.`,
	}
	cmd.AddCommand(newUtilJSONCmd(), newUtilYAMLCmd(), newUtilHashCmd(), newUtilPathCmd(), newUtilSchemaCmd(), newUtilRenderCmd(), newUtilNormalizeCmd(), newUtilMatchCmd(), newUtilFrontmatterCmd())
	return cmd
}

// ── json ────────────────────────────────────────────────────────────────────

func newUtilJSONCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "json", Short: "Emit, read and validate JSON"}
	cmd.AddCommand(newUtilJSONEmitCmd(), newUtilJSONGetCmd(), newUtilJSONValidateCmd(), newUtilJSONLenCmd(), newUtilJSONFormatCmd(), newUtilJSONJoinCmd(), newUtilJSONStampCmd())
	return cmd
}

func newUtilJSONEmitCmd() *cobra.Command {
	var fields, intFields, jsonFields, nullableFields []string
	var indent int
	cmd := &cobra.Command{
		Use:   "emit",
		Short: "Print one JSON object built from key=value pairs",
		Long: `Print a single JSON object on stdout.

Every --field value is taken VERBATIM as a string and encoded here, so a
caller never builds JSON by string concatenation. Use --int for a value that
must be a number rather than a string.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			obj := map[string]any{}
			for _, kv := range fields {
				k, v, ok := strings.Cut(kv, "=")
				if !ok || k == "" {
					return fmt.Errorf("--field %q is not key=value", kv)
				}
				obj[k] = v
			}
			for _, kv := range intFields {
				k, v, ok := strings.Cut(kv, "=")
				if !ok || k == "" {
					return fmt.Errorf("--int %q is not key=value", kv)
				}
				n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
				if err != nil {
					return fmt.Errorf("--int %q: %v", kv, err)
				}
				obj[k] = n
			}
			// --field-or-null distinguishes "absent" from "empty string". The
			// call site emits diff_fallback as null when the phase-start SHA
			// WAS found and as a range when it was not, and a consumer reads
			// those as different facts. --field would flatten the first into
			// "", which reads as a fallback that happened and produced nothing.
			for _, kv := range nullableFields {
				k, v, ok := strings.Cut(kv, "=")
				if !ok || k == "" {
					return fmt.Errorf("--field-or-null %q is not key=value", kv)
				}
				if v == "" {
					obj[k] = nil
				} else {
					obj[k] = v
				}
			}

			// --json takes a value that IS already JSON — an aggregate built
			// by an earlier step. It is PARSED rather than spliced, so a
			// malformed fragment is refused here instead of producing a
			// document that only looks well-formed.
			for _, kv := range jsonFields {
				k, v, ok := strings.Cut(kv, "=")
				if !ok || k == "" {
					return fmt.Errorf("--json %q is not key=value", kv)
				}
				var parsed any
				if err := json.Unmarshal([]byte(v), &parsed); err != nil {
					return fmt.Errorf("--json %q: value is not valid JSON: %w", k, err)
				}
				obj[k] = parsed
			}
			var b []byte
			var err error
			if indent > 0 {
				b, err = json.MarshalIndent(obj, "", strings.Repeat(" ", indent))
			} else {
				b, err = json.Marshal(obj)
			}
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(b))
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&fields, "field", nil, "key=value, repeatable; value encoded as a string")
	cmd.Flags().StringArrayVar(&intFields, "int", nil, "key=value, repeatable; value encoded as a number")
	cmd.Flags().StringArrayVar(&jsonFields, "json", nil, "key=value, repeatable; value parsed as JSON and embedded")
	cmd.Flags().StringArrayVar(&nullableFields, "field-or-null", nil, "key=value, repeatable; an EMPTY value becomes JSON null rather than \"\"")
	cmd.Flags().IntVar(&indent, "indent", 0, "pretty-print with N spaces of indent")
	return cmd
}

func newUtilJSONGetCmd() *cobra.Command {
	var def string
	var hasDefault bool
	cmd := &cobra.Command{
		Use:   "get <file> <dotted.path>",
		Short: "Print one value from a JSON file",
		Long: `Print the value at a dotted path, e.g. l2_summary.status.

A MISSING PATH IS AN ERROR unless --default is given. Printing an empty line
for "not found" is what makes a caller treat absence as a value, and this
whole migration keeps finding that shape.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := readFileOrStdin(cmd, args[0])
			if err != nil {
				return err
			}
			var doc any
			if err := json.Unmarshal(b, &doc); err != nil {
				return fmt.Errorf("%s is not valid JSON: %w", args[0], err)
			}
			return printLookup(cmd, doc, args[1], args[0], def, hasDefault)
		},
	}
	cmd.Flags().StringVar(&def, "default", "", "value to print when the path is absent")
	cmd.PreRun = func(c *cobra.Command, _ []string) { hasDefault = c.Flags().Changed("default") }
	return cmd
}

// newUtilJSONLenCmd exists so a shell caller can iterate an array without a
// JSON parser of its own.
//
// The prompt bodies this supports loop over a sidecar's directives. Without a
// length they had to be built by an embedded python program; with one, the
// caller runs a plain `for` and renders each item through `util render`, so
// every value still crosses as argv and nothing is concatenated.
func newUtilJSONLenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "len <file> [dotted.path]",
		Short: "Print the number of elements in a JSON array (or keys in an object)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := os.ReadFile(args[0]) //nolint:gosec // operator-supplied path
			if err != nil {
				return err
			}
			var doc any
			if err := json.Unmarshal(b, &doc); err != nil {
				return fmt.Errorf("%s is not valid JSON: %w", args[0], err)
			}
			target := doc
			if len(args) == 2 && strings.TrimSpace(args[1]) != "" {
				v, ok := lookupPath(doc, args[1])
				if !ok {
					return fmt.Errorf("%s: no value at %q", args[0], args[1])
				}
				target = v
			}
			switch t := target.(type) {
			case []any:
				fmt.Fprintln(cmd.OutOrStdout(), len(t))
			case map[string]any:
				fmt.Fprintln(cmd.OutOrStdout(), len(t))
			default:
				return fmt.Errorf("value is %T, not an array or object", target)
			}
			return nil
		},
	}
}

func newUtilJSONValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <file>",
		Short: "Exit 0 if the file parses as JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := os.ReadFile(args[0]) //nolint:gosec // operator-supplied path
			if err != nil {
				return err
			}
			var doc any
			if err := json.Unmarshal(b, &doc); err != nil {
				return fmt.Errorf("%s is not valid JSON: %w", args[0], err)
			}
			return nil
		},
	}
}

// ── yaml ────────────────────────────────────────────────────────────────────

func newUtilYAMLCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "yaml", Short: "Read, validate and canonicalise YAML"}
	cmd.AddCommand(newUtilYAMLGetCmd(), newUtilYAMLValidateCmd(), newUtilYAMLCanonicalCmd())
	return cmd
}

func newUtilYAMLGetCmd() *cobra.Command {
	var def string
	var hasDefault bool
	cmd := &cobra.Command{
		Use:   "get <file> <dotted.path>",
		Short: "Print one value from a YAML file",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := os.ReadFile(args[0]) //nolint:gosec // operator-supplied path
			if err != nil {
				return err
			}
			var doc any
			if err := yaml.Unmarshal(b, &doc); err != nil {
				return fmt.Errorf("%s is not valid YAML: %w", args[0], err)
			}
			return printLookup(cmd, doc, args[1], args[0], def, hasDefault)
		},
	}
	cmd.Flags().StringVar(&def, "default", "", "value to print when the path is absent")
	cmd.PreRun = func(c *cobra.Command, _ []string) { hasDefault = c.Flags().Changed("default") }
	return cmd
}

func newUtilYAMLValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <file>",
		Short: "Exit 0 if the file parses as YAML",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := os.ReadFile(args[0]) //nolint:gosec // operator-supplied path
			if err != nil {
				return err
			}
			var doc any
			if err := yaml.Unmarshal(b, &doc); err != nil {
				return fmt.Errorf("%s is not valid YAML: %w", args[0], err)
			}
			return nil
		},
	}
}

func newUtilYAMLCanonicalCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "canonicalize <file>",
		Short: "Re-emit YAML with keys sorted, for stable hashing and diffing",
		Long: `Print the document with mapping keys sorted.

Callers hash the result to detect a MEANING change rather than a formatting
one, which is why key order has to be normalised first.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := os.ReadFile(args[0]) //nolint:gosec // operator-supplied path
			if err != nil {
				return err
			}
			var doc any
			if err := yaml.Unmarshal(b, &doc); err != nil {
				return fmt.Errorf("%s is not valid YAML: %w", args[0], err)
			}
			// yaml.v3 already emits mapping keys in sorted order, deterministically —
			// measured across repeated runs on a document with unsorted keys at two
			// levels, identical bytes every time. A hand-rolled sort here would be a
			// second implementation of a property the library already guarantees, and
			// the kind that rots silently when the library changes.
			out, err := yaml.Marshal(doc)
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(out)
			return err
		},
	}
}

// ── hash ────────────────────────────────────────────────────────────────────

func newUtilHashCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "hash <file>",
		Short: "Print the SHA-256 of a file, lowercase hex",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := os.ReadFile(args[0]) //nolint:gosec // operator-supplied path
			if err != nil {
				return err
			}
			sum := sha256.Sum256(b)
			fmt.Fprintln(cmd.OutOrStdout(), hex.EncodeToString(sum[:]))
			return nil
		},
	}
}

// ── path ────────────────────────────────────────────────────────────────────

func newUtilPathCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "path", Short: "Path arithmetic"}
	cmd.AddCommand(&cobra.Command{
		Use:   "relative <base> <target>",
		Short: "Print target expressed relative to base",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			rel, err := filepath.Rel(args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), rel)
			return nil
		},
	})
	return cmd
}

// ── shared lookup ───────────────────────────────────────────────────────────

func printLookup(cmd *cobra.Command, doc any, path, file, def string, hasDefault bool) error {
	v, found := lookupPath(doc, path)
	if !found {
		if hasDefault {
			fmt.Fprintln(cmd.OutOrStdout(), def)
			return nil
		}
		return fmt.Errorf("%s: no value at %q (pass --default to treat absence as a value)", file, path)
	}
	fmt.Fprintln(cmd.OutOrStdout(), scalarString(v))
	return nil
}

func lookupPath(doc any, path string) (any, bool) {
	cur := doc
	if strings.TrimSpace(path) == "" {
		return nil, false
	}
	for _, seg := range strings.Split(path, ".") {
		switch t := cur.(type) {
		case map[string]any:
			nv, ok := t[seg]
			if !ok {
				return nil, false
			}
			cur = nv
		case map[any]any:
			nv, ok := t[seg]
			if !ok {
				return nil, false
			}
			cur = nv
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(t) {
				return nil, false
			}
			cur = t[i]
		default:
			return nil, false
		}
	}
	return cur, true
}

// scalarString renders a looked-up value for a shell caller.
//
// A composite value is rendered as JSON rather than as Go's %v, because
// `map[a:1]` is not something a caller can do anything with and would silently
// become a shell string.
func scalarString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
		return fmt.Sprintf("%v", v)
	}
}

// ── schema ──────────────────────────────────────────────────────────────────

// newUtilSchemaCmd validates a YAML or JSON document against a JSON Schema.
//
// REPLACES A FALLBACK AS WELL AS A CALL. The command bodies that did this in
// python carried a second, weaker path for when the `jsonschema` module was
// not installed — "assert all 9 required top-level fields are present". Two
// validators for one contract, and the weaker one ran on exactly the machines
// nobody tested on. The library is a build-time dependency of this binary, so
// there is no absent case left and no second implementation to disagree.
func newUtilSchemaCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "schema-validate <document> <schema>",
		Short: "Validate a YAML or JSON document against a JSON Schema",
		Long: `Validate a document against a JSON Schema (draft 2020-12).

The document may be YAML or JSON; the schema must be JSON. Every violation is
printed, one per line, prefixed with the instance path — a validator that
stops at the first error makes a caller fix one field per run.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			docBytes, err := os.ReadFile(args[0]) //nolint:gosec // operator-supplied path
			if err != nil {
				return err
			}
			var doc any
			if err := yaml.Unmarshal(docBytes, &doc); err != nil {
				return fmt.Errorf("%s is not valid YAML or JSON: %w", args[0], err)
			}
			// Round-trip through JSON so the value the validator sees is the
			// JSON data model, not YAML's — map[any]any and non-string keys
			// are legal YAML and would make the validator report a type error
			// that describes the loader rather than the document.
			normalized, err := json.Marshal(doc)
			if err != nil {
				return fmt.Errorf("%s cannot be represented as JSON: %w", args[0], err)
			}
			var instance any
			if err := json.Unmarshal(normalized, &instance); err != nil {
				return err
			}

			schemaBytes, err := os.ReadFile(args[1]) //nolint:gosec // operator-supplied path
			if err != nil {
				return err
			}
			var schemaDoc any
			if err := json.Unmarshal(schemaBytes, &schemaDoc); err != nil {
				return fmt.Errorf("%s is not valid JSON Schema: %w", args[1], err)
			}
			c := jsonschema.NewCompiler()
			if err := c.AddResource("schema.json", schemaDoc); err != nil {
				return err
			}
			sch, err := c.Compile("schema.json")
			if err != nil {
				return err
			}
			if err := sch.Validate(instance); err != nil {
				// THE LIBRARY'S OWN STRING, FLATTENED — not a walk over
				// BasicOutput().Errors. internal/sidecar/schemavalidate.go
				// records why: walking ErrorKind internals panicked on a nil
				// localiser, and the public rendering is stable. It also
				// renders as a raw Go struct (`&{[title]}`) when printed with
				// %s, which is what this printed before the note was found.
				fmt.Fprintf(cmd.ErrOrStderr(), "SIDECAR INVALID: %s\n", args[0])
				for _, ln := range strings.Split(err.Error(), "\n") {
					ln = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(ln), "- "))
					// The head line names where the SCHEMA lives, which is
					// noise to someone whose document will not validate.
					if ln == "" || strings.HasPrefix(ln, "jsonschema validation failed") {
						continue
					}
					fmt.Fprintf(cmd.ErrOrStderr(), "  %s\n", ln)
				}
				return fmt.Errorf("%s does not conform to %s", args[0], args[1])
			}
			fmt.Fprintln(cmd.OutOrStdout(), "OK")
			return nil
		},
	}
}

// ── render ──────────────────────────────────────────────────────────────────

// newUtilRenderCmd substitutes {{key}} placeholders in a template.
//
// WHY THIS IS A SAFETY VERB, NOT A CONVENIENCE. The prompt bodies it replaces
// interpolate values that come from sidecars, which come from artifact prose —
// attacker-influenceable by the command bodies' own admission. They were built
// with python3 precisely so the values arrived as argv rather than as shell
// string concatenation.
//
// Substitution is LITERAL AND SINGLE-PASS. A replaced value is never rescanned
// for placeholders, so a value containing "{{other}}" cannot pull in a second
// field, and a value containing "$(...)" or backticks is text rather than
// something a shell will later evaluate. Both are the injection this verb
// exists to make impossible rather than merely discouraged.
func newUtilRenderCmd() *cobra.Command {
	var fields []string
	var templateFile string
	cmd := &cobra.Command{
		Use:   "render",
		Short: "Substitute {{key}} placeholders in a template, literally",
		Long: `Read a template and substitute {{key}} with the matching --field value.

Reads the template from --template-file, or from stdin when that is omitted.

Substitution is literal and single-pass: a substituted value is never rescanned,
so a value cannot introduce a placeholder or a shell construct that something
downstream would expand. An UNSUBSTITUTED placeholder is an error — a prompt
that shipped with a literal {{intent}} in it would be a silently degraded
prompt, not a broken one, and nothing would report it.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var tmpl []byte
			var err error
			if templateFile != "" {
				tmpl, err = os.ReadFile(templateFile) //nolint:gosec // operator-supplied path
			} else {
				tmpl, err = io.ReadAll(cmd.InOrStdin())
			}
			if err != nil {
				return err
			}

			vals := map[string]string{}
			for _, kv := range fields {
				k, v, ok := strings.Cut(kv, "=")
				if !ok || k == "" {
					return fmt.Errorf("--field %q is not key=value", kv)
				}
				vals[k] = v
			}

			// One pass over the template, emitting either literal text or a
			// value. Building a replacer and running it repeatedly would
			// rescan substituted text, which is the thing this must not do.
			var out strings.Builder
			src := string(tmpl)
			for {
				i := strings.Index(src, "{{")
				if i < 0 {
					out.WriteString(src)
					break
				}
				j := strings.Index(src[i:], "}}")
				if j < 0 {
					out.WriteString(src)
					break
				}
				key := strings.TrimSpace(src[i+2 : i+j])
				v, ok := vals[key]
				if !ok {
					return fmt.Errorf("template placeholder {{%s}} has no --field value "+
						"(an unsubstituted placeholder would ship as literal text)", key)
				}
				out.WriteString(src[:i])
				out.WriteString(v)
				src = src[i+j+2:]
			}
			_, err = io.WriteString(cmd.OutOrStdout(), out.String())
			return err
		},
	}
	cmd.Flags().StringArrayVar(&fields, "field", nil, "key=value, repeatable; substituted literally for {{key}}")
	cmd.Flags().StringVar(&templateFile, "template-file", "", "template path; stdin when omitted")
	return cmd
}

// ── normalize / match ───────────────────────────────────────────────────────

// newUtilNormalizeCmd applies INV-006's normalisation to one value.
//
// NFKC, then casefold, then strip — the order matters and the invariant names
// it. An externally-controlled value compared against an allowlist WITHOUT
// this is bypassable with one homoglyph: a fullwidth lookalike folds to the
// allowed string only after NFKC, so the raw comparison is unequal and the
// value is treated as out of scope. That bypass was live in shipped code and
// is what INV-006 exists to close.
func newUtilNormalizeCmd() *cobra.Command {
	var noFold bool
	cmd := &cobra.Command{
		Use:   "normalize <value>",
		Short: "NFKC + casefold + strip a value, per INV-006",
		Long: `Print the value normalised for allowlist comparison.

NFKC, then Unicode case folding, then whitespace strip. Emitted WITHOUT a
trailing newline so a caller can compare it directly.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := norm.NFKC.String(args[0])
			if !noFold {
				out = cases.Fold().String(out)
			}
			_, err := io.WriteString(cmd.OutOrStdout(), strings.TrimSpace(out))
			return err
		},
	}
	// --no-fold IS NOT A CONVENIENCE. post-flight.md normalises a PLAN SLUG,
	// and its own comment records that casefolding here "silently disagreed
	// with that contract — fixed by dropping it, not by lowercasing the
	// contract": plan filenames are case-sensitive, so folding the slug makes
	// the lookup miss a file that exists. Normalising without folding is a
	// different operation, not a weaker one, and a caller has to say which it
	// wants.
	cmd.Flags().BoolVar(&noFold, "no-fold", false, "NFKC and strip only; do NOT casefold (for case-sensitive identifiers)")
	return cmd
}

// newUtilMatchCmd tests a value against an anchored regular expression.
//
// EXIT CODE ONLY, no output. The call sites are allowlist guards whose whole
// job is to refuse; printing a match would invite a caller to use the printed
// text instead of the value it already had, which is how a "validated" value
// becomes a different one.
func newUtilMatchCmd() *cobra.Command {
	var pattern string
	cmd := &cobra.Command{
		Use:   "match <value>",
		Short: "Exit 0 if the value matches --pattern, non-zero otherwise",
		Long: `Test a value against a regular expression and report via exit code only.

The pattern is used as written; anchor it yourself. Nothing is printed, because
these are allowlist guards and a caller should keep using the value it already
holds rather than one echoed back by the check.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if pattern == "" {
				return fmt.Errorf("--pattern is required")
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				return fmt.Errorf("--pattern is not a valid regular expression: %w", err)
			}
			if !re.MatchString(args[0]) {
				// SilenceUsage/SilenceErrors: a non-match is the answer, not a
				// usage mistake, and printing a cobra usage block would bury
				// it in noise at every guard site.
				cmd.SilenceUsage = true
				cmd.SilenceErrors = true
				return fmt.Errorf("no match")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&pattern, "pattern", "", "regular expression (anchor it yourself)")
	return cmd
}

// newUtilJSONFormatCmd validates JSON on stdin and re-emits it pretty-printed.
//
// The call site it replaces guards a real hazard: the input is an AGENT's
// output, and writing that to disk unvalidated means a later reader discovers
// the malformation instead of the writer. Parsing before writing moves the
// failure to the step that can still do something about it.
func newUtilJSONFormatCmd() *cobra.Command {
	var indent int
	cmd := &cobra.Command{
		Use:   "format",
		Short: "Validate JSON on stdin and re-emit it pretty-printed",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return err
			}
			var doc any
			if err := json.Unmarshal(raw, &doc); err != nil {
				return fmt.Errorf("input is not valid JSON: %w", err)
			}
			b, err := json.MarshalIndent(doc, "", strings.Repeat(" ", indent))
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(b))
			return nil
		},
	}
	cmd.Flags().IntVar(&indent, "indent", 2, "spaces of indent")
	return cmd
}

// readFileOrStdin reads a path, or stdin when the path is "-".
func readFileOrStdin(cmd *cobra.Command, path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(cmd.InOrStdin())
	}
	return os.ReadFile(path) //nolint:gosec // operator-supplied path
}

// newUtilJSONJoinCmd prints the elements of a JSON array on one line.
//
// AN ABSENT ARRAY IS AN ERROR, an EMPTY one prints an empty line. The call
// site reads a specialist list and branches on whether it is empty; collapsing
// "the key is missing" into "the list is empty" would turn a malformed scope
// document into a silent decision to dispatch nobody.
func newUtilJSONJoinCmd() *cobra.Command {
	var sep string
	cmd := &cobra.Command{
		Use:   "join <file|-> <dotted.path>",
		Short: "Join a JSON array's elements with a separator",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := readFileOrStdin(cmd, args[0])
			if err != nil {
				return err
			}
			var doc any
			if err := json.Unmarshal(b, &doc); err != nil {
				return fmt.Errorf("%s is not valid JSON: %w", args[0], err)
			}
			v, ok := lookupPath(doc, args[1])
			if !ok {
				return fmt.Errorf("%s: no value at %q", args[0], args[1])
			}
			arr, ok := v.([]any)
			if !ok {
				return fmt.Errorf("%s: value at %q is %T, not an array", args[0], args[1], v)
			}
			parts := make([]string, 0, len(arr))
			for _, e := range arr {
				parts = append(parts, scalarString(e))
			}
			fmt.Fprintln(cmd.OutOrStdout(), strings.Join(parts, sep))
			return nil
		},
	}
	cmd.Flags().StringVar(&sep, "sep", ",", "separator")
	return cmd
}

// ── frontmatter ─────────────────────────────────────────────────────────────

// newUtilFrontmatterCmd appends a value to an inline list in YAML frontmatter.
//
// TEXT SURGERY, DELIBERATELY. The file is a markdown document whose body must
// come back byte-identical; parsing the frontmatter and re-emitting it would
// reformat every other key and reorder anything the author cared about. So the
// edit is confined to the one line.
//
// IDEMPOTENT, WHICH THE PREDECESSOR WAS NOT. The python appended
// unconditionally, so running the command twice produced
// `produced_specs: [SPEC-1, SPEC-1]`. A back-reference list with a duplicate
// is not a worse-formatted list, it is a wrong one — it says the same spec was
// produced twice. Corrected here rather than reproduced.
func newUtilFrontmatterCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "frontmatter", Short: "Edit YAML frontmatter keys"}
	cmd.AddCommand(&cobra.Command{
		Use:   "append <file> <key> <value>",
		Short: "Append a value to an inline list in frontmatter, idempotently",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, key, value := args[0], args[1], args[2]
			b, err := os.ReadFile(path) //nolint:gosec // operator-supplied path
			if err != nil {
				return err
			}
			content := string(b)

			listRe := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `:\s*\[([^\]]*)\]`)
			if m := listRe.FindStringSubmatchIndex(content); m != nil {
				inner := strings.TrimSpace(content[m[2]:m[3]])
				for _, existing := range strings.Split(inner, ",") {
					if strings.TrimSpace(existing) == value {
						return nil // already present; no duplicate, no rewrite
					}
				}
				joined := value
				if inner != "" {
					joined = strings.TrimRight(inner, ", ") + ", " + value
				}
				content = content[:m[2]] + joined + content[m[3]:]
			} else {
				// No such key: insert after the opening frontmatter fence. A
				// file with no frontmatter at all is refused rather than
				// silently given one — inventing a fence would change what the
				// document IS.
				if !strings.HasPrefix(content, "---\n") {
					return fmt.Errorf("%s has no YAML frontmatter to append to", path)
				}
				content = strings.Replace(content, "---\n", "---\n"+key+": ["+value+"]\n", 1)
			}
			return os.WriteFile(path, []byte(content), 0o644) //nolint:gosec // markdown doc
		},
	})
	return cmd
}

// newUtilJSONStampCmd reads a JSON file, sets dotted paths, and re-emits it.
//
// FOR STUB AND FIXTURE PATHS. It exists so a canned report can be given the
// identity of the run that is using it — plan, phase, dispatch_mode — without
// a caller hand-editing JSON.
//
// --now IS GENERATED HERE, NOT PASSED IN. A stub report stamped with a
// caller-supplied time is a fixture pretending to be a run; generating it means
// the timestamp is at least honest about when the file was produced.
//
// A path whose PARENT does not exist is an error rather than a silent create.
// The call sites stamp meta.plan on a report that is supposed to HAVE a meta
// block, and quietly growing one would produce a document shaped unlike
// anything the schema describes.
func newUtilJSONStampCmd() *cobra.Command {
	var file string
	var sets, intSets, nows []string
	var indent int
	cmd := &cobra.Command{
		Use:   "stamp <file>",
		Short: "Set dotted paths in a JSON file and print the result",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := os.ReadFile(args[0]) //nolint:gosec // operator-supplied path
			if err != nil {
				return err
			}
			var doc any
			if err := json.Unmarshal(b, &doc); err != nil {
				return fmt.Errorf("%s is not valid JSON: %w", args[0], err)
			}
			apply := func(kv string, conv func(string) (any, error)) error {
				k, v, ok := strings.Cut(kv, "=")
				if !ok || k == "" {
					return fmt.Errorf("%q is not path=value", kv)
				}
				cv, cerr := conv(v)
				if cerr != nil {
					return fmt.Errorf("%q: %w", kv, cerr)
				}
				return setPath(doc, k, cv)
			}
			for _, kv := range sets {
				if err := apply(kv, func(v string) (any, error) { return v, nil }); err != nil {
					return err
				}
			}
			for _, kv := range intSets {
				if err := apply(kv, func(v string) (any, error) {
					return strconv.ParseInt(strings.TrimSpace(v), 10, 64)
				}); err != nil {
					return err
				}
			}
			for _, path := range nows {
				if err := setPath(doc, path, time.Now().UTC().Format("2006-01-02T15:04:05Z")); err != nil {
					return err
				}
			}
			out, err := json.MarshalIndent(doc, "", strings.Repeat(" ", indent))
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&sets, "set", nil, "dotted.path=value, repeatable; value set as a string")
	cmd.Flags().StringArrayVar(&intSets, "set-int", nil, "dotted.path=value, repeatable; value set as a number")
	cmd.Flags().StringArrayVar(&nows, "now", nil, "dotted.path, repeatable; set to the current UTC instant")
	cmd.Flags().IntVar(&indent, "indent", 2, "spaces of indent")
	cmd.Flags().StringVar(&file, "file", "", "unused; the document is the positional argument")
	_ = cmd.Flags().MarkHidden("file")
	return cmd
}

// setPath walks to a dotted path's parent and assigns. The parent must exist:
// see newUtilJSONStampCmd on why a missing one is an error.
func setPath(doc any, path string, value any) error {
	segs := strings.Split(path, ".")
	cur := doc
	for _, seg := range segs[:len(segs)-1] {
		m, ok := cur.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: %q is not an object", path, seg)
		}
		nxt, ok := m[seg]
		if !ok {
			return fmt.Errorf("%s: no %q to set into (a stamp must not invent structure)", path, seg)
		}
		cur = nxt
	}
	m, ok := cur.(map[string]any)
	if !ok {
		return fmt.Errorf("%s: parent is not an object", path)
	}
	m[segs[len(segs)-1]] = value
	return nil
}
