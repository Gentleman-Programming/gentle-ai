package kickoff

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v3/internal/multirole"
	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"
)

func sampleKickoff(change string) Kickoff {
	return Kickoff{
		Schema:   KickoffSchemaV1,
		Change:   change,
		SealedBy: "cli",
		Config: FlowConfig{
			FlowMode:             FlowSDD,
			ExecutionStyle:       ExecutionCheckpointed,
			ExecutionStyleSource: "session_pace",
			HandoffPolicy:        HandoffPerCheckpoint,
			Roles: []KickoffRole{
				{Role: "fullstack", GatePolicy: multirole.PolicyBlocking, TasksFile: "tasks.md", VerifyFile: "verify-report.md"},
			},
		},
		Lifecycle: Lifecycle{DeploymentTarget: "staging", PostArchivePolicy: "bug_only"},
	}
}

func TestSealFirstWriteWinsAndReturnsSealedValue(t *testing.T) {
	changeRoot := t.TempDir()
	k := sampleKickoff("inc-99-example")

	sealed, ok, err := Seal(changeRoot, k)
	if err != nil {
		t.Fatalf("Seal() error inesperado = %v", err)
	}
	if !ok {
		t.Fatal("Seal() ok = false en el primer sellado, se esperaba true")
	}
	if sealed.Change != "inc-99-example" {
		t.Errorf("Change = %q, se esperaba inc-99-example", sealed.Change)
	}
	if sealed.SealedAt.IsZero() {
		t.Error("SealedAt quedo en cero tras el primer sellado")
	}

	onDisk, err := os.ReadFile(filepath.Join(changeRoot, KickoffFileName))
	if err != nil {
		t.Fatalf("kickoff.yaml no se escribio en disco: %v", err)
	}
	if len(onDisk) == 0 {
		t.Fatal("kickoff.yaml se escribio vacio")
	}
}

func TestSealSecondWriteWithDifferentContentDoesNotOverwrite(t *testing.T) {
	changeRoot := t.TempDir()
	first := sampleKickoff("inc-99-example")
	firstSealed, ok, err := Seal(changeRoot, first)
	if err != nil || !ok {
		t.Fatalf("primer Seal() = (%v, %v, %v)", firstSealed, ok, err)
	}

	second := sampleKickoff("inc-99-example")
	second.Config.ExecutionStyle = ExecutionContinuous // contenido distinto
	secondSealed, ok, err := Seal(changeRoot, second)
	if err != nil {
		t.Fatalf("segundo Seal() error inesperado = %v", err)
	}
	if ok {
		t.Fatal("segundo Seal() ok = true, se esperaba false: no debe reescribir")
	}
	if secondSealed.Config.ExecutionStyle != ExecutionCheckpointed {
		t.Errorf("segundo Seal() devolvio %q, se esperaba el ganador ya escrito %q",
			secondSealed.Config.ExecutionStyle, ExecutionCheckpointed)
	}

	onDisk, err := os.ReadFile(filepath.Join(changeRoot, KickoffFileName))
	if err != nil {
		t.Fatalf("releer kickoff.yaml: %v", err)
	}
	reparsed, err := ParseKickoff(onDisk)
	if err != nil {
		t.Fatalf("ParseKickoff() sobre el fichero en disco: %v", err)
	}
	if reparsed.Config.ExecutionStyle != ExecutionCheckpointed {
		t.Errorf("el fichero en disco quedo con %q, se esperaba que conservara %q",
			reparsed.Config.ExecutionStyle, ExecutionCheckpointed)
	}
}

func TestSealPreexistingUnreadableFileReturnsNamedError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows no impone permisos POSIX de lectura sobre ficheros de propietario")
	}
	changeRoot := t.TempDir()
	path := filepath.Join(changeRoot, KickoffFileName)
	if err := os.WriteFile(path, []byte("no importa: solo se necesita que exista"), 0000); err != nil {
		t.Fatalf("preparar fichero illegible: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0644) })

	_, _, err := Seal(changeRoot, sampleKickoff("inc-99-example"))
	if err == nil {
		t.Fatal("Seal() sobre un kickoff.yaml preexistente e ilegible debia devolver error")
	}
}

func TestSealDoesNotLeaveTemporaryFileOnSuccessOrCollision(t *testing.T) {
	changeRoot := t.TempDir()
	if _, _, err := Seal(changeRoot, sampleKickoff("inc-99-example")); err != nil {
		t.Fatalf("primer Seal() error = %v", err)
	}
	if _, _, err := Seal(changeRoot, sampleKickoff("inc-99-example")); err != nil {
		t.Fatalf("segundo Seal() (colision) error = %v", err)
	}

	entries, err := os.ReadDir(changeRoot)
	if err != nil {
		t.Fatalf("leer changeRoot: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != KickoffFileName {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("changeRoot contiene %v, se esperaba solo %q (ningun temporal debe sobrevivir)", names, KickoffFileName)
	}
}

func TestSealPublicationFailureReturnsErrorAndNoIdentity(t *testing.T) {
	changeRoot := t.TempDir()
	original := publishKickoff
	t.Cleanup(func() { publishKickoff = original })
	publishKickoff = func(source, destination string) error {
		return errors.New("fallo de publicacion inyectado")
	}

	_, _, err := Seal(changeRoot, sampleKickoff("inc-99-example"))
	if err == nil {
		t.Fatal("Seal() con fallo de publicacion debia devolver error")
	}
	if _, statErr := os.Stat(filepath.Join(changeRoot, KickoffFileName)); statErr == nil {
		t.Fatal("Seal() con fallo de publicacion dejo kickoff.yaml en disco")
	}
}

func TestSealRealPublisherStillHonoursNoReplace(t *testing.T) {
	// Confirms the production seam (reviewtransaction.PublishFileNoReplace)
	// is wired, not just the overridable var.
	if publishKickoff == nil {
		t.Fatal("publishKickoff seam is nil")
	}
	changeRoot := t.TempDir()
	dest := filepath.Join(changeRoot, "probe.txt")
	src1 := filepath.Join(changeRoot, "src1.txt")
	src2 := filepath.Join(changeRoot, "src2.txt")
	if err := os.WriteFile(src1, []byte("uno"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src2, []byte("dos"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := reviewtransaction.PublishFileNoReplace(src1, dest); err != nil {
		t.Fatalf("primera publicacion: %v", err)
	}
	err := reviewtransaction.PublishFileNoReplace(src2, dest)
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("segunda publicacion = %v, se esperaba os.ErrExist", err)
	}
}

func TestLoadAbsentFileReturnsNilNil(t *testing.T) {
	changeRoot := t.TempDir()
	k, err := Load(changeRoot)
	if err != nil {
		t.Fatalf("Load() sobre changeRoot sin kickoff.yaml devolvio error = %v, se esperaba nil", err)
	}
	if k != nil {
		t.Fatalf("Load() = %+v, se esperaba nil (sin sellar)", k)
	}
}

func TestLoadCorruptFileReturnsError(t *testing.T) {
	changeRoot := t.TempDir()
	path := filepath.Join(changeRoot, KickoffFileName)
	if err := os.WriteFile(path, []byte("schema: [esto, no, es, un, mapeo\n  valido: si"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(changeRoot); err == nil {
		t.Fatal("Load() sobre un kickoff.yaml corrupto debia devolver error")
	}
}

func TestSealInjectableClockLeavesOtherFieldsUntouched(t *testing.T) {
	changeRoot := t.TempDir()
	fixed := time.Date(2026, 9, 21, 15, 25, 0, 0, time.UTC)
	original := nowFunc
	t.Cleanup(func() { nowFunc = original })
	nowFunc = func() time.Time { return fixed }

	sealed, ok, err := Seal(changeRoot, sampleKickoff("inc-99-example"))
	if err != nil || !ok {
		t.Fatalf("Seal() = (ok=%v, err=%v)", ok, err)
	}
	if !sealed.SealedAt.Equal(fixed) {
		t.Errorf("SealedAt = %v, se esperaba el reloj inyectado %v", sealed.SealedAt, fixed)
	}
}
