package semantic

// ConnectorType define los tipos de conectores semánticos admitidos en Axiom.
type ConnectorType string

const (
	// ConnectorSerena representa el servidor MCP de Serena (LSP / Tree-sitter).
	ConnectorSerena ConnectorType = "serena"

	// ConnectorCodeGraph representa la herramienta de grafos de conocimiento de código.
	ConnectorCodeGraph ConnectorType = "codegraph"

	// ConnectorNativeAST representa el motor semántico autónomo nativo en Go (go/parser, go/ast).
	ConnectorNativeAST ConnectorType = "native-ast"

	// ConnectorAuto indica resolución automática priorizando conectores externos y fallback a native-ast.
	ConnectorAuto ConnectorType = "auto"
)

// SymbolKind define la clasificación de un símbolo de código analizado.
type SymbolKind string

const (
	KindStruct    SymbolKind = "struct"
	KindInterface SymbolKind = "interface"
	KindFunc      SymbolKind = "func"
	KindMethod    SymbolKind = "method"
	KindType      SymbolKind = "type"
)

// SymbolItem representa un símbolo de código extraído con su ubicación exacta.
type SymbolItem struct {
	Name       string     `json:"name"`
	Kind       SymbolKind `json:"kind"`
	Package    string     `json:"package"`
	FilePath   string     `json:"file_path"`
	LineNumber int        `json:"line_number"`
	Signature  string     `json:"signature,omitempty"`
	Receiver   string     `json:"receiver,omitempty"`
	DocComment string     `json:"doc_comment,omitempty"`
	Role       string     `json:"role,omitempty"`
}

// DependencyRelation representa una arista en el grafo de dependencias entre paquetes.
type DependencyRelation struct {
	SourcePackage string `json:"source_package"`
	TargetPackage string `json:"target_package"`
	IsInternal    bool   `json:"is_internal"`
	FilePath      string `json:"file_path,omitempty"`
	Role          string `json:"role,omitempty"`
}

// AgentToolStatus describe la presencia y configuración de una herramienta semántica en un agente.
type AgentToolStatus struct {
	AgentName  string `json:"agent_name"`
	ConfigPath string `json:"config_path"`
	Configured bool   `json:"configured"`
	Details    string `json:"details,omitempty"`
}

// SemanticStatus contiene el diagnóstico integral del entorno semántico y métricas del workspace.
type SemanticStatus struct {
	ConfiguredConnector ConnectorType     `json:"configured_connector"`
	ActiveConnector     ConnectorType     `json:"active_connector"`
	SerenaAvailable     bool              `json:"serena_available"`
	CodeGraphAvailable  bool              `json:"codegraph_available"`
	NativeASTReady      bool              `json:"native_ast_ready"`
	Agents              []AgentToolStatus `json:"agents"`
	TotalPackages       int               `json:"total_packages"`
	TotalSymbols        int               `json:"total_symbols"`
	Warnings            []string          `json:"warnings,omitempty"`
}

// SemanticQuery define los filtros opcionales para consultar símbolos de código.
type SemanticQuery struct {
	Query string     `json:"query,omitempty"`
	Kind  SymbolKind `json:"kind,omitempty"`
	Role  string     `json:"role,omitempty"`
}

// ReindexResult reporta el resultado de la reindexación de CodeGraph.
type ReindexResult struct {
	Success   bool   `json:"success"`
	Connector string `json:"connector"`
	Message   string `json:"message"`
	Output    string `json:"output,omitempty"`
	Duration  string `json:"duration"`
}
