package filter

import (
	"path/filepath"

	"github.com/reviewdog/reviewdog/diff"
	"github.com/reviewdog/reviewdog/proto/rdf"
)

// FilteredDiagnostic represents Diagnostic with filtering info.
type FilteredDiagnostic struct {
	Diagnostic   *rdf.Diagnostic
	ShouldReport bool
	// false if the result is outside diff files.
	InDiffFile bool
	// true if the result is inside a diff hunk.
	// If it's a multiline result, both start and end must be in the same diff
	// hunk.
	InDiffContext bool

	// Source lines text of the diagnostic message's line-range.
	// It contains a whole line even if the diagnostic range have column fields.
	// Optional. Currently available only when it's in diff context.
	SourceLines []string

	OldPath string
	OldLine int
}

// FilterCheck filters check results by diff. It doesn't drop check which
// is not in diff but set FilteredDiagnostic.ShouldReport field false.
func FilterCheck(results []*rdf.Diagnostic, diff []*diff.FileDiff, strip int,
	cwd string, mode Mode) []*FilteredDiagnostic {
	checks := make([]*FilteredDiagnostic, 0, len(results))
	df := NewDiffFilter(diff, strip, cwd, mode)
	for _, result := range results {
		check := &FilteredDiagnostic{Diagnostic: result}
		loc := result.GetLocation()
		loc.Path = NormalizePath(loc.GetPath(), cwd, "")
		startLine := int(loc.GetRange().GetStart().GetLine())
		endLine := int(loc.GetRange().GetEnd().GetLine())
		if endLine == 0 {
			endLine = startLine
		}
		check.InDiffContext = true
		sourceLines := []string{}
		for l := startLine; l <= endLine; l++ {
			shouldReport, difffile, diffline := df.ShouldReport(loc.GetPath(), l)
			check.ShouldReport = check.ShouldReport || shouldReport
			// all lines must be in diff.
			check.InDiffContext = check.InDiffContext && diffline != nil
			if diffline != nil {
				sourceLines = append(sourceLines, diffline.Content)
			}
			if difffile != nil {
				check.InDiffFile = true
				if l == startLine {
					// TODO(haya14busa): Support endline as well especially for GitLab.
					check.OldPath, check.OldLine = getOldPosition(difffile, strip, loc.GetPath(), l)
				}
			}
		}
		if check.InDiffContext {
			check.SourceLines = sourceLines
		}
		checks = append(checks, check)
	}
	return checks
}

// NormalizePath return normalized path with workdir and relative path to
// project.
func NormalizePath(path, workdir, projectRelPath string) string {
	path = filepath.Clean(path)
	if path == "." {
		return ""
	}
	// Convert absolute path to relative path only if the path is in current
	// directory.
	if filepath.IsAbs(path) && workdir != "" && contains(path, workdir) {
		relPath, err := filepath.Rel(workdir, path)
		if err == nil {
			path = relPath
		}
	}
	if !filepath.IsAbs(path) && projectRelPath != "" {
		path = filepath.Join(projectRelPath, path)
	}
	return filepath.ToSlash(path)
}

func getOldPosition(filediff *diff.FileDiff, strip int, newPath string, newLine int) (oldPath string, oldLine int) {
	if filediff == nil {
		return "", 0
	}
	if NormalizeDiffPath(filediff.PathNew, strip) != newPath {
		return "", 0
	}
	oldPath = NormalizeDiffPath(filediff.PathOld, strip)
	delta := 0
	for _, hunk := range filediff.Hunks {
		if newLine < hunk.StartLineNew {
			break
		}
		delta += hunk.LineLengthOld - hunk.LineLengthNew
		for _, line := range hunk.Lines {
			if line.LnumNew == newLine {
				return oldPath, line.LnumOld
			}
		}
	}
	return oldPath, newLine + delta
}
