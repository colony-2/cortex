package alias

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var aliasNameRe = regexp.MustCompile(`^[a-z0-9_-]+$`)

// Entry represents a single alias command definition.
type Entry struct {
	Name       string
	Command    string
	ArgsRegex  string
	compiledRE *regexp.Regexp
}

// Manifest represents parsed alias definitions.
type Manifest struct {
	Entries map[string]*Entry
}

// LoadManifest parses the .shai-cmds file.
func LoadManifest(path string) (*Manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	manifest := &Manifest{
		Entries: make(map[string]*Entry),
	}

	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		entry, err := parseLine(line)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", filepath.Base(path), lineNum, err)
		}
		if _, exists := manifest.Entries[entry.Name]; exists {
			return nil, fmt.Errorf("%s:%d: duplicate alias %q", filepath.Base(path), lineNum, entry.Name)
		}
		manifest.Entries[entry.Name] = entry
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return manifest, nil
}

func parseLine(line string) (*Entry, error) {
	aliasToken, rest := readField(line)
	if aliasToken == "" || rest == "" {
		return nil, fmt.Errorf("invalid format: expected alias, regex, command")
	}
	regexToken, remainder := readField(rest)
	if regexToken == "" || remainder == "" {
		return nil, fmt.Errorf("invalid format: expected alias, regex, command")
	}
	command := strings.TrimSpace(remainder)
	if command == "" {
		return nil, fmt.Errorf("missing command")
	}
	if !aliasNameRe.MatchString(aliasToken) {
		return nil, fmt.Errorf("invalid alias %q", aliasToken)
	}
	entry := &Entry{
		Name:    aliasToken,
		Command: command,
	}
	if regexToken != "-" {
		wrapped := fmt.Sprintf("^(?:%s)$", regexToken)
		re, err := regexp.Compile(wrapped)
		if err != nil {
			return nil, fmt.Errorf("invalid regex for %q: %w", aliasToken, err)
		}
		entry.ArgsRegex = regexToken
		entry.compiledRE = re
	}
	return entry, nil
}

func readField(line string) (field, rest string) {
	line = strings.TrimLeft(line, " \t")
	if line == "" {
		return "", ""
	}
	for i := 0; i < len(line); i++ {
		if line[i] == ' ' || line[i] == '\t' {
			return line[:i], line[i+1:]
		}
	}
	return line, ""
}

// ValidateArgs ensures the provided arg string matches the manifest entry.
func (e *Entry) ValidateArgs(argString string) error {
	if e.compiledRE == nil {
		if strings.TrimSpace(argString) != "" {
			return fmt.Errorf("alias %q does not accept arguments", e.Name)
		}
		return nil
	}
	if e.compiledRE.MatchString(argString) {
		return nil
	}
	return fmt.Errorf("arguments %q do not match allowed pattern %q", argString, e.ArgsRegex)
}
