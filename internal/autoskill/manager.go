package autoskill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// IndexRegenerator refreshes the unified skills index after a successful
// promotion (REQ-22.13). It is a plain func so this package never imports
// skillregistry internals (D-12).
type IndexRegenerator func(cwd, home string) error

// Manager gestiona el ciclo de vida del buzón transitorio de skills y la promoción a producción.
type Manager struct {
	workspaceRoot string
	inboxDir      string
	skillsDir     string
	client        *Client
	detector      *Detector
	miner         *Miner
	// RegenerateIndex refreshes the unified skills index after a successful
	// promotion. A nil regenerator is reported through
	// ApproveOutcome.RegenerateError and never reverts the promotion
	// (D-12).
	RegenerateIndex IndexRegenerator
}

// ApproveOutcome reports a promotion and its derived index refresh. The index
// is a derived view of the filesystem, never a transaction participant
// (D-12): a refresh failure leaves the promotion intact.
type ApproveOutcome struct {
	// Promoted is true when the skill now lives in skills/.
	Promoted bool
	// RegenerateError is non-nil when the index refresh failed after a
	// completed promotion. The promotion stands (REQ-22.13).
	RegenerateError error
}

// NewManager inicializa el gestor de gobernanza de skills.
func NewManager(workspaceRoot string, client *Client, detector *Detector, miner *Miner) *Manager {
	if workspaceRoot == "" {
		workspaceRoot = "."
	}
	if client == nil {
		client = NewClient(ClientOptions{
			CacheDir: filepath.Join(workspaceRoot, ".axiom", "cache", "autoskills"),
		})
	}
	if detector == nil {
		detector = NewDetector(SKILLS_MAP)
	}
	if miner == nil {
		miner = NewMiner()
	}

	return &Manager{
		workspaceRoot: workspaceRoot,
		inboxDir:      filepath.Join(workspaceRoot, ".axiom", "skills", "inbox"),
		skillsDir:     filepath.Join(workspaceRoot, "skills"),
		client:        client,
		detector:      detector,
		miner:         miner,
	}
}

// Scan ejecuta la detección de tecnologías y minería heurística, depositando candidatos en el buzón.
func (m *Manager) Scan(ctx context.Context, targetRole string, offline bool) (*ScanReport, error) {
	report := &ScanReport{
		DetectedTechnologies: make([]string, 0),
		SkillsProposed:       make([]SkillProposal, 0),
	}

	// 1. Asegurar directorios de buzón y caché
	if err := os.MkdirAll(m.inboxDir, 0755); err != nil {
		return nil, fmt.Errorf("error creando directorio de buzón %s: %w", m.inboxDir, err)
	}

	// 2. Detección de Tecnologías
	detected, err := m.detector.Detect(m.workspaceRoot, targetRole)
	if err != nil {
		return nil, fmt.Errorf("error detectando tecnologías: %w", err)
	}

	techNames := make(map[string]bool)
	for _, d := range detected {
		techNames[d.Tech.Name] = true
	}
	for t := range techNames {
		report.DetectedTechnologies = append(report.DetectedTechnologies, t)
	}
	sort.Strings(report.DetectedTechnologies)

	// 3. Obtener Manifiesto del Registro de midudev (si no falla)
	m.client.offline = offline
	regIndex, regErr := m.client.FetchIndex(ctx)

	// 4. Procesar Skills de Registro
	if regErr == nil && regIndex != nil {
		for _, d := range detected {
			for _, skillRef := range d.Tech.Skills {
				skillName := extractCanonicalSkillName(skillRef)

				// Omitir si ya está activa en skills/
				if m.isSkillInstalled(skillName) {
					continue
				}
				// Omitir si ya está en el buzón
				if m.isSkillInInbox(skillName) {
					continue
				}

				entry, found := findRegistryEntry(regIndex, skillRef, skillName)
				if !found {
					continue
				}

				filesData, fetchErr := m.client.FetchAndVerifySkill(ctx, skillName, entry)
				if fetchErr != nil {
					continue // No depositar si falla la integridad criptográfica
				}

				skillMD := string(filesData["SKILL.md"])
				if skillMD == "" {
					continue
				}

				// Crear carpeta en inbox
				skillInboxPath := filepath.Join(m.inboxDir, skillName)
				if err := os.MkdirAll(skillInboxPath, 0755); err != nil {
					continue
				}

				// Escribir archivos descargados
				for fileRel, data := range filesData {
					destPath := filepath.Join(skillInboxPath, fileRel)
					_ = os.MkdirAll(filepath.Dir(destPath), 0755)
					_ = os.WriteFile(destPath, data, 0644)
				}

				// Escribir metadata.json
				meta := ProposalMetadata{
					Name:          skillName,
					Origin:        OriginMidudev,
					Source:        entry.Source,
					Files:         entry.Files,
					SHA256:        entry.SHA256,
					Verified:      true,
					Role:          d.Role,
					DetectedBy:    fmt.Sprintf("dependencia/configuración (%s)", d.Tech.Name),
					Justification: fmt.Sprintf("Tecnología %s identificada en repositorio de rol %s", d.Tech.Name, d.Role),
					CreatedAt:     time.Now().UTC(),
				}

				metaBytes, _ := json.MarshalIndent(meta, "", "  ")
				_ = os.WriteFile(filepath.Join(skillInboxPath, "metadata.json"), metaBytes, 0644)

				proposal := SkillProposal{
					Metadata: meta,
					SkillMD:  skillMD,
				}
				report.SkillsProposed = append(report.SkillsProposed, proposal)
				report.RegistrySkillsCount++
			}
		}
	}

	// 5. Minería Heurística de Repositorio Local
	minedList, mErr := m.miner.MinePatterns(m.workspaceRoot)
	if mErr == nil {
		for _, mined := range minedList {
			if m.isSkillInstalled(mined.Name) || m.isSkillInInbox(mined.Name) {
				continue
			}

			skillInboxPath := filepath.Join(m.inboxDir, mined.Name)
			if err := os.MkdirAll(skillInboxPath, 0755); err != nil {
				continue
			}

			_ = os.WriteFile(filepath.Join(skillInboxPath, "SKILL.md"), []byte(mined.SkillMD), 0644)

			meta := ProposalMetadata{
				Name:          mined.Name,
				Origin:        OriginMined,
				Source:        "local-repository-miner",
				Files:         []string{"SKILL.md"},
				SHA256:        map[string]string{"SKILL.md": "mined-local"},
				Verified:      true,
				Role:          "core",
				DetectedBy:    mined.PatternID,
				Justification: mined.Justification,
				CreatedAt:     time.Now().UTC(),
			}

			metaBytes, _ := json.MarshalIndent(meta, "", "  ")
			_ = os.WriteFile(filepath.Join(skillInboxPath, "metadata.json"), metaBytes, 0644)

			proposal := SkillProposal{
				Metadata: meta,
				SkillMD:  mined.SkillMD,
			}
			report.SkillsProposed = append(report.SkillsProposed, proposal)
			report.MinedSkillsCount++
		}
	}

	// 6. Contar total actual en inbox
	allInbox, _ := m.ListInbox()
	report.TotalInInbox = len(allInbox)

	return report, nil
}

// ListInbox retorna la lista completa de propuestas pendientes de aprobación en el buzón.
func (m *Manager) ListInbox() ([]SkillProposal, error) {
	var list []SkillProposal

	if _, err := os.Stat(m.inboxDir); os.IsNotExist(err) {
		return list, nil
	}

	entries, err := os.ReadDir(m.inboxDir)
	if err != nil {
		return nil, fmt.Errorf("error leyendo buzón de skills: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		skillName := e.Name()
		folder := filepath.Join(m.inboxDir, skillName)

		metaPath := filepath.Join(folder, "metadata.json")
		skillMDPath := filepath.Join(folder, "SKILL.md")

		var meta ProposalMetadata
		metaBytes, err := os.ReadFile(metaPath)
		if err == nil {
			_ = json.Unmarshal(metaBytes, &meta)
		} else {
			meta = ProposalMetadata{
				Name:      skillName,
				Origin:    OriginMidudev,
				Verified:  false,
				CreatedAt: time.Now().UTC(),
			}
		}

		skillMD := ""
		if mdBytes, err := os.ReadFile(skillMDPath); err == nil {
			skillMD = string(mdBytes)
		}

		list = append(list, SkillProposal{
			Metadata: meta,
			SkillMD:  skillMD,
		})
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].Metadata.Name < list[j].Metadata.Name
	})

	return list, nil
}

// Approve promueve atómicamente la skill del buzón transitorio al directorio canónico skills/
// y después regenera el índice unificado de skills (REQ-22.13). El error de
// retorno corresponde solo a fallos de promoción: un fallo de indexación queda
// en ApproveOutcome.RegenerateError, sin deshacer la promoción (D-12).
func (m *Manager) Approve(skillName string) (ApproveOutcome, error) {
	skillName = strings.TrimSpace(skillName)
	if skillName == "" {
		return ApproveOutcome{}, errors.New("nombre de skill no puede estar vacío")
	}

	sourceDir := filepath.Join(m.inboxDir, skillName)
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return ApproveOutcome{}, fmt.Errorf("la propuesta '%s' no existe en el buzón transitorio", skillName)
	}

	destDir := filepath.Join(m.skillsDir, skillName)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return ApproveOutcome{}, fmt.Errorf("error creando directorio destino %s: %w", destDir, err)
	}

	// Copiar archivos (excepto metadata.json)
	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if info.Name() == "metadata.json" {
			return nil
		}

		rel, _ := filepath.Rel(sourceDir, path)
		targetPath := filepath.Join(destDir, rel)
		_ = os.MkdirAll(filepath.Dir(targetPath), 0755)

		return copyFile(path, targetPath)
	})

	if err != nil {
		return ApproveOutcome{}, fmt.Errorf("error promocionando skill a %s: %w", destDir, err)
	}

	// Eliminar de inbox tras promoción exitosa
	_ = os.RemoveAll(sourceDir)

	outcome := ApproveOutcome{Promoted: true}
	outcome.RegenerateError = m.regenerateIndex()
	return outcome, nil
}

// regenerateIndex runs the injected regenerator after a completed promotion.
// A missing regenerator is itself a reported failure: the index is stale and
// the caller must not be told otherwise (D-12).
func (m *Manager) regenerateIndex() error {
	if m.RegenerateIndex == nil {
		return errors.New("index regenerator not configured")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return m.RegenerateIndex(m.workspaceRoot, home)
}

// Reject purga de forma permanente una propuesta del buzón transitorio.
func (m *Manager) Reject(skillName string) error {
	skillName = strings.TrimSpace(skillName)
	if skillName == "" {
		return errors.New("nombre de skill no puede estar vacío")
	}

	targetDir := filepath.Join(m.inboxDir, skillName)
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		return fmt.Errorf("la propuesta '%s' no existe en el buzón transitorio", skillName)
	}

	if err := os.RemoveAll(targetDir); err != nil {
		return fmt.Errorf("error eliminando propuesta %s del buzón: %w", skillName, err)
	}

	return nil
}

func (m *Manager) isSkillInstalled(name string) bool {
	p := filepath.Join(m.skillsDir, name, "SKILL.md")
	_, err := os.Stat(p)
	return err == nil
}

func (m *Manager) isSkillInInbox(name string) bool {
	p := filepath.Join(m.inboxDir, name)
	_, err := os.Stat(p)
	return err == nil
}

func extractCanonicalSkillName(ref string) string {
	parts := strings.Split(ref, "/")
	return parts[len(parts)-1]
}

func findRegistryEntry(idx *RegistryIndex, fullRef, name string) (RegistrySkillEntry, bool) {
	if entry, ok := idx.Skills[name]; ok {
		return entry, true
	}
	if entry, ok := idx.Skills[fullRef]; ok {
		return entry, true
	}
	for k, entry := range idx.Skills {
		if strings.HasSuffix(k, "/"+name) || entry.SkillPath == fullRef || strings.HasSuffix(entry.SkillPath, "/"+name) {
			return entry, true
		}
	}
	return RegistrySkillEntry{}, false
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
