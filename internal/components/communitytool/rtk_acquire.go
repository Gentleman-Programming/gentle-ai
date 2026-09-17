package communitytool

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	rtkMaxArchiveSize           int64 = 16 << 20
	rtkMaxExecutableSize              = 64 << 20
	rtkMaxTarGzDecompressedSize int64 = rtkMaxArchiveSize
)

var errRTKTarGzDecompressedLimit = errors.New("RTK tar.gz exceeds total decompressed limit")

type rtkHTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type rtkArchiveMember struct {
	data io.Reader
	size int64
}

func acquireRTKCandidate(client rtkHTTPClient, stagingDir string, asset rtkCandidateAsset) (string, error) {
	if client == nil || asset.SizeBytes <= 0 || asset.SizeBytes > rtkMaxArchiveSize || asset.ExecutableSizeBytes <= 0 || len(asset.ExecutableSHA256) != sha256.Size*2 || !safeRTKMemberName(asset.ExecutableMember) {
		return "", fmt.Errorf("invalid RTK acquisition contract")
	}
	entries, err := os.ReadDir(stagingDir)
	if err != nil || len(entries) != 0 {
		return "", fmt.Errorf("RTK staging directory must exist and be empty")
	}
	req, err := http.NewRequest(http.MethodGet, asset.URL, nil)
	if err != nil {
		return "", fmt.Errorf("build RTK download request: %w", err)
	}
	response, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download RTK candidate: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("download RTK candidate: HTTP %d", response.StatusCode)
	}
	archive, err := io.ReadAll(io.LimitReader(response.Body, asset.SizeBytes+1))
	if err != nil || int64(len(archive)) != asset.SizeBytes {
		return "", fmt.Errorf("RTK candidate size verification failed")
	}
	sum := sha256.Sum256(archive)
	if hex.EncodeToString(sum[:]) != asset.SHA256 {
		return "", fmt.Errorf("RTK candidate checksum verification failed")
	}
	member, err := rtkArchiveExecutable(archive, asset.Name, asset.ExecutableMember)
	if err != nil {
		return "", err
	}
	return extractRTKExecutable(stagingDir, asset.ExecutableMember, member, asset.ExecutableSizeBytes, asset.ExecutableSHA256)
}

func rtkArchiveExecutable(archive []byte, archiveName, expected string) (rtkArchiveMember, error) {
	if strings.HasSuffix(archiveName, ".tar.gz") {
		return rtkTarGzExecutable(archive, expected)
	}
	return rtkZIPExecutable(archive, expected)
}

func rtkZIPExecutable(archive []byte, expected string) (rtkArchiveMember, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return rtkArchiveMember{}, fmt.Errorf("read RTK candidate ZIP: %w", err)
	}
	if len(reader.File) != 1 {
		return rtkArchiveMember{}, fmt.Errorf("RTK candidate ZIP must contain exactly one member")
	}
	file := reader.File[0]
	if !safeRTKMemberName(file.Name) || file.Name != expected || !file.Mode().IsRegular() || file.UncompressedSize64 > rtkMaxExecutableSize {
		return rtkArchiveMember{}, fmt.Errorf("RTK candidate ZIP member is not the expected regular executable")
	}
	input, err := file.Open()
	if err != nil {
		return rtkArchiveMember{}, fmt.Errorf("open RTK executable: %w", err)
	}
	return rtkArchiveMember{data: input, size: int64(file.UncompressedSize64)}, nil
}

func rtkTarGzExecutable(archive []byte, expected string) (rtkArchiveMember, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return rtkArchiveMember{}, fmt.Errorf("read RTK candidate tar.gz: %w", err)
	}
	bounded := &rtkBoundedReader{Reader: gzipReader, remaining: rtkMaxTarGzDecompressedSize}
	tarReader := tar.NewReader(bounded)
	header, err := tarReader.Next()
	if err != nil {
		_ = gzipReader.Close()
		return rtkArchiveMember{}, fmt.Errorf("read RTK candidate tar.gz: %w", err)
	}
	if !safeRTKMemberName(header.Name) || header.Name != expected || header.Typeflag != tar.TypeReg || header.Size < 0 || header.Size > rtkMaxExecutableSize {
		_ = gzipReader.Close()
		return rtkArchiveMember{}, fmt.Errorf("RTK candidate tar.gz member is not the expected regular executable")
	}
	if _, err := tarReader.Next(); err != io.EOF {
		_ = gzipReader.Close()
		if errors.Is(err, errRTKTarGzDecompressedLimit) {
			return rtkArchiveMember{}, errRTKTarGzDecompressedLimit
		}
		return rtkArchiveMember{}, fmt.Errorf("RTK candidate tar.gz must contain exactly one member")
	}
	// Parse the bounded stream once before extraction so malformed, duplicate, or
	// trailing members are rejected before any output file is created.
	_ = gzipReader.Close()
	gzipReader, err = gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return rtkArchiveMember{}, fmt.Errorf("read RTK candidate tar.gz: %w", err)
	}
	bounded = &rtkBoundedReader{Reader: gzipReader, remaining: rtkMaxTarGzDecompressedSize}
	tarReader = tar.NewReader(bounded)
	if _, err := tarReader.Next(); err != nil {
		_ = gzipReader.Close()
		return rtkArchiveMember{}, fmt.Errorf("read RTK candidate tar.gz: %w", err)
	}
	return rtkArchiveMember{data: &rtkReadCloser{Reader: io.LimitReader(tarReader, header.Size), closer: gzipReader}, size: header.Size}, nil
}

type rtkBoundedReader struct {
	io.Reader
	remaining int64
}

func (r *rtkBoundedReader) Read(buffer []byte) (int, error) {
	if r.remaining <= 0 {
		var extra [1]byte
		n, err := r.Reader.Read(extra[:])
		if n > 0 {
			return 0, errRTKTarGzDecompressedLimit
		}
		return 0, err
	}
	if int64(len(buffer)) > r.remaining {
		buffer = buffer[:r.remaining]
	}
	n, err := r.Reader.Read(buffer)
	r.remaining -= int64(n)
	return n, err
}

type rtkReadCloser struct {
	io.Reader
	closer io.Closer
}

func (r *rtkReadCloser) Close() error { return r.closer.Close() }

func safeRTKMemberName(name string) bool {
	return name != "" && filepath.Base(name) == name && path.Base(name) == name && !filepath.IsAbs(name) && !strings.Contains(name, "\\") && !strings.Contains(name, "/")
}

func extractRTKExecutable(stagingDir, name string, member rtkArchiveMember, expectedSize int64, expectedSHA256 string) (string, error) {
	if closer, ok := member.data.(io.Closer); ok {
		defer closer.Close()
	}
	path := filepath.Join(stagingDir, name)
	output, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		return "", fmt.Errorf("create RTK executable: %w", err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(path)
		}
	}()
	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(output, hasher), io.LimitReader(member.data, member.size+1))
	if err != nil {
		_ = output.Close()
		return "", fmt.Errorf("extract RTK executable: %w", err)
	}
	if written != member.size || written != expectedSize {
		_ = output.Close()
		return "", fmt.Errorf("extract RTK executable: unexpected member size")
	}
	if hex.EncodeToString(hasher.Sum(nil)) != expectedSHA256 {
		_ = output.Close()
		return "", fmt.Errorf("extract RTK executable: checksum verification failed")
	}
	if err := output.Close(); err != nil {
		return "", fmt.Errorf("close RTK executable: %w", err)
	}
	ok = true
	return path, nil
}
