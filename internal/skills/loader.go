package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	skillFileName = "SKILL.md"
	maxScanDepth  = 6
	maxNameLen    = 64
	maxDescLen    = 1024
)

// skillFrontmatter is the YAML structure at the top of SKILL.md files.
type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Metadata    struct {
		ShortDescription string `yaml:"short-description"`
	} `yaml:"metadata"`
}

// SkillRoots returns the directories to scan for skills.
// Scans:
//   - cwd/.agents/skills and ancestors (repo scope)
//   - codexHome/skills (user scope, excluding .system)
//   - codexHome/skills/.system (system scope)
func SkillRoots(codexHome, cwd string) []skillRoot {
	var roots []skillRoot

	// Repo scope: walk up from cwd looking for .agents/skills
	if cwd != "" {
		dir := cwd
		for {
			candidate := filepath.Join(dir, ".agents", "skills")
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				roots = append(roots, skillRoot{Path: candidate, Scope: ScopeRepo})
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	// User scope: codexHome/skills (excluding .system subdirectory)
	if codexHome != "" {
		userDir := filepath.Join(codexHome, "skills")
		if info, err := os.Stat(userDir); err == nil && info.IsDir() {
			roots = append(roots, skillRoot{Path: userDir, Scope: ScopeUser})
		}

		// System scope: codexHome/skills/.system
		systemDir := filepath.Join(codexHome, "skills", ".system")
		if info, err := os.Stat(systemDir); err == nil && info.IsDir() {
			roots = append(roots, skillRoot{Path: systemDir, Scope: ScopeSystem})
		}
	}

	// Admin scope: /etc/codex/skills
	adminDir := "/etc/codex/skills"
	if info, err := os.Stat(adminDir); err == nil && info.IsDir() {
		roots = append(roots, skillRoot{Path: adminDir, Scope: ScopeAdmin})
	}

	return roots
}

type skillRoot struct {
	Path  string
	Scope SkillScope
}

// DiscoverSkills scans all skill roots and returns discovered skills.
// Skills are deduplicated by path and sorted by scope then name.
func DiscoverSkills(codexHome, cwd string) ([]SkillMetadata, error) {
	roots := SkillRoots(codexHome, cwd)
	seen := make(map[string]bool)
	var all []SkillMetadata

	for _, root := range roots {
		skills, err := scanDirectory(root.Path, root.Scope, 0)
		if err != nil {
			continue // skip inaccessible roots
		}
		for _, s := range skills {
			if !seen[s.Path] {
				seen[s.Path] = true
				all = append(all, s)
			}
		}
	}

	// Sort by scope priority then name
	sort.Slice(all, func(i, j int) bool {
		si, sj := scopePriority(all[i].Scope), scopePriority(all[j].Scope)
		if si != sj {
			return si < sj
		}
		return all[i].Name < all[j].Name
	})

	return all, nil
}

func scopePriority(s SkillScope) int {
	switch s {
	case ScopeSystem:
		return 0
	case ScopeAdmin:
		return 1
	case ScopeUser:
		return 2
	case ScopeRepo:
		return 3
	default:
		return 4
	}
}

// scanDirectory recursively scans a directory for SKILL.md files up to maxScanDepth.
func scanDirectory(dir string, scope SkillScope, depth int) ([]SkillMetadata, error) {
	if depth > maxScanDepth {
		return nil, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var results []SkillMetadata

	// Check for SKILL.md in this directory
	skillPath := filepath.Join(dir, skillFileName)
	if meta, err := parseSkillFile(skillPath, scope); err == nil {
		results = append(results, meta)
	}

	// Recurse into subdirectories
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip hidden directories (except .system which is handled as a root)
		if strings.HasPrefix(name, ".") {
			continue
		}
		sub, err := scanDirectory(filepath.Join(dir, name), scope, depth+1)
		if err != nil {
			continue
		}
		results = append(results, sub...)
	}

	return results, nil
}

// parseSkillFile reads and parses a single SKILL.md file.
func parseSkillFile(path string, scope SkillScope) (SkillMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SkillMetadata{}, err
	}

	fm, _, err := parseFrontmatter(string(data))
	if err != nil {
		return SkillMetadata{}, fmt.Errorf("parse frontmatter %s: %w", path, err)
	}

	if fm.Name == "" {
		return SkillMetadata{}, fmt.Errorf("skill %s: missing name", path)
	}
	if len(fm.Name) > maxNameLen {
		return SkillMetadata{}, fmt.Errorf("skill %s: name exceeds %d characters", path, maxNameLen)
	}
	if fm.Description == "" {
		return SkillMetadata{}, fmt.Errorf("skill %s: missing description", path)
	}
	if len(fm.Description) > maxDescLen {
		fm.Description = fm.Description[:maxDescLen]
	}

	absPath, _ := filepath.Abs(path)
	return SkillMetadata{
		Name:             fm.Name,
		Description:      fm.Description,
		ShortDescription: fm.Metadata.ShortDescription,
		Path:             absPath,
		Scope:            scope,
	}, nil
}

// parseFrontmatter extracts YAML frontmatter and body from a SKILL.md file.
// The file must start with "---\n", contain YAML, then "---\n", followed by body.
func parseFrontmatter(content string) (skillFrontmatter, string, error) {
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return skillFrontmatter{}, "", fmt.Errorf("missing opening frontmatter delimiter")
	}

	// Find the closing delimiter
	rest := content[4:] // skip "---\n"
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		idx = strings.Index(rest, "\n---\r\n")
	}
	if idx < 0 {
		return skillFrontmatter{}, "", fmt.Errorf("missing closing frontmatter delimiter")
	}

	yamlContent := rest[:idx]
	body := strings.TrimSpace(rest[idx+5:]) // skip "\n---\n"

	var fm skillFrontmatter
	if err := yaml.Unmarshal([]byte(yamlContent), &fm); err != nil {
		return skillFrontmatter{}, "", fmt.Errorf("parse YAML: %w", err)
	}

	return fm, body, nil
}

// LoadSkillContent reads a SKILL.md file and returns its full content.
func LoadSkillContent(path string) (SkillContent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SkillContent{}, err
	}

	fm, body, err := parseFrontmatter(string(data))
	if err != nil {
		return SkillContent{}, err
	}

	absPath, _ := filepath.Abs(path)
	return SkillContent{
		SkillMetadata: SkillMetadata{
			Name:             fm.Name,
			Description:      fm.Description,
			ShortDescription: fm.Metadata.ShortDescription,
			Path:             absPath,
		},
		Body: body,
	}, nil
}
