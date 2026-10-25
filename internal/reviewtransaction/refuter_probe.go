package reviewtransaction

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// refuterProbeEntry is one blob of the candidate tree the S11 probe copy holds.
type refuterProbeEntry struct {
	mode string
	oid  string
	path string
}

// MaterializeRefuterProbeTree writes the frozen candidate tree into dir, the
// scratch copy a refuter may run one reproducing command in (S11). It reads
// Git objects only -- never the live workspace, whose bytes may have drifted
// since START, and never .gitattributes export rules, which would rewrite or
// omit files. Regular files keep their executable bit, symlinks are created
// after every file so no write can pass through one, and submodules stay
// empty directories. dir must be a directory the caller owns and removes.
func MaterializeRefuterProbeTree(ctx context.Context, repo, tree, dir string) error {
	listing, err := runGitInventory(ctx, repo, "ls-tree", "-r", "-z", "--full-tree", tree)
	if err != nil {
		return fmt.Errorf("list refuter probe tree: %w", err)
	}
	var blobs, links []refuterProbeEntry
	for _, record := range bytes.Split(listing, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		header, path, found := strings.Cut(string(record), "\t")
		fields := strings.Fields(header)
		if !found || len(fields) != 3 {
			return fmt.Errorf("unexpected refuter probe tree entry %q", record) // refusal:by-design world-action: malformed Git protocol output cannot be made trustworthy by a review command
		}
		entry := refuterProbeEntry{mode: fields[0], oid: fields[2], path: path}
		target, err := refuterProbePath(dir, path)
		if err != nil {
			return err
		}
		switch {
		case fields[1] == "commit":
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case fields[1] != "blob":
			return fmt.Errorf("unexpected refuter probe tree object %q at %q", fields[1], path) // refusal:by-design world-action: malformed Git protocol output cannot be made trustworthy by a review command
		case entry.mode == "120000":
			links = append(links, entry)
		default:
			blobs = append(blobs, entry)
		}
	}
	return writeRefuterProbeBlobs(ctx, repo, dir, append(blobs, links...))
}

// refuterProbePath maps a tree path under dir, refusing any path that could
// leave it.
func refuterProbePath(dir, path string) (string, error) {
	for _, segment := range strings.Split(path, "/") {
		if segment == "" || segment == "." || segment == ".." || strings.EqualFold(segment, ".git") {
			return "", fmt.Errorf("refuter probe tree path %q is unsafe", path) // refusal:by-design world-action: a candidate tree path outside the probe copy cannot be materialized
		}
	}
	return filepath.Join(dir, filepath.FromSlash(path)), nil
}

// writeRefuterProbeBlobs streams every blob through one cat-file batch, in
// order, so a large candidate costs one Git process instead of one per file.
func writeRefuterProbeBlobs(ctx context.Context, repo, dir string, entries []refuterProbeEntry) error {
	if len(entries) == 0 {
		return nil
	}
	var request bytes.Buffer
	for _, entry := range entries {
		request.WriteString(entry.oid + "\n")
	}
	command := gitCommandContext(ctx, "git", "--no-replace-objects", "-C", repo, "cat-file", "--batch")
	command.Env = sanitizedGitEnvironment(os.Environ(), nil)
	command.WaitDelay = gitCommandWaitDelay
	command.Stdin = &request
	var stderr bytes.Buffer
	command.Stderr = &stderr
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return err
	}
	writeErr := readRefuterProbeBatch(bufio.NewReader(stdout), dir, entries)
	if writeErr != nil {
		_ = command.Process.Kill()
	}
	waitErr := command.Wait()
	if writeErr != nil {
		return writeErr
	}
	if waitErr != nil {
		return fmt.Errorf("read refuter probe blobs: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func readRefuterProbeBatch(reader *bufio.Reader, dir string, entries []refuterProbeEntry) error {
	for _, entry := range entries {
		header, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read refuter probe blob %s: %w", entry.oid, err)
		}
		fields := strings.Fields(header)
		if len(fields) != 3 || fields[0] != entry.oid || fields[1] != "blob" {
			return fmt.Errorf("read refuter probe blob %s: unexpected cat-file batch header %q", entry.oid, strings.TrimSpace(header)) // refusal:by-design world-action: malformed Git protocol output cannot be made trustworthy by a review command
		}
		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || size < 0 {
			return fmt.Errorf("read refuter probe blob %s: invalid size %q", entry.oid, fields[2]) // refusal:by-design world-action: malformed Git protocol output cannot be made trustworthy by a review command
		}
		target, err := refuterProbePath(dir, entry.path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if entry.mode == "120000" {
			link, err := io.ReadAll(io.LimitReader(reader, size))
			if err != nil || int64(len(link)) != size {
				return fmt.Errorf("read refuter probe symlink %q: %w", entry.path, errors.Join(err, io.ErrUnexpectedEOF))
			}
			if err := os.Symlink(string(link), target); err != nil {
				return err
			}
		} else if err := writeRefuterProbeFile(reader, target, entry, size); err != nil {
			return err
		}
		if terminator, err := reader.ReadByte(); err != nil || terminator != '\n' {
			return fmt.Errorf("read refuter probe blob %s: missing batch terminator", entry.oid) // refusal:by-design world-action: malformed Git protocol output cannot be made trustworthy by a review command
		}
	}
	return nil
}

func writeRefuterProbeFile(reader io.Reader, target string, entry refuterProbeEntry, size int64) error {
	perm := os.FileMode(0o644)
	if entry.mode == "100755" {
		perm = 0o755
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	written, copyErr := io.CopyN(file, reader, size)
	closeErr := file.Close()
	if copyErr != nil || written != size {
		return fmt.Errorf("write refuter probe file %q: %w", entry.path, errors.Join(copyErr, io.ErrUnexpectedEOF))
	}
	return closeErr
}
