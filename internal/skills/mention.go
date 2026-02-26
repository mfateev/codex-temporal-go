package skills

import (
	"regexp"
	"strings"
)

// mentionPattern matches $skill-name tokens in user input.
// Skill names are alphanumeric with hyphens and underscores.
var mentionPattern = regexp.MustCompile(`\$([a-zA-Z0-9][a-zA-Z0-9_-]*)`)

// ParseMentions extracts skill names from user input text.
// Returns unique names in order of first appearance.
func ParseMentions(text string) []string {
	matches := mentionPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var names []string
	for _, m := range matches {
		name := m[1]
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

// ResolveMentions maps mentioned skill names to their metadata from the loaded skills list.
// Returns only skills that match by name (case-insensitive) and are not disabled.
func ResolveMentions(names []string, loaded []SkillMetadata, disabledPaths []string) []SkillMetadata {
	if len(names) == 0 || len(loaded) == 0 {
		return nil
	}

	disabled := make(map[string]bool, len(disabledPaths))
	for _, p := range disabledPaths {
		disabled[p] = true
	}

	// Build lookup by lowercase name
	byName := make(map[string]SkillMetadata, len(loaded))
	for _, s := range loaded {
		lower := strings.ToLower(s.Name)
		if !disabled[s.Path] {
			byName[lower] = s
		}
	}

	var resolved []SkillMetadata
	seen := make(map[string]bool)
	for _, name := range names {
		lower := strings.ToLower(name)
		if s, ok := byName[lower]; ok && !seen[s.Path] {
			seen[s.Path] = true
			resolved = append(resolved, s)
		}
	}
	return resolved
}

// StripMentions removes $skill-name tokens from user input text,
// returning the cleaned text for sending to the LLM.
func StripMentions(text string) string {
	return strings.TrimSpace(mentionPattern.ReplaceAllString(text, ""))
}
