package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"netatlas/internal/ingest"
)

func inputOffsetStatePath(inputPath, configuredStatePath string) string {
	stateDir := filepath.Dir(configuredStatePath)
	base := filepath.Base(inputPath)
	base = strings.TrimSpace(base)
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "input"
	}
	return filepath.Join(stateDir, base+"-dnslog-offset.json")
}

func prepareInputOffsetState(inputPath, configuredStatePath string, dryRun bool) (string, error) {
	statePath := inputOffsetStatePath(inputPath, configuredStatePath)
	if dryRun {
		return statePath, nil
	}

	store := ingest.NewOffsetStore(statePath)
	if err := store.EnsureDir(); err != nil {
		return "", err
	}

	current, err := currentInputState(inputPath)
	if err != nil {
		return "", err
	}

	if err := migrateMatchingDefaultState(configuredStatePath, statePath, current); err != nil {
		return "", err
	}
	if err := moveCollidingState(statePath, current); err != nil {
		return "", err
	}

	return statePath, nil
}

func currentInputState(inputPath string) (ingest.OffsetState, error) {
	info, err := os.Stat(inputPath)
	if err != nil {
		return ingest.OffsetState{}, fmt.Errorf("stat input file %q: %w", inputPath, err)
	}
	current, err := ingest.CurrentState(inputPath, info)
	if err != nil {
		return ingest.OffsetState{}, fmt.Errorf("identify input file %q: %w", inputPath, err)
	}
	return current, nil
}

func migrateMatchingDefaultState(defaultStatePath, inputStatePath string, current ingest.OffsetState) error {
	if defaultStatePath == inputStatePath {
		return nil
	}
	if _, err := os.Stat(inputStatePath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat input offset state %q: %w", inputStatePath, err)
	}

	defaultStore := ingest.NewOffsetStore(defaultStatePath)
	saved, err := defaultStore.Load()
	if err != nil {
		return err
	}
	if isZeroOffsetState(saved) || !offsetStateBelongsToInput(saved, current) {
		return nil
	}

	if err := os.Rename(defaultStatePath, inputStatePath); err != nil {
		return fmt.Errorf("migrate offset state %q to %q: %w", defaultStatePath, inputStatePath, err)
	}
	return nil
}

func moveCollidingState(statePath string, current ingest.OffsetState) error {
	store := ingest.NewOffsetStore(statePath)
	saved, err := store.Load()
	if err != nil {
		return err
	}
	if isZeroOffsetState(saved) || offsetStateBelongsToInput(saved, current) {
		return nil
	}

	archivePath := nextCollisionStatePath(statePath, saved)
	if err := os.Rename(statePath, archivePath); err != nil {
		return fmt.Errorf("move colliding offset state %q to %q: %w", statePath, archivePath, err)
	}
	return nil
}

func offsetStateBelongsToInput(saved, current ingest.OffsetState) bool {
	if saved.Path != "" && sameCleanPath(saved.Path, current.Path) {
		return true
	}
	if saved.FileID != (ingest.FileID{}) && ingest.SameFile(saved.FileID, current.FileID) {
		return true
	}
	return false
}

func sameCleanPath(a, b string) bool {
	absA, err := filepath.Abs(a)
	if err == nil {
		a = absA
	}
	absB, err := filepath.Abs(b)
	if err == nil {
		b = absB
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func isZeroOffsetState(state ingest.OffsetState) bool {
	return state.Path == "" &&
		state.Offset == 0 &&
		state.Size == 0 &&
		state.FileID == (ingest.FileID{})
}

func nextCollisionStatePath(statePath string, saved ingest.OffsetState) string {
	ext := filepath.Ext(statePath)
	stem := strings.TrimSuffix(statePath, ext)
	suffix := "previous"
	if saved.FileID != (ingest.FileID{}) {
		suffix = fmt.Sprintf("dev-%d-inode-%d", saved.FileID.Device, saved.FileID.Inode)
	}

	candidate := fmt.Sprintf("%s.%s%s", stem, suffix, ext)
	for i := 1; ; i++ {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
		candidate = fmt.Sprintf("%s.%s.%d%s", stem, suffix, i, ext)
	}
}
