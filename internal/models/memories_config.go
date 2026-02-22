package models

// MemoriesConfig holds user-configurable memory settings.
// Maps to: codex-rs/core/src/config/types.rs MemoriesConfig
type MemoriesConfig struct {
	// MaxRawMemoriesForGlobal is the max stage-1 outputs fed to phase-2.
	// Default 1024, clamped to 4096.
	MaxRawMemoriesForGlobal int `json:"max_raw_memories_for_global,omitempty"`

	// MaxRolloutAgeDays limits how old a rollout can be to participate.
	// Default 30, clamped 0-90.
	MaxRolloutAgeDays int `json:"max_rollout_age_days,omitempty"`

	// MinRolloutIdleHours is the minimum idle time before extraction.
	// Default 12, clamped 1-48.
	MinRolloutIdleHours int `json:"min_rollout_idle_hours,omitempty"`

	// MaxRolloutsPerStartup limits per-startup extractions. Default 8, max 128.
	MaxRolloutsPerStartup int `json:"max_rollouts_per_startup,omitempty"`

	// Phase1Model overrides the model for phase-1 extraction.
	Phase1Model string `json:"phase1_model,omitempty"`

	// Phase2Model overrides the model for phase-2 consolidation.
	Phase2Model string `json:"phase2_model,omitempty"`
}

// DefaultMemoriesConfig returns config with sensible defaults.
func DefaultMemoriesConfig() MemoriesConfig {
	return MemoriesConfig{
		MaxRawMemoriesForGlobal: 1024,
		MaxRolloutAgeDays:       30,
		MinRolloutIdleHours:     12,
		MaxRolloutsPerStartup:   8,
	}
}

// Clamp enforces limits on user-provided values.
func (c *MemoriesConfig) Clamp() {
	if c.MaxRawMemoriesForGlobal <= 0 {
		c.MaxRawMemoriesForGlobal = 1024
	}
	if c.MaxRawMemoriesForGlobal > 4096 {
		c.MaxRawMemoriesForGlobal = 4096
	}
	if c.MaxRolloutAgeDays <= 0 {
		c.MaxRolloutAgeDays = 30
	}
	if c.MaxRolloutAgeDays > 90 {
		c.MaxRolloutAgeDays = 90
	}
	if c.MinRolloutIdleHours <= 0 {
		c.MinRolloutIdleHours = 12
	}
	if c.MinRolloutIdleHours < 1 {
		c.MinRolloutIdleHours = 1
	}
	if c.MinRolloutIdleHours > 48 {
		c.MinRolloutIdleHours = 48
	}
	if c.MaxRolloutsPerStartup <= 0 {
		c.MaxRolloutsPerStartup = 8
	}
	if c.MaxRolloutsPerStartup > 128 {
		c.MaxRolloutsPerStartup = 128
	}
}
