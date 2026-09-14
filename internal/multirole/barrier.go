package multirole

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	verdictRegex = regexp.MustCompile(`(?m)^verdict:\s*([a-zA-Z_]+)`)
)

// CountTasks analiza el contenido de un archivo de tareas markdown y calcula el total, completadas y pendientes.
func CountTasks(content string) RoleTaskProgress {
	lines := strings.Split(content, "\n")
	var completed, pending int

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [x]") || strings.HasPrefix(trimmed, "- [X]") {
			completed++
		} else if strings.HasPrefix(trimmed, "- [ ]") {
			pending++
		}
	}

	total := completed + pending
	var percent float64
	if total > 0 {
		percent = (float64(completed) / float64(total)) * 100.0
	}

	return RoleTaskProgress{
		Total:     total,
		Completed: completed,
		Pending:   pending,
		Percent:   percent,
	}
}

// ExtractPendingTasks extrae las tareas incompletas de un contenido markdown asociándoles la referencia del cambio de origen.
func ExtractPendingTasks(changeName, role, specRef, changeDirRef, content string) []DeferredTask {
	lines := strings.Split(content, "\n")
	var deferred []DeferredTask

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [ ]") {
			taskText := strings.TrimSpace(trimmed[5:])
			if taskText != "" {
				deferred = append(deferred, DeferredTask{
					Change:       changeName,
					Role:         role,
					TaskText:     taskText,
					SpecRef:      specRef,
					ChangeDirRef: changeDirRef,
				})
			}
		}
	}

	return deferred
}

// FormatDeferredTaskMarkdown genera la representación markdown de una tarea diferida para el incremento acumulativo.
func FormatDeferredTaskMarkdown(dt DeferredTask) string {
	return fmt.Sprintf("- [ ] [Ref: %s] (%s) %s\n  - Origen: %s\n  - Spec: %s\n",
		dt.Change, dt.Role, dt.TaskText, dt.ChangeDirRef, dt.SpecRef)
}

// EvaluateBarrier evalúa la barrera de sincronización en el directorio del cambio para los roles asignados.
func EvaluateBarrier(changeDir, changeName string, roles []RoleAssignment) (*BarrierReport, error) {
	report := &BarrierReport{
		Change:    changeName,
		Satisfied: true,
	}

	for _, r := range roles {
		roleStatus := RoleExecutionStatus{
			Assignment: r,
			Verdict:    "missing",
		}

		// 1. Localizar archivo de tareas
		taskFile := filepath.Join(changeDir, fmt.Sprintf("tasks.%s.md", r.Role))
		if _, err := os.Stat(taskFile); err != nil && len(roles) == 1 {
			fallbackTask := filepath.Join(changeDir, "tasks.md")
			if _, err2 := os.Stat(fallbackTask); err2 == nil {
				taskFile = fallbackTask
			}
		}

		if data, err := os.ReadFile(taskFile); err == nil {
			roleStatus.TasksFound = true
			roleStatus.Tasks = CountTasks(string(data))
			roleStatus.ApplyDone = (roleStatus.Tasks.Pending == 0 && roleStatus.Tasks.Total > 0)
		}

		// 2. Localizar archivo de verificación
		verifyFile := filepath.Join(changeDir, fmt.Sprintf("verify-report.%s.md", r.Role))
		if _, err := os.Stat(verifyFile); err != nil && len(roles) == 1 {
			fallbackVerify := filepath.Join(changeDir, "verify-report.md")
			if _, err2 := os.Stat(fallbackVerify); err2 == nil {
				verifyFile = fallbackVerify
			}
		}

		if data, err := os.ReadFile(verifyFile); err == nil {
			roleStatus.VerifyDone = false
			matches := verdictRegex.FindStringSubmatch(string(data))
			if len(matches) > 1 {
				roleStatus.Verdict = strings.ToLower(matches[1])
				if roleStatus.Verdict == "pass" || roleStatus.Verdict == "pass_with_warnings" {
					roleStatus.VerifyDone = true
				}
			}
		}

		roleStatus.Compliant = (roleStatus.ApplyDone && roleStatus.VerifyDone)

		// 3. Evaluar según la política de compuerta (GatePolicy)
		switch r.GatePolicy {
		case PolicyBlocking:
			if !roleStatus.TasksFound {
				report.Blockers = append(report.Blockers, fmt.Sprintf("rol obligatorio %q: no tiene archivo de tareas (tasks.%s.md)", r.Role, r.Role))
			} else if roleStatus.Tasks.Pending > 0 {
				report.Blockers = append(report.Blockers, fmt.Sprintf("rol obligatorio %q: tiene %d tarea(s) pendiente(s) de un total de %d", r.Role, roleStatus.Tasks.Pending, roleStatus.Tasks.Total))
			}

			if !roleStatus.VerifyDone {
				if roleStatus.Verdict == "missing" {
					report.Blockers = append(report.Blockers, fmt.Sprintf("rol obligatorio %q: no tiene informe de verificación (verify-report.%s.md)", r.Role, r.Role))
				} else {
					report.Blockers = append(report.Blockers, fmt.Sprintf("rol obligatorio %q: verificación no superada (veredicto: %s)", r.Role, roleStatus.Verdict))
				}
			}

		case PolicyDeferred:
			if !roleStatus.TasksFound || roleStatus.Tasks.Pending > 0 {
				report.Warnings = append(report.Warnings, fmt.Sprintf("rol diferido %q: tiene %d tarea(s) pendiente(s) (no bloquea el cierre)", r.Role, roleStatus.Tasks.Pending))
				if roleStatus.TasksFound {
					specRef := fmt.Sprintf("openspec/changes/%s/spec.md", changeName)
					changeRef := fmt.Sprintf("openspec/changes/%s/", changeName)
					taskContent, _ := os.ReadFile(taskFile)
					pendingTasks := ExtractPendingTasks(changeName, r.Role, specRef, changeRef, string(taskContent))
					report.DeferredTasks = append(report.DeferredTasks, pendingTasks...)
				}
			}

		case PolicyOptional:
			if !roleStatus.Compliant {
				report.Warnings = append(report.Warnings, fmt.Sprintf("rol opcional %q: no completado", r.Role))
			}
		}

		report.Roles = append(report.Roles, roleStatus)
	}

	if len(report.Blockers) > 0 {
		report.Satisfied = false
	}

	return report, nil
}

// MigrateDeferredTasks vuelca las tareas diferidas en el archivo acumulativo especificado.
func MigrateDeferredTasks(targetCumulativeFile string, tasks []DeferredTask) error {
	if len(tasks) == 0 {
		return nil
	}

	dir := filepath.Dir(targetCumulativeFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("no se pudo crear el directorio acumulativo %q: %w", dir, err)
	}

	var sb strings.Builder
	// Si el archivo no existe, inicializar encabezado
	if _, err := os.Stat(targetCumulativeFile); os.IsNotExist(err) {
		sb.WriteString("# Tareas Acumuladas de QA y Automatización E2E\n\n")
		sb.WriteString("Este archivo concentra las tareas diferidas no concluidas de los incrementos archivados.\n\n")
		sb.WriteString("## Tareas Pendientes\n\n")
	}

	for _, t := range tasks {
		sb.WriteString(FormatDeferredTaskMarkdown(t))
	}

	f, err := os.OpenFile(targetCumulativeFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("no se pudo abrir el archivo acumulativo %q: %w", targetCumulativeFile, err)
	}
	defer f.Close()

	if _, err := f.WriteString(sb.String()); err != nil {
		return fmt.Errorf("error al escribir tareas diferidas en %q: %w", targetCumulativeFile, err)
	}

	return nil
}
