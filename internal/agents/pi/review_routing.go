package pi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ReviewRouting contains argv-only overrides; zero values retain Pi's persisted defaults.
type ReviewRouting struct{ Model, Thinking string }

// ResolveReviewRouting reads gentle-pi's assignment authority without modifying it.
// root must be the validated repository root, never the transport scratch directory.
func ResolveReviewRouting(root, role string) (ReviewRouting, error) {
	var route ReviewRouting
	dir, set := os.LookupEnv("GENTLE_PI_CONFIG_HOME")
	if !set {
		home, err := os.UserHomeDir()
		if err != nil {
			return route, fmt.Errorf("locate Pi routing home: %w", err)
		}
		dir = filepath.Join(home, ".pi", "gentle-ai")
	}
	path := filepath.Join(dir, "models.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		path = filepath.Join(root, ".pi", "gentle-ai", "models.json")
		data, err = os.ReadFile(path)
	}
	if os.IsNotExist(err) {
		return route, nil
	}
	if err != nil {
		return route, fmt.Errorf("read Pi routing %s: %w", path, err)
	}
	invalid := func() (ReviewRouting, error) {
		return ReviewRouting{}, fmt.Errorf("invalid Pi routing for %s in %s; repair the assignment using gentle-pi model configuration before retrying", role, path)
	}
	var config map[string]json.RawMessage
	if json.Unmarshal(data, &config) != nil || config == nil {
		return invalid()
	}
	entry, exists := config[role]
	if !exists {
		return route, nil
	}
	var model string
	if json.Unmarshal(entry, &model) == nil && string(entry) != "null" {
		route.Model = strings.TrimSpace(model)
		if route.Model == "" {
			return invalid()
		}
	} else {
		var fields map[string]json.RawMessage
		if json.Unmarshal(entry, &fields) != nil || fields == nil {
			return invalid()
		}
		for key, value := range fields {
			switch key {
			case "model":
				if json.Unmarshal(value, &model) != nil {
					return invalid()
				}
				route.Model = strings.TrimSpace(model)
				if route.Model == "" {
					return invalid()
				}
			case "thinking":
				if json.Unmarshal(value, &route.Thinking) != nil {
					return invalid()
				}
				switch route.Thinking {
				case "off", "minimal", "low", "medium", "high", "xhigh", "max":
				default:
					return invalid()
				}
			default:
				return invalid()
			}
		}
	}
	if route.Model != "" && !regexp.MustCompile(`^[A-Za-z0-9._~:@/+%-]+$`).MatchString(route.Model) {
		return invalid()
	}
	return route, nil
}
