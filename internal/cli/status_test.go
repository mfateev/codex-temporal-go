package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mfateev/temporal-agent-harness/internal/models"
)

func TestFormatStatusDisplay_Basic(t *testing.T) {
	m := &Model{
		modelName: "gpt-4o",
		provider:  "openai",
		config: Config{
			Cwd:         "/tmp/test",
			Permissions: models.Permissions{ApprovalMode: "on-failure"},
		},
		workflowID: "test-workflow-123",
	}

	result := m.formatStatusDisplay()
	assert.Contains(t, result, "gpt-4o")
	assert.Contains(t, result, "openai")
	assert.Contains(t, result, "on-failure")
	assert.Contains(t, result, "/tmp/test")
	assert.Contains(t, result, "test-workflow-123")
}

func TestFormatStatusDisplay_CachedTokensShown(t *testing.T) {
	m := &Model{
		modelName:         "gpt-4o",
		provider:          "openai",
		totalTokens:       1000,
		totalCachedTokens: 500,
		config:            Config{Permissions: models.Permissions{}},
	}

	result := m.formatStatusDisplay()
	assert.Contains(t, result, "1000")
	assert.Contains(t, result, "500 cached")
}

func TestFormatStatusDisplay_CachedTokensHidden(t *testing.T) {
	m := &Model{
		modelName:         "gpt-4o",
		provider:          "openai",
		totalTokens:       1000,
		totalCachedTokens: 0,
		config:            Config{Permissions: models.Permissions{}},
	}

	result := m.formatStatusDisplay()
	assert.Contains(t, result, "1000")
	assert.False(t, strings.Contains(result, "cached"))
}

func TestFormatStatusDisplay_PlannerActive(t *testing.T) {
	m := &Model{
		modelName:     "gpt-4o",
		provider:      "openai",
		plannerActive: true,
		config:        Config{Permissions: models.Permissions{}},
	}

	result := m.formatStatusDisplay()
	assert.Contains(t, result, "Plan mode")
	assert.Contains(t, result, "active")
}
