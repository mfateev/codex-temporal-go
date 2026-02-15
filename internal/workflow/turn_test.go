package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mfateev/temporal-agent-harness/internal/models"
)

// TestEffectiveAutoCompactLimit_ClampedToContextWindow verifies that when the
// configured limit exceeds 90% of context window, it is clamped.
func TestEffectiveAutoCompactLimit_ClampedToContextWindow(t *testing.T) {
	s := &SessionState{
		Config: models.SessionConfiguration{
			AutoCompactTokenLimit: 200000,
			Model: models.ModelConfig{
				ContextWindow: 128000,
			},
		},
	}
	// 90% of 128000 = 115200
	assert.Equal(t, 115200, s.effectiveAutoCompactLimit())
}

// TestEffectiveAutoCompactLimit_ConfiguredLower verifies that when the configured
// limit is below the context window clamp, it is used as-is.
func TestEffectiveAutoCompactLimit_ConfiguredLower(t *testing.T) {
	s := &SessionState{
		Config: models.SessionConfiguration{
			AutoCompactTokenLimit: 80000,
			Model: models.ModelConfig{
				ContextWindow: 128000,
			},
		},
	}
	assert.Equal(t, 80000, s.effectiveAutoCompactLimit())
}

// TestEffectiveAutoCompactLimit_Disabled verifies that when auto-compact is
// disabled (limit=0), the method returns 0.
func TestEffectiveAutoCompactLimit_Disabled(t *testing.T) {
	s := &SessionState{
		Config: models.SessionConfiguration{
			AutoCompactTokenLimit: 0,
			Model: models.ModelConfig{
				ContextWindow: 128000,
			},
		},
	}
	assert.Equal(t, 0, s.effectiveAutoCompactLimit())
}

// TestEffectiveAutoCompactLimit_NoContextWindow verifies that when context
// window is 0 (unknown), the configured limit is used as-is.
func TestEffectiveAutoCompactLimit_NoContextWindow(t *testing.T) {
	s := &SessionState{
		Config: models.SessionConfiguration{
			AutoCompactTokenLimit: 100000,
			Model: models.ModelConfig{
				ContextWindow: 0,
			},
		},
	}
	assert.Equal(t, 100000, s.effectiveAutoCompactLimit())
}
