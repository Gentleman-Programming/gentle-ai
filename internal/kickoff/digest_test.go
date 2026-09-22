package kickoff

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("escribir %q: %v", path, err)
	}
}

func TestArtifactDigestSameContentDifferentLineEndingsMatch(t *testing.T) {
	dir := t.TempDir()
	lfPath := filepath.Join(dir, "lf.md")
	crlfPath := filepath.Join(dir, "crlf.md")
	writeFile(t, lfPath, "linea uno\nlinea dos\nlinea tres\n")
	writeFile(t, crlfPath, "linea uno\r\nlinea dos\r\nlinea tres\r\n")

	lfDigest, err := ArtifactDigest([]string{lfPath})
	if err != nil {
		t.Fatalf("ArtifactDigest(lf) error = %v", err)
	}
	crlfDigest, err := ArtifactDigest([]string{crlfPath})
	if err != nil {
		t.Fatalf("ArtifactDigest(crlf) error = %v", err)
	}
	if lfDigest != crlfDigest {
		t.Fatalf("digest LF = %q, digest CRLF = %q; el mismo contenido con distinto fin de linea debe producir el mismo digest", lfDigest, crlfDigest)
	}
}

func TestArtifactDigestPathOrderDoesNotAlterResult(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a.md")
	pathB := filepath.Join(dir, "b.md")
	writeFile(t, pathA, "contenido A\n")
	writeFile(t, pathB, "contenido B\n")

	forward, err := ArtifactDigest([]string{pathA, pathB})
	if err != nil {
		t.Fatalf("ArtifactDigest(A,B) error = %v", err)
	}
	backward, err := ArtifactDigest([]string{pathB, pathA})
	if err != nil {
		t.Fatalf("ArtifactDigest(B,A) error = %v", err)
	}
	if forward != backward {
		t.Fatalf("digest(A,B) = %q, digest(B,A) = %q; el orden de entrada de las rutas no debe alterar el resultado", forward, backward)
	}
}

func TestArtifactDigestDifferentContentProducesDifferentDigest(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a.md")
	writeFile(t, pathA, "version uno\n")
	first, err := ArtifactDigest([]string{pathA})
	if err != nil {
		t.Fatalf("ArtifactDigest() error = %v", err)
	}

	writeFile(t, pathA, "version dos, remediada\n")
	second, err := ArtifactDigest([]string{pathA})
	if err != nil {
		t.Fatalf("ArtifactDigest() error = %v", err)
	}

	if first == second {
		t.Fatal("ArtifactDigest() produjo el mismo digest para contenidos distintos")
	}
}

func TestArtifactDigestMissingFileReturnsNamedError(t *testing.T) {
	dir := t.TempDir()
	_, err := ArtifactDigest([]string{filepath.Join(dir, "no-existe.md")})
	if err == nil {
		t.Fatal("ArtifactDigest() con un fichero ausente debia devolver error")
	}
}
