package awsconfigure

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// setting is one key = value line in an AWS shared config or credentials file.
type setting struct {
	key, value string
}

// writeCredentials sets each of creds in profile's section of the shared
// credentials file, keeping every other line, and leaves the file at mode 0600.
func writeCredentials(profile string, creds []setting) error {
	path, err := credentialsFile()
	if err != nil {
		return err
	}
	content, err := os.ReadFile(path) //nolint:gosec
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	if err := f.Chmod(0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	if _, err := f.WriteString(setINI(string(content), profile, creds)); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return f.Close()
}

// credentialsFile returns the shared credentials file the AWS CLI reads:
// $AWS_SHARED_CREDENTIALS_FILE if set, otherwise ~/.aws/credentials.
func credentialsFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	path := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
	if path == "" {
		return filepath.Join(home, ".aws", "credentials"), nil
	}
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		return filepath.Join(home, rest), nil
	}
	return path, nil
}

// setINI returns content with settings set in section. Existing keys are
// replaced in place, missing keys are added at the end of the section, and the
// section is appended if absent.
func setINI(content, section string, settings []setting) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if content == "" {
		lines = nil
	}

	start, end := -1, len(lines)
	for i, line := range lines {
		name, ok := sectionName(line)
		if !ok {
			continue
		}
		if start >= 0 {
			end = i
			break
		}
		if name == section {
			start = i
		}
	}
	if start < 0 {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "["+section+"]")
		start, end = len(lines)-1, len(lines)
	}

	var missing []string
	for _, s := range settings {
		line := s.key + " = " + s.value
		found := false
		for i := start + 1; i < end; i++ {
			if key, _, ok := strings.Cut(lines[i], "="); ok && strings.TrimSpace(key) == s.key {
				lines[i] = line
				found = true
			}
		}
		if !found {
			missing = append(missing, line)
		}
	}

	insert := end
	for insert > start+1 && strings.TrimSpace(lines[insert-1]) == "" {
		insert--
	}
	lines = append(lines[:insert], append(missing, lines[insert:]...)...)
	return strings.Join(lines, "\n") + "\n"
}

// sectionName returns the name in a "[name]" header line.
func sectionName(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "[") || !strings.HasSuffix(line, "]") {
		return "", false
	}
	return strings.TrimSpace(line[1 : len(line)-1]), true
}
