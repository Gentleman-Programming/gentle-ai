// Package logline parses one logfmt-style log line (`ts=... level=info
// msg="hello world"`) into ordered key/value fields, so tools that replay or
// audit machine-readable output can read fields back without re-implementing
// the quoting rules. Grammar: fields are separated by spaces or tabs; key is
// non-empty ASCII letters, digits, `_`, `.` or `-`; value is raw bytes up to
// the next separator or a double-quoted string with backslash escapes. The
// first `=` splits a field, so an unquoted value may contain `=`. Duplicate
// keys are kept in order and Get resolves to the last one. A blank line
// parses to an empty Entry, never an error.
package logline

import (
	"fmt"
	"strings"
)

// Field is one parsed `key=value` pair in input order.
type Field struct {
	Key   string
	Value string
}

// Entry is a parsed log line: the raw input preserved verbatim plus the
// fields in the order they appeared.
type Entry struct {
	Raw    string
	Fields []Field
}

// Get returns the value of the last occurrence of key in the entry. The
// second result is false when the key is absent.
func (e Entry) Get(key string) (string, bool) {
	value := ""
	found := false
	for _, f := range e.Fields {
		if f.Key == key {
			value, found = f.Value, true
		}
	}
	return value, found
}

// ParseError describes why Parse rejected a line. Pos is the byte offset of
// the offending field inside the input.
type ParseError struct {
	Op  string // always "logline.Parse"
	Pos int    // byte offset of the offending field
	Msg string // what was wrong, safe to print
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s: %s at offset %d", e.Op, e.Msg, e.Pos)
}

func parseErrorf(pos int, format string, args ...any) *ParseError {
	return &ParseError{Op: "logline.Parse", Pos: pos, Msg: fmt.Sprintf(format, args...)}
}

// Parse converts one logfmt line into an Entry. Malformed fields — a token
// without `=`, an empty or invalid key, or an unterminated quoted value —
// return a *ParseError naming the byte offset and an empty Entry, so the
// caller sees exactly which field to fix rather than a truncated result.
func Parse(line string) (Entry, error) {
	entry := Entry{Raw: line}
	for i := 0; i < len(line); {
		if isSpace(line[i]) {
			i++
			continue
		}
		field, next, err := parseField(line, i)
		if err != nil {
			return Entry{}, err
		}
		entry.Fields = append(entry.Fields, field)
		i = next
	}
	return entry, nil
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' }

// parseField parses one `key=value` field starting at start and returns the
// field plus the offset just past it. Quoted values are consumed while
// scanning, not split beforehand, so `msg="hello world"` stays one field.
func parseField(line string, start int) (Field, int, error) {
	i := start
	for i < len(line) && !isSpace(line[i]) && line[i] != '=' {
		i++
	}
	if i >= len(line) || isSpace(line[i]) {
		return Field{}, 0, parseErrorf(start, "token without '='")
	}
	key := line[start:i]
	i++ // skip '='

	var value string
	if i < len(line) && line[i] == '"' {
		var err error
		value, i, err = parseQuoted(line, i, start)
		if err != nil {
			return Field{}, 0, err
		}
	} else {
		valueStart := i
		for i < len(line) && !isSpace(line[i]) {
			i++
		}
		value = line[valueStart:i]
	}
	if err := validateKey(key, start); err != nil {
		return Field{}, 0, err
	}
	return Field{Key: key, Value: value}, i, nil
}

// parseQuoted reads a double-quoted value starting at the opening quote and
// returns the unquoted content plus the offset just past the closing quote.
func parseQuoted(line string, open, start int) (string, int, error) {
	var out strings.Builder
	out.Grow(len(line) - open)
	i := open + 1
	for i < len(line) {
		c := line[i]
		if c == '"' {
			return out.String(), i + 1, nil
		}
		if c == '\\' && i+1 < len(line) {
			i++
			switch line[i] {
			case 'n':
				out.WriteByte('\n')
			case 't':
				out.WriteByte('\t')
			default:
				out.WriteByte(line[i])
			}
			i++
			continue
		}
		out.WriteByte(c)
		i++
	}
	return "", 0, parseErrorf(start, "unterminated quoted value")
}

// validateKey rejects an empty key or any byte outside the allowed set.
func validateKey(key string, start int) error {
	if key == "" {
		return parseErrorf(start, "empty key")
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '_' || c == '.' || c == '-':
		default:
			return parseErrorf(start, "invalid key")
		}
	}
	return nil
}
