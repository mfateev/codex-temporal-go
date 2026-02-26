package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFrontmatter_Valid(t *testing.T) {
	content := "---\nname: test-skill\ndescription: A test skill\nmetadata:\n  short-description: Short desc\n---\n\n# Body\n\nContent here."
	fm, body, err := parseFrontmatter(content)
	require.NoError(t, err)
	assert.Equal(t, "test-skill", fm.Name)
	assert.Equal(t, "A test skill", fm.Description)
	assert.Equal(t, "Short desc", fm.Metadata.ShortDescription)
	assert.Equal(t, "# Body\n\nContent here.", body)
}

func TestParseFrontmatter_MissingOpening(t *testing.T) {
	_, _, err := parseFrontmatter("name: test\n---\nbody")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing opening")
}

func TestParseFrontmatter_MissingClosing(t *testing.T) {
	_, _, err := parseFrontmatter("---\nname: test\nbody without closing")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing closing")
}

func TestParseSkillFile_Valid(t *testing.T) {
	dir := t.TempDir()
	skillPath := filepath.Join(dir, "SKILL.md")
	content := "---\nname: my-skill\ndescription: Does things\n---\n\n# My Skill\n\nDoes cool things."
	require.NoError(t, os.WriteFile(skillPath, []byte(content), 0644))

	meta, err := parseSkillFile(skillPath, ScopeUser)
	require.NoError(t, err)
	assert.Equal(t, "my-skill", meta.Name)
	assert.Equal(t, "Does things", meta.Description)
	assert.Equal(t, ScopeUser, meta.Scope)
	assert.Contains(t, meta.Path, "SKILL.md")
}

func TestParseSkillFile_MissingName(t *testing.T) {
	dir := t.TempDir()
	skillPath := filepath.Join(dir, "SKILL.md")
	content := "---\ndescription: No name\n---\n\nBody"
	require.NoError(t, os.WriteFile(skillPath, []byte(content), 0644))

	_, err := parseSkillFile(skillPath, ScopeUser)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing name")
}

func TestParseSkillFile_MissingDescription(t *testing.T) {
	dir := t.TempDir()
	skillPath := filepath.Join(dir, "SKILL.md")
	content := "---\nname: test\n---\n\nBody"
	require.NoError(t, os.WriteFile(skillPath, []byte(content), 0644))

	_, err := parseSkillFile(skillPath, ScopeUser)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing description")
}

func TestDiscoverSkills_FindsSkills(t *testing.T) {
	// Create a temp codex home with skills
	codexHome := t.TempDir()
	skillDir := filepath.Join(codexHome, "skills", "my-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0755))

	content := "---\nname: my-skill\ndescription: A discovered skill\n---\n\n# My Skill\n\nContent."
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644))

	skills, err := DiscoverSkills(codexHome, "")
	require.NoError(t, err)
	assert.Len(t, skills, 1)
	assert.Equal(t, "my-skill", skills[0].Name)
	assert.Equal(t, ScopeUser, skills[0].Scope)
}

func TestDiscoverSkills_RepoScope(t *testing.T) {
	// Create a temp cwd with .agents/skills
	cwd := t.TempDir()
	skillDir := filepath.Join(cwd, ".agents", "skills", "repo-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0755))

	content := "---\nname: repo-skill\ndescription: A repo skill\n---\n\nBody"
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644))

	skills, err := DiscoverSkills("", cwd)
	require.NoError(t, err)
	assert.Len(t, skills, 1)
	assert.Equal(t, "repo-skill", skills[0].Name)
	assert.Equal(t, ScopeRepo, skills[0].Scope)
}

func TestDiscoverSkills_Empty(t *testing.T) {
	codexHome := t.TempDir()
	skills, err := DiscoverSkills(codexHome, "")
	require.NoError(t, err)
	assert.Empty(t, skills)
}

func TestDiscoverSkills_Deduplicates(t *testing.T) {
	// Same skill path shouldn't appear twice
	codexHome := t.TempDir()
	skillDir := filepath.Join(codexHome, "skills", "dup-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0755))

	content := "---\nname: dup-skill\ndescription: Dup\n---\n\nBody"
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644))

	skills, err := DiscoverSkills(codexHome, "")
	require.NoError(t, err)
	assert.Len(t, skills, 1)
}

func TestLoadSkillContent_FullContent(t *testing.T) {
	dir := t.TempDir()
	skillPath := filepath.Join(dir, "SKILL.md")
	content := "---\nname: content-skill\ndescription: Has content\n---\n\n# Instructions\n\nDo the thing."
	require.NoError(t, os.WriteFile(skillPath, []byte(content), 0644))

	sc, err := LoadSkillContent(skillPath)
	require.NoError(t, err)
	assert.Equal(t, "content-skill", sc.Name)
	assert.Equal(t, "Has content", sc.Description)
	assert.Equal(t, "# Instructions\n\nDo the thing.", sc.Body)
}

func TestLoadSkillContent_NotFound(t *testing.T) {
	_, err := LoadSkillContent("/nonexistent/SKILL.md")
	assert.Error(t, err)
}

func TestDiscoverSkills_SortedByScopeThenName(t *testing.T) {
	codexHome := t.TempDir()

	// Create user skill "beta"
	betaDir := filepath.Join(codexHome, "skills", "beta")
	require.NoError(t, os.MkdirAll(betaDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(betaDir, "SKILL.md"),
		[]byte("---\nname: beta\ndescription: B\n---\n\nB"), 0644))

	// Create user skill "alpha"
	alphaDir := filepath.Join(codexHome, "skills", "alpha")
	require.NoError(t, os.MkdirAll(alphaDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(alphaDir, "SKILL.md"),
		[]byte("---\nname: alpha\ndescription: A\n---\n\nA"), 0644))

	skills, err := DiscoverSkills(codexHome, "")
	require.NoError(t, err)
	require.Len(t, skills, 2)
	assert.Equal(t, "alpha", skills[0].Name)
	assert.Equal(t, "beta", skills[1].Name)
}
