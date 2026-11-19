package reviewtransaction

import (
	"bytes"
	"context"
	"go/scanner"
	"go/token"
	"path"
	"sort"
	"strings"
	"unicode/utf8"
)

// WordingOnlyPaths proves whether a candidate changes only comments in regular
// .go files and returns the sorted paths that proof covers. It is a refinement
// of a medium assessment, never a tier of its own: START keeps classifying the
// same candidate as medium and selects its lens for it, while `review assess`
// publishes the narrower consequence.
//
// Anything this prover cannot place stays medium. A migrated passive
// documentation path is allowed to ride along (its content is already proven
// inert elsewhere); any other path, any mode change, any addition or deletion,
// any binary blob, any read over budget and any token-stream difference returns
// nil, and nil always means "keep the review panel".
//
// Two kinds of change are excluded on purpose, because a lexical classifier
// cannot tell them from behavior. A Go string literal can be logic (a command in
// exec.Command, a format verb, a filesystem path, a SQL statement, a flag name,
// a map key), so a changed literal keeps the medium tier. A shell source already
// carries the shell_source high signal and a shell comment is one `#` away from
// a command, so wording-only never lowers it: `.sh` stays exactly where it is.
func (builder SnapshotBuilder) WordingOnlyPaths(ctx context.Context, snapshot Snapshot) ([]string, error) {
	stats, err := builder.DiffStats(ctx, snapshot)
	if err != nil {
		return nil, err
	}
	activeContent, err := builder.activePassiveContentPaths(ctx, snapshot, stats)
	if err != nil {
		return nil, err
	}
	candidates := make([]DiffStat, 0, len(stats))
	for _, stat := range stats {
		if isGeneratedGoldenPath(stat.Path) {
			continue
		}
		if _, contradicted := activeContent[stat.Path]; isPassiveContentCandidateStat(stat) && !contradicted {
			continue
		}
		if !isWordingOnlyGoCandidateStat(stat) {
			return nil, nil
		}
		candidates = append(candidates, stat)
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	paths := make([]string, 0, len(candidates))
	for _, stat := range candidates {
		paths = append(paths, stat.Path)
	}
	repo, err := builder.repositoryRoot(ctx)
	if err != nil {
		return nil, err
	}
	scanned := int64(0)
	treeBlobs := make(map[string]map[string]treeBlob, 2)
	for _, tree := range []string{snapshot.BaseTree, snapshot.CandidateTree} {
		blobs, err := treeBlobSizes(ctx, repo, tree, paths)
		if err != nil {
			return nil, err
		}
		byPath := make(map[string]treeBlob, len(blobs))
		for _, blob := range blobs {
			scanned += blob.size
			byPath[blob.path] = blob
		}
		treeBlobs[tree] = byPath
	}
	if scanned > processBoundaryScanByteLimit {
		return nil, nil
	}
	oids := make([]string, 0, len(candidates)*2)
	batchOutputLimit := int64(0)
	for _, stat := range candidates {
		for _, tree := range []string{snapshot.BaseTree, snapshot.CandidateTree} {
			blob, present := treeBlobs[tree][stat.Path]
			if !present {
				return nil, nil
			}
			recordBytes := catFileBatchRecordBytes(blob.oid, blob.size)
			if recordBytes >= processBoundaryScanByteLimit-batchOutputLimit {
				return nil, nil
			}
			batchOutputLimit += recordBytes
			oids = append(oids, blob.oid)
		}
	}
	contents, err := batchBlobContents(ctx, repo, oids, int(batchOutputLimit))
	if err != nil {
		return nil, err
	}
	proven := make([]string, 0, len(candidates))
	for _, stat := range candidates {
		baseBlob := treeBlobs[snapshot.BaseTree][stat.Path]
		candidateBlob := treeBlobs[snapshot.CandidateTree][stat.Path]
		if !goCommentOnlyChange(contents[baseBlob.oid], contents[candidateBlob.oid]) {
			return nil, nil
		}
		proven = append(proven, stat.Path)
	}
	sort.Strings(proven)
	return proven, nil
}

// isWordingOnlyGoCandidateStat nominates a .go edit the prover can even
// attempt: an ordinary content edit to a regular, non-executable file whose git
// mode is literally unchanged. An added or deleted file has no pair to compare,
// and a configuration path is consumed as instructions rather than read as
// prose.
func isWordingOnlyGoCandidateStat(stat DiffStat) bool {
	if strings.ToLower(path.Ext(stat.Path)) != ".go" {
		return false
	}
	if stat.Binary || stat.ModeOnly {
		return false
	}
	if stat.OldMode != "100644" || stat.NewMode != "100644" {
		return false
	}
	if stat.Additions+stat.Deletions <= 0 {
		return false
	}
	return !isConfigurationReviewPath(stat.Path)
}

// goTokenText is one token of the behavior-bearing stream: the kind and its
// literal. Positions are deliberately dropped, since a comment rewrite shifts
// every following line and column without changing any token.
type goTokenText struct {
	tok token.Token
	lit string
}

// goDirectiveCommentPrefixes name the comments the toolchain reads as code
// (build constraints, //line, //go:embed and friends, cgo //export //extern,
// and go test's Output comments). They are compared verbatim and in order, so
// adding, removing, or rewording one is never proof of a wording-only change.
var goDirectiveCommentPrefixes = []string{
	"//go:", "// +build", "//line ", "/*line ", "//export ", "//extern ",
}

// isGoDirectiveComment also matches example output comments the way go test
// does: case-insensitive, after any leading space, in either comment form.
func isGoDirectiveComment(comment string) bool {
	for _, prefix := range goDirectiveCommentPrefixes {
		if strings.HasPrefix(comment, prefix) {
			return true
		}
	}
	return isGoExampleOutputComment(comment)
}

// isGoExampleOutputComment matches the comment that opens an example's expected
// output, with go test's case-insensitive, space-tolerant reading.
func isGoExampleOutputComment(comment string) bool {
	text := strings.TrimPrefix(strings.TrimPrefix(comment, "//"), "/*")
	text = strings.ToLower(strings.TrimSpace(text))
	return strings.HasPrefix(text, "output:") || strings.HasPrefix(text, "unordered output:")
}

// goCommentOnlyChange reports whether candidate is base with comments only
// changed: the same non-comment token stream, the same directive comments, and
// a clean scan of both sides. Anything it cannot decide returns false, keeping
// the caller on the review panel.
func goCommentOnlyChange(base, candidate []byte) bool {
	baseTokens, baseDirectives, ok := scanGoForCommentOnlyProof(base)
	if !ok {
		return false
	}
	candidateTokens, candidateDirectives, ok := scanGoForCommentOnlyProof(candidate)
	if !ok {
		return false
	}
	if len(baseTokens) != len(candidateTokens) {
		return false
	}
	for index := range baseTokens {
		if baseTokens[index] != candidateTokens[index] {
			return false
		}
	}
	if len(baseDirectives) != len(candidateDirectives) {
		return false
	}
	for index := range baseDirectives {
		if baseDirectives[index] != candidateDirectives[index] {
			return false
		}
	}
	return true
}

// scanGoForCommentOnlyProof tokenizes one blob with comments enabled and splits
// it into the non-comment stream and the ordered directive comments. Any scan
// error, NUL byte or invalid UTF-8 disqualifies the blob: a file the tokenizer
// cannot read whole is a file this prover cannot vouch for. A file importing
// "C" is excluded too, because its preamble comments are the C code it links.
func scanGoForCommentOnlyProof(content []byte) ([]goTokenText, []string, bool) {
	if bytes.IndexByte(content, 0) >= 0 || !utf8.Valid(content) {
		return nil, nil, false
	}
	fileSet := token.NewFileSet()
	file := fileSet.AddFile("", fileSet.Base(), len(content))
	failed := false
	var source scanner.Scanner
	source.Init(file, content, func(token.Position, string) { failed = true }, scanner.ScanComments)
	tokens := make([]goTokenText, 0, len(content)/8)
	directives := make([]string, 0, 1)
	// The line comments after an example's Output comment are its expected
	// output, so a file with one has every comment compared verbatim.
	comments := make([]string, 0, 8)
	hasExampleOutput := false
	for {
		_, kind, literal := source.Scan()
		if kind == token.EOF {
			if hasExampleOutput {
				directives = comments
			}
			return tokens, directives, !failed
		}
		if kind == token.COMMENT {
			comments = append(comments, literal)
			hasExampleOutput = hasExampleOutput || isGoExampleOutputComment(literal)
			if isGoDirectiveComment(literal) {
				directives = append(directives, literal)
			}
			continue
		}
		if kind == token.STRING && literal == `"C"` {
			return nil, nil, false
		}
		tokens = append(tokens, goTokenText{tok: kind, lit: literal})
	}
}
