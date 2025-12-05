package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/dagger/dagger/dagql"
	"github.com/dagger/dagger/engine/cache"
)

// Helper function to set up a test LLM router with GitHub Copilot configuration
func setupGitHubCopilotRouter(t *testing.T, vars map[string]string) *LLMRouter {
	q := LLMTestQuery{}

	baseCache, err := cache.NewCache[string, dagql.AnyResult](context.Background(), "")
	assert.NoError(t, err)
	srv := dagql.NewServer(q, dagql.NewSessionCache(baseCache))

	dagql.Fields[LLMTestQuery]{
		dagql.Func("secret", func(ctx context.Context, self LLMTestQuery, args struct {
			URI string
		}) (mockSecret, error) {
			if _, ok := vars[args.URI]; !ok {
				t.Fatalf("uri not found: %s", args.URI)
			}
			return mockSecret{uri: args.URI}, nil
		}),
	}.Install(srv)

	dagql.Fields[mockSecret]{
		dagql.Func("plaintext", func(ctx context.Context, self mockSecret, _ struct{}) (string, error) {
			return vars[self.uri], nil
		}),
	}.Install(srv)

	ctx := context.Background()
	r, err := NewLLMRouter(ctx, srv)
	assert.NoError(t, err)
	return r
}

func TestGithubCopilotConfig(t *testing.T) {
	vars := map[string]string{
		"file://.env":                    "",
		"env://ANTHROPIC_API_KEY":        "",
		"env://ANTHROPIC_BASE_URL":       "",
		"env://ANTHROPIC_MODEL":          "",
		"env://OPENAI_API_KEY":           "",
		"env://OPENAI_AZURE_VERSION":     "",
		"env://OPENAI_BASE_URL":          "",
		"env://OPENAI_MODEL":             "",
		"env://OPENAI_DISABLE_STREAMING": "",
		"env://GEMINI_API_KEY":           "",
		"env://GEMINI_BASE_URL":          "",
		"env://GEMINI_MODEL":             "",
		"env://GITHUB_TOKEN":             "ghp_test_token_123",
		"env://GITHUB_MODEL":             "gpt-4o",
		"env://GITHUB_CLI_VERSION":       "1.2.3",
	}

	r := setupGitHubCopilotRouter(t, vars)

	// Verify GitHub Copilot configuration was loaded
	assert.Equal(t, "ghp_test_token_123", r.GitHubToken)
	assert.Equal(t, "gpt-4o", r.GitHubModel)
	assert.Equal(t, "1.2.3", r.GitHubCliVersion)
}

func TestGithubCopilotConfigDefaults(t *testing.T) {
	vars := map[string]string{
		"file://.env":                    "",
		"env://ANTHROPIC_API_KEY":        "",
		"env://ANTHROPIC_BASE_URL":       "",
		"env://ANTHROPIC_MODEL":          "",
		"env://OPENAI_API_KEY":           "",
		"env://OPENAI_AZURE_VERSION":     "",
		"env://OPENAI_BASE_URL":          "",
		"env://OPENAI_MODEL":             "",
		"env://OPENAI_DISABLE_STREAMING": "",
		"env://GEMINI_API_KEY":           "",
		"env://GEMINI_BASE_URL":          "",
		"env://GEMINI_MODEL":             "",
		"env://GITHUB_TOKEN":             "ghp_test_token_456",
		"env://GITHUB_MODEL":             "",
		"env://GITHUB_CLI_VERSION":       "",
	}

	r := setupGitHubCopilotRouter(t, vars)

	// Verify defaults are set when config is not provided
	assert.Equal(t, "ghp_test_token_456", r.GitHubToken)
	assert.Equal(t, "", r.GitHubModel)            // No default model
	assert.Equal(t, "latest", r.GitHubCliVersion) // Defaults to "latest"
}

func TestGithubCopilotConfigEnvFile(t *testing.T) {
	vars := map[string]string{
		"file://.env": `GITHUB_TOKEN=ghp_env_file_token
GITHUB_MODEL=o1-preview
GITHUB_CLI_VERSION=2.0.0`,
		"env://ANTHROPIC_API_KEY":        "",
		"env://ANTHROPIC_BASE_URL":       "",
		"env://ANTHROPIC_MODEL":          "",
		"env://OPENAI_API_KEY":           "",
		"env://OPENAI_AZURE_VERSION":     "",
		"env://OPENAI_BASE_URL":          "",
		"env://OPENAI_MODEL":             "",
		"env://OPENAI_DISABLE_STREAMING": "",
		"env://GEMINI_API_KEY":           "",
		"env://GEMINI_BASE_URL":          "",
		"env://GEMINI_MODEL":             "",
		"env://GITHUB_TOKEN":             "",
		"env://GITHUB_MODEL":             "",
		"env://GITHUB_CLI_VERSION":       "",
	}

	r := setupGitHubCopilotRouter(t, vars)

	// Verify config loaded from .env file
	assert.Equal(t, "ghp_env_file_token", r.GitHubToken)
	assert.Equal(t, "o1-preview", r.GitHubModel)
	assert.Equal(t, "2.0.0", r.GitHubCliVersion)
}

// TestGitHubModelRoutingDetection verifies that the router's model detection logic works correctly
// (Note: Full endpoint creation testing requires integration tests with running Dagger engine)
func TestGitHubModelRoutingDetection(t *testing.T) {
	vars := map[string]string{
		"file://.env":                    "",
		"env://ANTHROPIC_API_KEY":        "dummy",
		"env://ANTHROPIC_BASE_URL":       "",
		"env://ANTHROPIC_MODEL":          "",
		"env://OPENAI_API_KEY":           "dummy",
		"env://OPENAI_AZURE_VERSION":     "",
		"env://OPENAI_BASE_URL":          "",
		"env://OPENAI_MODEL":             "",
		"env://OPENAI_DISABLE_STREAMING": "",
		"env://GEMINI_API_KEY":           "dummy",
		"env://GEMINI_BASE_URL":          "",
		"env://GEMINI_MODEL":             "",
		"env://GITHUB_TOKEN":             "ghp_test_token",
		"env://GITHUB_MODEL":             "gpt-4o",
		"env://GITHUB_CLI_VERSION":       "latest",
	}

	r := setupGitHubCopilotRouter(t, vars)

	// Test that model detection correctly identifies GitHub models
	testCases := []struct {
		model        string
		expectedProv LLMProvider
	}{
		{"github-gpt-4o", GitHub},
		{"github/gpt-5", GitHub},
		{"gpt-4", OpenAI},
		{"claude-3.5-sonnet", Anthropic},
		{"gemini-2.5-flash", Google},
	}

	for _, tc := range testCases {
		t.Run(tc.model, func(t *testing.T) {
			endpoint, err := r.Route(tc.model)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedProv, endpoint.Provider)
		})
	}
}

// TestGitHubConfigNotSet verifies behavior when GITHUB_TOKEN is not configured
func TestGitHubConfigNotSet(t *testing.T) {
	vars := map[string]string{
		"file://.env":                    "",
		"env://ANTHROPIC_API_KEY":        "",
		"env://ANTHROPIC_BASE_URL":       "",
		"env://ANTHROPIC_MODEL":          "",
		"env://OPENAI_API_KEY":           "",
		"env://OPENAI_AZURE_VERSION":     "",
		"env://OPENAI_BASE_URL":          "",
		"env://OPENAI_MODEL":             "",
		"env://OPENAI_DISABLE_STREAMING": "",
		"env://GEMINI_API_KEY":           "",
		"env://GEMINI_BASE_URL":          "",
		"env://GEMINI_MODEL":             "",
		"env://GITHUB_TOKEN":             "", // Not set
		"env://GITHUB_MODEL":             "",
		"env://GITHUB_CLI_VERSION":       "",
	}

	r := setupGitHubCopilotRouter(t, vars)

	// Verify token is empty when not configured
	assert.Equal(t, "", r.GitHubToken)
	assert.Equal(t, "latest", r.GitHubCliVersion) // Still defaults to "latest"
}

// TestGitHubModelAlias verifies that the "github" alias resolves to the default GitHub model
func TestGitHubModelAlias(t *testing.T) {
	model := resolveModelAlias("github")
	assert.Equal(t, modelDefaultGitHub, model)
}

// TestGitHubTokenConfigurationTypes tests that the router can receive GitHub token through different methods
func TestGitHubTokenConfigurationTypes(t *testing.T) {
	testCases := []struct {
		name  string
		token string
	}{
		{"standard token", "ghp_abc123def456"},
		{"token with long string", "ghp_" + string(make([]byte, 32))},
		{"empty token", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			vars := map[string]string{
				"file://.env":                    "",
				"env://ANTHROPIC_API_KEY":        "",
				"env://ANTHROPIC_BASE_URL":       "",
				"env://ANTHROPIC_MODEL":          "",
				"env://OPENAI_API_KEY":           "",
				"env://OPENAI_AZURE_VERSION":     "",
				"env://OPENAI_BASE_URL":          "",
				"env://OPENAI_MODEL":             "",
				"env://OPENAI_DISABLE_STREAMING": "",
				"env://GEMINI_API_KEY":           "",
				"env://GEMINI_BASE_URL":          "",
				"env://GEMINI_MODEL":             "",
				"env://GITHUB_TOKEN":             tc.token,
				"env://GITHUB_MODEL":             "",
				"env://GITHUB_CLI_VERSION":       "",
			}

			r := setupGitHubCopilotRouter(t, vars)
			assert.Equal(t, tc.token, r.GitHubToken)
		})
	}
}
