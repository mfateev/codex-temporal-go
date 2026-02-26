package skills

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseMentions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"empty", "", nil},
		{"no mentions", "hello world", nil},
		{"single mention", "use $repo-scout to find files", []string{"repo-scout"}},
		{"multiple mentions", "$foo and $bar", []string{"foo", "bar"}},
		{"duplicate mentions", "$foo $bar $foo", []string{"foo", "bar"}},
		{"underscore in name", "$my_skill", []string{"my_skill"}},
		{"dollar sign alone", "costs $5", []string{"5"}},
		{"mention at start", "$skill hello", []string{"skill"}},
		{"mention at end", "use $skill", []string{"skill"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseMentions(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveMentions(t *testing.T) {
	loaded := []SkillMetadata{
		{Name: "repo-scout", Path: "/skills/repo-scout/SKILL.md", Scope: ScopeUser},
		{Name: "skill-installer", Path: "/skills/skill-installer/SKILL.md", Scope: ScopeSystem},
		{Name: "disabled-skill", Path: "/skills/disabled/SKILL.md", Scope: ScopeUser},
	}
	disabled := []string{"/skills/disabled/SKILL.md"}

	tests := []struct {
		name     string
		mentions []string
		expected int
	}{
		{"no mentions", nil, 0},
		{"match by name", []string{"repo-scout"}, 1},
		{"case insensitive", []string{"Repo-Scout"}, 1},
		{"no match", []string{"nonexistent"}, 0},
		{"disabled skill excluded", []string{"disabled-skill"}, 0},
		{"multiple matches", []string{"repo-scout", "skill-installer"}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveMentions(tt.mentions, loaded, disabled)
			assert.Len(t, result, tt.expected)
		})
	}
}

func TestStripMentions(t *testing.T) {
	assert.Equal(t, "use  to find files", StripMentions("use $repo-scout to find files"))
	assert.Equal(t, "hello world", StripMentions("hello world"))
	assert.Equal(t, "and", StripMentions("$foo and $bar"))
}
