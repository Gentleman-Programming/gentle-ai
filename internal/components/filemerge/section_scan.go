package filemerge

import "strings"

// Section is one matched gentle-ai marker pair.
type Section struct {
	ID        string // marker id, e.g. "persona" (never a whitelist — whatever the file contains)
	Content   string // raw text between open and close markers, markers excluded
	StartLine int    // 1-based line number of the opening marker
	EndLine   int    // 1-based line number of the closing marker
	CharCount int    // len(Content) in bytes
	LineCount int    // number of content lines (0 when Content == "")
}

// AnomalyKind classifies a structural marker defect.
type AnomalyKind string

const (
	AnomalyOrphanOpen  AnomalyKind = "orphan-open"  // opener with no matching closer (unbounded block)
	AnomalyOrphanClose AnomalyKind = "orphan-close" // closer with no preceding opener
	AnomalyMismatch    AnomalyKind = "mismatch"     // closer id != currently-open id
)

// Anomaly is a structural marker defect surfaced by ScanSections.
type Anomaly struct {
	Kind AnomalyKind
	ID   string // id on the offending marker
	Line int    // 1-based line of the offending marker
}

// ScanResult is the full outcome of one pass over a markdown file.
type ScanResult struct {
	Sections  []Section
	Anomalies []Anomaly
}

// ScanSections finds every <!-- gentle-ai:ID --> ... <!-- /gentle-ai:ID --> pair
// in content, generically (no id whitelist), plus any structural anomalies.
func ScanSections(content string) ScanResult {
	var result ScanResult

	var (
		openActive   bool
		openID       string
		openLine     int
		contentLines []string
	)

	lines := strings.Split(content, "\n")
	for i, raw := range lines {
		lineNum := i + 1
		line := strings.TrimSuffix(raw, "\r")
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(trimmed, closePrefix) && strings.HasSuffix(trimmed, markerSuffix):
			id := strings.TrimSuffix(strings.TrimPrefix(trimmed, closePrefix), markerSuffix)
			switch {
			case !openActive:
				// No opener is active: this closer has nothing to pair with.
				result.Anomalies = append(result.Anomalies, Anomaly{Kind: AnomalyOrphanClose, ID: id, Line: lineNum})
			case id != openID:
				// Closer id doesn't match the active opener: report only the
				// mismatch (not a second AnomalyOrphanOpen for the discarded
				// opener) and drop the open context so scanning resumes clean
				// from the next marker — a single anomaly per malformed event.
				result.Anomalies = append(result.Anomalies, Anomaly{Kind: AnomalyMismatch, ID: id, Line: lineNum})
				openActive = false
				contentLines = nil
			default:
				sectionContent := strings.Join(contentLines, "\n")
				lineCount := 0
				if sectionContent != "" {
					lineCount = strings.Count(sectionContent, "\n") + 1
				}
				result.Sections = append(result.Sections, Section{
					ID:        openID,
					Content:   sectionContent,
					StartLine: openLine,
					EndLine:   lineNum,
					CharCount: len(sectionContent),
					LineCount: lineCount,
				})
				openActive = false
				contentLines = nil
			}

		case strings.HasPrefix(trimmed, markerPrefix) && strings.HasSuffix(trimmed, markerSuffix):
			id := strings.TrimSuffix(strings.TrimPrefix(trimmed, markerPrefix), markerSuffix)
			if openActive {
				result.Anomalies = append(result.Anomalies, Anomaly{Kind: AnomalyOrphanOpen, ID: openID, Line: openLine})
			}
			openActive = true
			openID = id
			openLine = lineNum
			contentLines = nil

		default:
			if openActive {
				contentLines = append(contentLines, line)
			}
		}
	}

	if openActive {
		result.Anomalies = append(result.Anomalies, Anomaly{Kind: AnomalyOrphanOpen, ID: openID, Line: openLine})
	}

	return result
}
