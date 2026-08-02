package config

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"strings"
)

// dotEnvFile is read from the working directory, which is backend/ when the
// server is started the documented way.
const dotEnvFile = ".env"

// loadDotEnv reads KEY=VALUE lines from path into the environment.
//
// A missing file is not an error: in production the environment is set by
// whatever runs the process, and this is only here so a developer can run the
// server without exporting five variables in every shell.
//
// **A real environment variable always wins.** The file fills gaps, it does not
// override, so `DATABASE_URL=... go run ./cmd/switchyard` behaves the way the
// person typing it expects.
func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := parseDotEnvLine(scanner.Text())
		if !ok {
			continue
		}
		if _, set := os.LookupEnv(key); set {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// parseDotEnvLine handles the subset of .env syntax worth supporting: comments,
// blank lines, an optional `export`, and quoted or bare values. Anything more
// elaborate belongs in the real environment, not a dev convenience file.
func parseDotEnvLine(line string) (key, value string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", "", false
	}

	trimmed = strings.TrimPrefix(trimmed, "export ")

	key, value, found := strings.Cut(trimmed, "=")
	if !found {
		return "", "", false
	}

	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", false
	}
	value = strings.TrimSpace(value)

	// Quotes are stripped, so a value with spaces or a trailing comment
	// character survives intact.
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			return key, value[1 : len(value)-1], true
		}
	}
	return key, value, true
}
