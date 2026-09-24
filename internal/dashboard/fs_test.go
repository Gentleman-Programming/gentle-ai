package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowseDirectories_EmptyPath(t *testing.T) {
	tempDir := t.TempDir()
	svc := NewService(tempDir)

	res, err := svc.BrowseDirectories("")
	if err != nil {
		t.Fatalf("BrowseDirectories falló con path vacío: %v", err)
	}

	if res.CurrentPath == "" {
		t.Errorf("CurrentPath no debería ser vacío")
	}
	if len(res.Drives) == 0 {
		t.Errorf("Drives no debería ser vacío")
	}
	if res.Separator == "" {
		t.Errorf("Separator no debería ser vacío")
	}
}

func TestBrowseDirectories_ValidDir(t *testing.T) {
	tempDir := t.TempDir()
	svc := NewService(tempDir)

	// Crear subdirectorios de prueba
	sub1 := filepath.Join(tempDir, "sub1")
	sub2 := filepath.Join(tempDir, "sub2")
	hidden := filepath.Join(tempDir, ".hidden")
	file := filepath.Join(tempDir, "test.txt")

	_ = os.Mkdir(sub1, 0755)
	_ = os.Mkdir(sub2, 0755)
	_ = os.Mkdir(hidden, 0755)
	_ = os.WriteFile(file, []byte("hola"), 0644)

	res, err := svc.BrowseDirectories(tempDir)
	if err != nil {
		t.Fatalf("BrowseDirectories falló en tempDir: %v", err)
	}

	if len(res.Directories) != 3 {
		t.Fatalf("Esperaba 3 subdirectorios, obtuvo %d", len(res.Directories))
	}

	foundHidden := false
	for _, d := range res.Directories {
		if d.Name == ".hidden" {
			foundHidden = true
			if !d.Hidden {
				t.Errorf(".hidden debería estar marcado como Hidden=true")
			}
		}
	}
	if !foundHidden {
		t.Errorf("No se encontró el directorio .hidden en los resultados")
	}
}

func TestFSDirectoriesEndpoint(t *testing.T) {
	tempDir := t.TempDir()
	subDir := filepath.Join(tempDir, "test_proj")
	_ = os.Mkdir(subDir, 0755)

	svc := NewService(tempDir)
	server := NewServer(svc)

	// 1. GET válido
	req := httptest.NewRequest(http.MethodGet, "/api/fs/directories?path="+tempDir, nil)
	rr := httptest.NewRecorder()
	server.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/fs/directories falló con código %d: %s", rr.Code, rr.Body.String())
	}

	var res FileSystemBrowseResult
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("Error deserializando JSON de respuesta: %v", err)
	}

	if res.CurrentPath != tempDir {
		t.Errorf("CurrentPath = %s, esperado %s", res.CurrentPath, tempDir)
	}

	// 2. Método no permitido (POST a /api/fs/directories)
	reqPost := httptest.NewRequest(http.MethodPost, "/api/fs/directories", nil)
	rrPost := httptest.NewRecorder()
	server.Router().ServeHTTP(rrPost, reqPost)

	if rrPost.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/fs/directories debería retornar 405, obtuvo %d", rrPost.Code)
	}
}

func TestFSNativePickerEndpoint(t *testing.T) {
	tempDir := t.TempDir()
	svc := NewService(tempDir)
	server := NewServer(svc)

	// GET a native-picker debe dar 405
	reqGet := httptest.NewRequest(http.MethodGet, "/api/fs/native-picker", nil)
	rrGet := httptest.NewRecorder()
	server.Router().ServeHTTP(rrGet, reqGet)

	if rrGet.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/fs/native-picker debería dar 405, obtuvo %d", rrGet.Code)
	}
}

func TestEmbeddedAssets_FolderPicker(t *testing.T) {
	tempDir := t.TempDir()
	svc := NewService(tempDir)
	server := NewServer(svc)

	// 1. Validar que index.html contiene el modal y el botón de examinar
	reqHTML := httptest.NewRequest(http.MethodGet, "/", nil)
	rrHTML := httptest.NewRecorder()
	server.Router().ServeHTTP(rrHTML, reqHTML)
	if rrHTML.Code != http.StatusOK {
		t.Fatalf("GET / falló con código %d", rrHTML.Code)
	}
	bodyHTML := rrHTML.Body.String()
	for _, marker := range []string{"modal-folder-picker", "btn-browse-project-path", "btn-browse-header-project"} {
		if !strings.Contains(bodyHTML, marker) {
			t.Errorf("index.html embebido debe contener '%s'", marker)
		}
	}

	// 2. Validar que app.js contiene la lógica del folder picker
	reqJS := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	rrJS := httptest.NewRecorder()
	server.Router().ServeHTTP(rrJS, reqJS)
	if rrJS.Code != http.StatusOK {
		t.Fatalf("GET /app.js falló con código %d", rrJS.Code)
	}
	bodyJS := rrJS.Body.String()
	for _, marker := range []string{"openFolderPicker", "btn-browse-role-repo", "folderPickerState"} {
		if !strings.Contains(bodyJS, marker) {
			t.Errorf("app.js embebido debe contener '%s'", marker)
		}
	}

	// 3. Validar que style.css contiene los estilos del folder picker
	reqCSS := httptest.NewRequest(http.MethodGet, "/style.css", nil)
	rrCSS := httptest.NewRecorder()
	server.Router().ServeHTTP(rrCSS, reqCSS)
	if rrCSS.Code != http.StatusOK {
		t.Fatalf("GET /style.css falló con código %d", rrCSS.Code)
	}
	bodyCSS := rrCSS.Body.String()
	if !strings.Contains(bodyCSS, "folder-picker-backdrop") {
		t.Errorf("style.css embebido debe contener 'folder-picker-backdrop'")
	}
}
