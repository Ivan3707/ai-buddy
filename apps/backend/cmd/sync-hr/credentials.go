package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

func privateCredentialsPath(path, repoRoot string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", errors.New("-credentials must be an absolute path")
	}
	path = filepath.Clean(path)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	resolvedRepo, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(resolvedRepo, resolvedDir)
	if err != nil {
		return "", err
	}
	if rel != "secrets" && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))) {
		return "", errors.New("credential file must be outside the repository or in its secrets directory")
	}
	if _, err := os.Lstat(path); err == nil {
		return "", errors.New("credential file already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return path, nil
}

func findRepositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("run sync-hr from inside the Git repository")
		}
		dir = parent
	}
}

// publishCredentials makes a complete, synced file visible with an exclusive
// hard link. A failed write never leaves a partial file at the target path.
func publishCredentials(path string, credentials []service.InitialCredential) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".credentials-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err = temp.Chmod(0600); err != nil {
		temp.Close()
		return err
	}
	if err = restrictWindowsACL(tempPath); err != nil {
		temp.Close()
		return err
	}
	if err = json.NewEncoder(temp).Encode(credentials); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = os.Link(tempPath, path); err != nil {
		return err
	}
	if err = os.Remove(tempPath); err != nil {
		if cleanupErr := os.Remove(path); cleanupErr != nil {
			return errors.Join(err, fmt.Errorf("remove published credentials after cleanup failure: %w", cleanupErr))
		}
		return err
	}
	return nil
}

func restrictWindowsACL(path string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	identity, err := user.Current()
	if err != nil {
		return err
	}
	args := []string{path, "/inheritance:r", "/grant:r", "*" + identity.Uid + ":(R,W)", "*S-1-5-18:F", "*S-1-5-32-544:F"}
	if output, err := exec.Command("icacls", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("restrict credentials ACL: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
