package reviewerprovider

import "strings"

// reviewerTransportFailureExcerptLimit bounds the stdout excerpt a failing
// child contributes to its transport error.
const reviewerTransportFailureExcerptLimit = 512

// reviewerTransportFailureDetail names why a reviewer child failed. stderr is
// the conventional channel and is returned unchanged when it carries anything.
// A child that explains itself on stdout instead (claude prints `Not logged
// in` there and exits 1) contributes its first line, bounded to
// reviewerTransportFailureExcerptLimit bytes, so the operator never reads an
// exit status followed by nothing (issue #3289). The excerpt is the child's
// own output and nothing else: the provider prompt never flows through here,
// and nothing is parsed, classified, or retried.
func reviewerTransportFailureDetail(stderr, stdout string) string {
	if strings.TrimSpace(stderr) != "" {
		return stderr
	}
	excerpt := strings.TrimSpace(stdout)
	if newline := strings.IndexByte(excerpt, '\n'); newline >= 0 {
		excerpt = excerpt[:newline]
	}
	if len(excerpt) > reviewerTransportFailureExcerptLimit {
		excerpt = strings.ToValidUTF8(excerpt[:reviewerTransportFailureExcerptLimit], "")
	}
	return strings.Join(strings.Fields(excerpt), " ")
}
