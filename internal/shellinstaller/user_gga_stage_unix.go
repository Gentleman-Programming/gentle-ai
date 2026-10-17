//go:build linux || darwin

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
)

// Configuration only: no CLI installation, provider authentication or hooks.
// GGA v2.10.1 honors the private XDG_CONFIG_HOME before HOME/.config.
func userStageGGA(ctx context.Context, root, mode, choice, provider string) error {
	if err := userValidateGGASelection(choice, provider); err != nil {
		return err
	}
	if choice == "" {
		return nil
	}
	if mode != "separate" {
		return experienceError("explicit GGA requires a private Separate configuration stage")
	}
	if choice == "off" {
		return nil
	}
	if ctx == nil {
		return privateError("canceled", context.Canceled)
	}
	if err := ctx.Err(); err != nil {
		return privateError("canceled", err)
	}
	if !privateHierarchyPath(root) {
		return experienceError("GGA stage requires an absolute private root")
	}
	rootIdentity, err := privateDirectory(root)
	if err != nil {
		return err
	}
	config := filepath.Join(root, "config")
	identity, err := privateDirectory(config)
	if err != nil {
		return err
	}
	directory, err := os.OpenRoot(config)
	if err != nil {
		return err
	}
	defer directory.Close()
	opened, err := directory.Stat(".")
	if err != nil || !os.SameFile(identity, opened) {
		return privateError("preimage", err)
	}
	if err := directory.Mkdir("gga", 0700); err != nil {
		return err
	}
	gga := filepath.Join(config, "gga")
	ggaIdentity, err := privateDirectory(gga)
	if err != nil {
		return err
	}
	files := []userExperienceStageFile{{"config", []byte("PROVIDER=\"" + provider + "\"\n")}}
	if err := userStageConfigFiles(ctx, root, "config/gga", files); err != nil {
		return err
	}
	for path, before := range map[string]os.FileInfo{root: rootIdentity, config: identity, gga: ggaIdentity} {
		fresh, err := privateDirectory(path)
		if err != nil || !os.SameFile(before, fresh) {
			return privateError("preimage", err)
		}
	}
	return nil
}
