package multirole

import "testing"

// TestIsReservedRole is task 15.2's RED: IsReservedRole does not exist yet,
// so this fails to compile until task 15.3 creates it.
func TestIsReservedRole(t *testing.T) {
	tests := []struct {
		name string
		role string
		want bool
	}{
		{name: "identidad reservada exacta", role: "fullstack", want: true},
		{name: "identidad reservada insensible a mayusculas", role: "Fullstack", want: true},
		{name: "identidad reservada en mayusculas", role: "FULLSTACK", want: true},
		{name: "rol de workspace ordinario no es reservado", role: "database", want: false},
		{name: "cadena vacia no es reservada", role: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsReservedRole(tt.role); got != tt.want {
				t.Errorf("IsReservedRole(%q) = %v, se esperaba %v", tt.role, got, tt.want)
			}
		})
	}
}

// TestRoleFullstackConstantValue pins the exact literal RoleFullstack
// carries: every caller across the codebase (detector.go, handoff's
// validator.go, and any future consumer) that seals or recognises the
// reserved identity must agree on this exact string.
func TestRoleFullstackConstantValue(t *testing.T) {
	if RoleFullstack != "fullstack" {
		t.Fatalf("RoleFullstack = %q, se esperaba %q", RoleFullstack, "fullstack")
	}
}
