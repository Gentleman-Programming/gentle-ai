package logline

import (
	"errors"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    []Field
		wantErr string // substring of the error message; empty means no error
	}{
		{
			name: "single pair",
			line: "level=info",
			want: []Field{{Key: "level", Value: "info"}},
		},
		{
			name: "several pairs in order",
			line: "ts=2024-01-02T15:04:05Z level=info msg=hello",
			want: []Field{
				{Key: "ts", Value: "2024-01-02T15:04:05Z"},
				{Key: "level", Value: "info"},
				{Key: "msg", Value: "hello"},
			},
		},
		{
			name: "quoted value keeps inner spaces",
			line: `msg="hello world" level=warn`,
			want: []Field{
				{Key: "msg", Value: "hello world"},
				{Key: "level", Value: "warn"},
			},
		},
		{
			name: "escaped quote survives inside quoted value",
			line: `msg="say \"hi\"" ok=true`,
			want: []Field{
				{Key: "msg", Value: `say "hi"`},
				{Key: "ok", Value: "true"},
			},
		},
		{
			name: "empty value is allowed",
			line: "note= level=info",
			want: []Field{
				{Key: "note", Value: ""},
				{Key: "level", Value: "info"},
			},
		},
		{
			name:    "token without equals is rejected",
			line:    "level=info straggler",
			wantErr: `token without '=' at offset 11`,
		},
		{
			name:    "empty key is rejected",
			line:    "=value",
			wantErr: "empty key at offset 0",
		},
		{
			name:    "invalid key character is rejected",
			line:    "we%rd=1",
			wantErr: "invalid key at offset 0",
		},
		{
			name:    "unterminated quote is rejected",
			line:    `msg="oops`,
			wantErr: `unterminated quoted value at offset 0`,
		},
		{
			name: "blank line yields an empty entry",
			line: "   ",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, err := Parse(tt.line)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Parse(%q) = %+v, want error containing %q", tt.line, entry, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Parse(%q) error = %q, want containing %q", tt.line, err.Error(), tt.wantErr)
				}
				var perr *ParseError
				if !errors.As(err, &perr) || perr.Op != "logline.Parse" {
					t.Fatalf("Parse(%q) error type = %T, want *ParseError with Op logline.Parse", tt.line, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) returned unexpected error: %v", tt.line, err)
			}
			if len(entry.Fields) != len(tt.want) {
				t.Fatalf("Parse(%q) fields = %+v, want %+v", tt.line, entry.Fields, tt.want)
			}
			for i, f := range entry.Fields {
				if f != tt.want[i] {
					t.Fatalf("Parse(%q) field %d = %+v, want %+v", tt.line, i, f, tt.want[i])
				}
			}
			if entry.Raw != tt.line {
				t.Fatalf("Parse(%q) Raw = %q, want the input preserved", tt.line, entry.Raw)
			}
		})
	}
}

func TestGetLastDuplicateWins(t *testing.T) {
	entry, err := Parse("a=1 a=2")
	if err != nil {
		t.Fatalf("Parse returned unexpected error: %v", err)
	}
	got, ok := entry.Get("a")
	if !ok || got != "2" {
		t.Fatalf("Get(a) = %q, %v, want \"2\", true (last occurrence wins)", got, ok)
	}
	if _, ok := entry.Get("missing"); ok {
		t.Fatalf("Get(missing) reported found, want false")
	}
}
