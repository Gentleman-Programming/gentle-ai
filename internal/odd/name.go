package odd

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// featureNameRegex exige kebab-case en minúsculas: bloques alfanuméricos
// separados por un único guion, sin extremos vacíos ni separadores de ruta.
var featureNameRegex = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// maxFeatureNameLength acota la longitud del nombre de feature. [D-10]
const maxFeatureNameLength = 64

// reservedWindowsNames es la denylist de nombres de dispositivo reservados
// por el sistema de ficheros de Windows, en minúsculas. [D-10]
var reservedWindowsNames = map[string]struct{}{
	"con": {}, "prn": {}, "aux": {}, "nul": {},
	"com1": {}, "com2": {}, "com3": {}, "com4": {}, "com5": {},
	"com6": {}, "com7": {}, "com8": {}, "com9": {},
	"lpt1": {}, "lpt2": {}, "lpt3": {}, "lpt4": {}, "lpt5": {},
	"lpt6": {}, "lpt7": {}, "lpt8": {}, "lpt9": {},
}

// ValidateFeatureName aplica formato kebab-case, longitud máxima y denylist
// de nombres reservados del sistema de ficheros. Es estrictamente más
// restrictiva que validIncrementNameRegex. [D-10]
func ValidateFeatureName(name string) error {
	if !featureNameRegex.MatchString(name) {
		return fmt.Errorf("el nombre %q debe estar en minúsculas kebab-case: %w", name, ErrInvalidFeatureName)
	}
	if len(name) > maxFeatureNameLength {
		return fmt.Errorf("el nombre %q supera el límite de %d caracteres: %w", name, maxFeatureNameLength, ErrInvalidFeatureName)
	}
	if _, reserved := reservedWindowsNames[name]; reserved {
		return fmt.Errorf("el nombre %q está reservado por el sistema de ficheros de Windows: %w", name, ErrReservedName)
	}
	return nil
}

// DocumentPath devuelve la ruta absoluta del documento y verifica que quede
// contenida bajo root tras el Join (defensa en profundidad). [D-10]
func DocumentPath(root, name string) (string, error) {
	if err := ValidateFeatureName(name); err != nil {
		return "", err
	}

	path := filepath.Join(root, "odd", "tasks", name+".md")

	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", fmt.Errorf("no se pudo calcular la ruta relativa de %q respecto a %q: %w", path, root, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("la ruta %q escapa de la raíz del workspace %q: %w", path, root, ErrPathEscape)
	}

	return path, nil
}
