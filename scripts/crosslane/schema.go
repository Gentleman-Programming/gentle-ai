package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const schemaLane = "schema"

// runSchemaLane validates every envelope the battery captured against the
// published schemas in contracts/review-integration/. An envelope whose
// declared schema identity has no published schema file, or whose bytes fail
// its published schema, is an emitter/schema divergence and fails the lane.
func (b *battery) runSchemaLane() {
	compiled, index, err := b.compilePublishedSchemas()
	if err != nil {
		b.fail(schemaLane, "compile published schemas", err.Error())
		return
	}

	type outcome struct {
		total    int
		failures []string
	}
	outcomes := map[string]*outcome{}
	for _, envelope := range b.envelopes {
		state := outcomes[envelope.Schema]
		if state == nil {
			state = &outcome{}
			outcomes[envelope.Schema] = state
		}
		state.total++
		schema, published := compiled[envelope.Schema]
		if !published {
			if len(state.failures) == 0 {
				state.failures = append(state.failures, "no published schema in contracts/ covers this emitted envelope")
			}
			continue
		}
		document, err := jsonschema.UnmarshalJSON(strings.NewReader(string(envelope.Body)))
		if err != nil {
			state.failures = append(state.failures, fmt.Sprintf("%s: %v", envelope.Source, err))
			continue
		}
		if err := schema.Validate(document); err != nil {
			detail := err.Error()
			if validation, ok := err.(*jsonschema.ValidationError); ok {
				detail = flattenValidationError(validation)
			}
			state.failures = append(state.failures, fmt.Sprintf("%s: %s", envelope.Source, detail))
		}
	}

	identities := make([]string, 0, len(outcomes))
	for identity := range outcomes {
		identities = append(identities, identity)
	}
	sort.Strings(identities)
	for _, identity := range identities {
		state := outcomes[identity]
		name := "conformance: " + identity
		if len(state.failures) == 0 {
			b.pass(schemaLane, name, fmt.Sprintf("%d captured envelope(s) match the published schema", state.total))
			continue
		}
		unique := dedupe(state.failures)
		note := fmt.Sprintf("%d/%d envelope(s) diverge: %s", len(state.failures), state.total, strings.Join(unique, " | "))
		b.fail(schemaLane, name, note)
	}
	_ = index
}

// compilePublishedSchemas compiles every schema under
// contracts/review-integration/{v1,v2}/schemas and indexes them by the
// envelope identity each one validates (properties.schema.const).
func (b *battery) compilePublishedSchemas() (map[string]*jsonschema.Schema, []string, error) {
	compiler := jsonschema.NewCompiler()
	compiler.UseRegexpEngine(crosslaneRegexpEngine)
	type pending struct {
		identity string
		id       string
	}
	var pendings []pending
	for _, version := range []string{"v1", "v2"} {
		paths, err := filepath.Glob(filepath.Join(b.repoRoot, "contracts", "review-integration", version, "schemas", "*.schema.json"))
		if err != nil {
			return nil, nil, err
		}
		for _, path := range paths {
			payload, err := os.ReadFile(path)
			if err != nil {
				return nil, nil, err
			}
			document, err := jsonschema.UnmarshalJSON(strings.NewReader(string(payload)))
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", path, err)
			}
			body, ok := document.(map[string]any)
			if !ok {
				return nil, nil, fmt.Errorf("%s: schema document is not an object", path)
			}
			id, _ := body["$id"].(string)
			if id == "" {
				return nil, nil, fmt.Errorf("%s: schema document has no $id", path)
			}
			if err := compiler.AddResource(id, document); err != nil {
				return nil, nil, fmt.Errorf("%s: %w", path, err)
			}
			identity := getString(map[string]any{"root": body}, "root", "properties", "schema", "const")
			if identity != "" {
				pendings = append(pendings, pending{identity: identity, id: id})
			}
		}
	}
	compiled := map[string]*jsonschema.Schema{}
	var identities []string
	for _, p := range pendings {
		schema, err := compiler.Compile(p.id)
		if err != nil {
			return nil, nil, fmt.Errorf("compile %s: %w", p.id, err)
		}
		// Later contract versions win when two files share an identity:
		// the walk visits v1 before v2, so the last assignment is newest.
		compiled[p.identity] = schema
		identities = append(identities, p.identity)
	}
	return compiled, identities, nil
}

func flattenValidationError(err *jsonschema.ValidationError) string {
	leaf := err
	for len(leaf.Causes) > 0 {
		leaf = leaf.Causes[0]
	}
	location := "/" + strings.Join(leaf.InstanceLocation, "/")
	detail := firstLine(leaf.Error())
	if index := strings.Index(detail, ": "); index >= 0 {
		detail = detail[index+2:]
	}
	return fmt.Sprintf("%s: %s", location, detail)
}

func dedupe(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	if len(out) > 3 {
		out = append(out[:3], fmt.Sprintf("(+%d more)", len(out)-3))
	}
	return out
}

// crosslaneRegexpEngine mirrors the repository's schema test engine: Go
// regexp cannot compile the one negative-lookahead path pattern used by the
// published schemas, so that exact pattern gets a semantic matcher.
func crosslaneRegexpEngine(pattern string) (jsonschema.Regexp, error) {
	if pattern == `^(?!/)(?!.*(?:^|/)\.\.(?:/|$)).+$` {
		return crosslaneRegexp{pattern: pattern}, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return crosslaneRegexp{pattern: pattern, re: re}, nil
}

type crosslaneRegexp struct {
	pattern string
	re      *regexp.Regexp
}

func (r crosslaneRegexp) String() string { return r.pattern }

func (r crosslaneRegexp) MatchString(value string) bool {
	if r.re == nil {
		return value != "" && !strings.HasPrefix(value, "/") && !strings.Contains(value, "\n") && !slices.Contains(strings.Split(value, "/"), "..")
	}
	return r.re.MatchString(value)
}
