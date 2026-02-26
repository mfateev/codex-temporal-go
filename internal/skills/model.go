package skills

// SkillScope indicates where a skill was discovered.
// Maps to: codex-rs/core/src/skills/model.rs SkillScope
type SkillScope string

const (
	ScopeRepo   SkillScope = "repo"   // .agents/skills in project hierarchy
	ScopeUser   SkillScope = "user"   // ~/.codex/skills or ~/.agents/skills
	ScopeSystem SkillScope = "system" // $CODEX_HOME/skills/.system
	ScopeAdmin  SkillScope = "admin"  // /etc/codex/skills
)

// SkillMetadata holds parsed metadata from a SKILL.md file.
// Maps to: codex-rs/core/src/skills/model.rs SkillMetadata
type SkillMetadata struct {
	Name             string     `json:"name"`
	Description      string     `json:"description"`
	ShortDescription string     `json:"short_description,omitempty"`
	Path             string     `json:"path"`  // Absolute path to the SKILL.md file
	Scope            SkillScope `json:"scope"`
}

// SkillContent holds a skill's full content (metadata + markdown body).
type SkillContent struct {
	SkillMetadata
	Body string `json:"body"` // Full markdown content after frontmatter
}
