package autoskill

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// DefaultRegistryBaseURL es la URL canónica del registro oficial de midudev/autoskills en GitHub.
	DefaultRegistryBaseURL = "https://raw.githubusercontent.com/midudev/autoskills/main/packages/autoskills/skills-registry"
)

// ClientOptions define los parámetros configurables para el cliente del registro.
type ClientOptions struct {
	BaseURL    string
	HTTPClient *http.Client
	CacheDir   string
	Offline    bool
}

// Client gestiona las comunicaciones HTTP con el registro de midudev/autoskills y la verificación criptográfica.
type Client struct {
	baseURL    string
	httpClient *http.Client
	cacheDir   string
	offline    bool
}

// NewClient inicializa un cliente con opciones personalizadas o valores predeterminados seguros.
func NewClient(opts ClientOptions) *Client {
	baseURL := strings.TrimRight(opts.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultRegistryBaseURL
	}

	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 15 * time.Second,
		}
	}

	return &Client{
		baseURL:    baseURL,
		httpClient: httpClient,
		cacheDir:   opts.CacheDir,
		offline:    opts.Offline,
	}
}

// FetchIndex obtiene el manifiesto index.json, recurriendo a caché si opera en modo offline o ante fallos de red.
func (c *Client) FetchIndex(ctx context.Context) (*RegistryIndex, error) {
	manifestRel := "index.json"

	// 1. Si está en modo offline, buscar directamente en caché
	if c.offline {
		if c.cacheDir != nilString() {
			cachedData, err := os.ReadFile(filepath.Join(c.cacheDir, manifestRel))
			if err == nil {
				return parseIndex(cachedData)
			}
		}
		return nil, errors.New("modo offline activo y no se encontró index.json en la caché local")
	}

	// 2. Intentar descarga remota
	url := fmt.Sprintf("%s/%s", c.baseURL, manifestRel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("error construyendo petición HTTP para index.json: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Fallback a caché si existe
		if c.cacheDir != nilString() {
			if cachedData, cErr := os.ReadFile(filepath.Join(c.cacheDir, manifestRel)); cErr == nil {
				return parseIndex(cachedData)
			}
		}
		return nil, fmt.Errorf("error descargando index.json desde %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Fallback a caché si existe
		if c.cacheDir != nilString() {
			if cachedData, cErr := os.ReadFile(filepath.Join(c.cacheDir, manifestRel)); cErr == nil {
				return parseIndex(cachedData)
			}
		}
		return nil, fmt.Errorf("respuesta HTTP no exitosa (%d) al obtener index.json", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error leyendo respuesta de index.json: %w", err)
	}

	// Guardar en caché si está configurada
	if c.cacheDir != nilString() {
		_ = os.MkdirAll(c.cacheDir, 0755)
		_ = os.WriteFile(filepath.Join(c.cacheDir, manifestRel), body, 0644)
	}

	return parseIndex(body)
}

// FetchSkillFile descarga un archivo individual de una skill.
func (c *Client) FetchSkillFile(ctx context.Context, skillPath, filePath string) ([]byte, error) {
	cachePath := ""
	if c.cacheDir != nilString() {
		cachePath = filepath.Join(c.cacheDir, skillPath, filePath)
	}

	// Si estamos offline o ya existe en caché, leerlo de disco
	if c.offline {
		if cachePath != "" {
			if data, err := os.ReadFile(cachePath); err == nil {
				return data, nil
			}
		}
		return nil, fmt.Errorf("modo offline activo y no se encontró %s en caché", filePath)
	}

	url := fmt.Sprintf("%s/%s/%s", c.baseURL, skillPath, filePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creando petición HTTP para %s: %w", url, err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if cachePath != "" {
			if data, cErr := os.ReadFile(cachePath); cErr == nil {
				return data, nil
			}
		}
		return nil, fmt.Errorf("error descargando archivo %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if cachePath != "" {
			if data, cErr := os.ReadFile(cachePath); cErr == nil {
				return data, nil
			}
		}
		return nil, fmt.Errorf("código HTTP %d al descargar %s", resp.StatusCode, url)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error leyendo cuerpo de %s: %w", url, err)
	}

	// Guardar en caché
	if cachePath != "" {
		_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
		_ = os.WriteFile(cachePath, data, 0644)
	}

	return data, nil
}

// VerifySHA256 calcula la suma de verificación SHA-256 de los bytes dados y la compara con la esperada.
func (c *Client) VerifySHA256(data []byte, expectedHash string) (bool, string) {
	hasher := sha256.New()
	hasher.Write(data)
	actualHash := hex.EncodeToString(hasher.Sum(nil))

	match := strings.EqualFold(actualHash, expectedHash)
	return match, actualHash
}

// FetchAndVerifySkill descarga todos los archivos de una skill y valida exhaustivamente sus hashes SHA-256.
func (c *Client) FetchAndVerifySkill(ctx context.Context, skillName string, entry RegistrySkillEntry) (map[string][]byte, error) {
	result := make(map[string][]byte)

	skillPath := entry.SkillPath
	if skillPath == "" {
		skillPath = skillName
	}

	for _, file := range entry.Files {
		data, err := c.FetchSkillFile(ctx, skillPath, file)
		if err != nil {
			// Si falla con skillPath, intentar con skillName
			if skillPath != skillName {
				var fallbackErr error
				data, fallbackErr = c.FetchSkillFile(ctx, skillName, file)
				if fallbackErr != nil {
					return nil, fmt.Errorf("error descargando archivo %s para skill %s: %w", file, skillName, err)
				}
			} else {
				return nil, fmt.Errorf("error descargando archivo %s para skill %s: %w", file, skillName, err)
			}
		}

		// Validar SHA-256 si está presente en el manifiesto
		expectedHash := entry.SHA256[file]
		if expectedHash != "" {
			valid, actualHash := c.VerifySHA256(data, expectedHash)
			if !valid {
				return nil, fmt.Errorf("discrepancia de integridad criptográfica SHA-256 en %s (skill: %s): esperado %s, obtenido %s", file, skillName, expectedHash, actualHash)
			}
		}

		result[file] = data
	}

	return result, nil
}

func parseIndex(data []byte) (*RegistryIndex, error) {
	var idx RegistryIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("error deserializando index.json: %w", err)
	}
	return &idx, nil
}

func nilString() string {
	return ""
}
