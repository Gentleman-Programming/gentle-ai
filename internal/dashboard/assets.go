package dashboard

import "embed"

// AssetsFS embebe todos los recursos estáticos del frontend (HTML, CSS, JS)
// directamente dentro del binario compilado de Axiom.
//
//go:embed assets/*
var AssetsFS embed.FS
