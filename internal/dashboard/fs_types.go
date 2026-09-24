package dashboard

// FileSystemDirectoryItem representa un directorio en la exploración del sistema de archivos local.
type FileSystemDirectoryItem struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Hidden bool   `json:"hidden"`
}

// FileSystemBrowseResult es la respuesta del endpoint de exploración de directorios locales.
type FileSystemBrowseResult struct {
	CurrentPath  string                    `json:"current_path"`
	ParentPath   string                    `json:"parent_path"`
	Drives       []string                  `json:"drives"`
	HomeDir      string                    `json:"home_dir"`
	WorkspaceDir string                    `json:"workspace_dir"`
	Separator    string                    `json:"separator"`
	Directories  []FileSystemDirectoryItem `json:"directories"`
}

// NativePickerRequest representa la petición para invocar el selector de carpetas del SO.
type NativePickerRequest struct {
	InitialPath string `json:"initial_path"`
}

// NativePickerResult representa la respuesta del selector de carpetas del SO.
type NativePickerResult struct {
	Path     string `json:"path,omitempty"`
	Canceled bool   `json:"canceled"`
	Error    string `json:"error,omitempty"`
}
