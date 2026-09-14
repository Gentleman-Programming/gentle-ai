package multirole

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/workspace"
	"gopkg.in/yaml.v3"
)

var (
	// regex para detectar bloques de código yaml que declaren roles
	yamlCodeBlockRegex = regexp.MustCompile("(?s)```ya?ml\\s*\\n(.*?)```")
)

type rolesWrapper struct {
	Roles []RoleAssignment `yaml:"roles"`
}

// DetectRoles lee un archivo design.md, extrae los roles asignados, los valida contra axiom.yaml y aplica fallback si es necesario.
func DetectRoles(designPath string, wsConfig *workspace.WorkspaceConfig) ([]RoleAssignment, error) {
	data, err := os.ReadFile(designPath)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer el archivo de diseño %q: %w", designPath, err)
	}

	roles, err := ParseRolesMarkdown(string(data))
	if err != nil {
		return nil, err
	}

	// Fallback si el diseño no declara roles explícitos
	if len(roles) == 0 {
		defaultRole := "core"
		if wsConfig != nil && len(wsConfig.Roles) > 0 {
			if _, exists := wsConfig.Roles["core"]; exists {
				defaultRole = "core"
			} else {
				for k := range wsConfig.Roles {
					defaultRole = k
					break
				}
			}
		}

		roles = []RoleAssignment{
			{
				Role:       defaultRole,
				Name:       defaultRole,
				GatePolicy: PolicyBlocking,
			},
		}
	}

	// Validación contra axiom.yaml
	if wsConfig != nil && len(wsConfig.Roles) > 0 {
		for _, r := range roles {
			if !roleExists(wsConfig, r.Role) {
				return nil, fmt.Errorf("el rol %q declarado en el diseño no existe en la configuración de roles de axiom.yaml", r.Role)
			}
		}
	}

	return roles, nil
}

// ParseRolesMarkdown busca y deserializa declaraciones de roles en el contenido markdown de design.md.
func ParseRolesMarkdown(content string) ([]RoleAssignment, error) {
	matches := yamlCodeBlockRegex.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		if len(m) > 1 && strings.Contains(m[1], "roles:") {
			var wrapper rolesWrapper
			if err := yaml.Unmarshal([]byte(m[1]), &wrapper); err == nil && len(wrapper.Roles) > 0 {
				return normalizeRoles(wrapper.Roles)
			}
		}
	}

	// Buscar si existe una lista directa de roles en YAML
	for _, m := range matches {
		if len(m) > 1 && strings.Contains(m[1], "role:") {
			var list []RoleAssignment
			if err := yaml.Unmarshal([]byte(m[1]), &list); err == nil && len(list) > 0 {
				return normalizeRoles(list)
			}
		}
	}

	return nil, nil
}

func normalizeRoles(roles []RoleAssignment) ([]RoleAssignment, error) {
	var normalized []RoleAssignment
	for _, r := range roles {
		roleName := strings.TrimSpace(r.Role)
		if roleName == "" {
			return nil, fmt.Errorf("se detectó una asignación de rol sin identificador 'role'")
		}
		r.Role = roleName

		// Por defecto, la política de compuerta es blocking
		policy := GatePolicy(strings.ToLower(strings.TrimSpace(string(r.GatePolicy))))
		if policy == "" {
			policy = PolicyBlocking
		}

		if policy != PolicyBlocking && policy != PolicyDeferred && policy != PolicyOptional {
			return nil, fmt.Errorf("política de compuerta 'gate_policy' inválida %q para el rol %q (admitidos: blocking, deferred, optional)", r.GatePolicy, r.Role)
		}
		r.GatePolicy = policy

		normalized = append(normalized, r)
	}

	return normalized, nil
}

func roleExists(cfg *workspace.WorkspaceConfig, role string) bool {
	for k, v := range cfg.Roles {
		if strings.EqualFold(k, role) || strings.EqualFold(v.Name, role) {
			return true
		}
	}
	return false
}
