package dashboard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// BrowseDirectories explora y lista los subdirectorios locales de una ruta dada,
// proporcionando también metadatos de navegación (unidades, directorio de usuario, padre).
func (s *Service) BrowseDirectories(targetPath string) (*FileSystemBrowseResult, error) {
	drives := getLogicalDrives()
	homeDir, _ := os.UserHomeDir()
	workspaceDir := s.getRootPath()
	sep := string(filepath.Separator)

	resolved := strings.TrimSpace(targetPath)
	if resolved == "" {
		if workspaceDir != "" && isDir(workspaceDir) {
			resolved = workspaceDir
		} else if homeDir != "" && isDir(homeDir) {
			resolved = homeDir
		} else if len(drives) > 0 {
			resolved = drives[0]
		} else {
			cwd, err := os.Getwd()
			if err != nil {
				resolved = sep
			} else {
				resolved = cwd
			}
		}
	}

	cleanPath := filepath.Clean(resolved)
	// Manejo de unidades Windows tipo "C:" -> "C:\"
	if len(cleanPath) == 2 && cleanPath[1] == ':' {
		cleanPath = cleanPath + "\\"
	}

	absPath, err := filepath.Abs(cleanPath)
	if err == nil {
		cleanPath = absPath
	}

	info, err := os.Stat(cleanPath)
	if err != nil {
		// Si la ruta especificada no existe, intentamos con el padre existente más cercano
		parentDir := cleanPath
		for {
			nextParent := filepath.Dir(parentDir)
			if nextParent == parentDir {
				break
			}
			parentDir = nextParent
			if pInfo, pErr := os.Stat(parentDir); pErr == nil && pInfo.IsDir() {
				cleanPath = parentDir
				info = pInfo
				err = nil
				break
			}
		}
	}

	if err != nil || !info.IsDir() {
		return nil, errors.New("la ruta especificada no existe o no es un directorio accesible")
	}

	// Calcular directorio padre
	parentPath := filepath.Dir(cleanPath)
	if parentPath == cleanPath {
		// Llegó a la raíz (ej. "C:\" o "/")
		parentPath = ""
	}

	entries, err := os.ReadDir(cleanPath)
	if err != nil {
		return nil, err
	}

	directories := make([]FileSystemDirectoryItem, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		fullPath := filepath.Join(cleanPath, name)
		hidden := strings.HasPrefix(name, ".")
		directories = append(directories, FileSystemDirectoryItem{
			Name:   name,
			Path:   fullPath,
			Hidden: hidden,
		})
	}

	// Ordenar alfabéticamente ignorando mayúsculas/minúsculas
	sort.Slice(directories, func(i, j int) bool {
		return strings.ToLower(directories[i].Name) < strings.ToLower(directories[j].Name)
	})

	return &FileSystemBrowseResult{
		CurrentPath:  cleanPath,
		ParentPath:   parentPath,
		Drives:       drives,
		HomeDir:      homeDir,
		WorkspaceDir: workspaceDir,
		Separator:    sep,
		Directories:  directories,
	}, nil
}

// PickFolderNative invoca el selector de carpetas nativo del sistema operativo si está soportado.
func (s *Service) PickFolderNative(ctx context.Context, initialPath string) (string, error) {
	return pickFolderNative(ctx, initialPath)
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
