package core

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"dagger.io/dagger"
	"dagger.io/dagger/dag"
	"dagger.io/dagger/telemetry"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/trace"
)

type GhcpClient struct {
	client   *dagger.Container
	endpoint *LLMEndpoint
}

func newGhcpClient(endpoint *LLMEndpoint) *GhcpClient {
	ctx := context.Background()

	var container = GhcpContainer(ctx, endpoint.Key)

	return &GhcpClient{
		client:   container,
		endpoint: endpoint,
	}
}

var _ LLMClient = (*GhcpClient)(nil)

func GhcpContainer(
	ctx context.Context,
	token string,
) *dagger.Container {
	return dag.Container().
		From("node:24-bookworm-slim").
		WithExec([]string{"npm", "install", "-g", "@github/copilot"}).
		WithEnvVariable("GITHUB_TOKEN", token).
		WithWorkdir("/workspace")
}

// Satisfy the LLMClient interface with SendQuery and IsRetryable
func (c *GhcpClient) SendQuery(ctx context.Context, history []*ModelMessage, tools []LLMTool) (_ *LLMResponse, rerr error) {

	// instrument the call with telemetry
	// todo: moving to setup function to clean this up
	stdio := telemetry.SpanStdio(ctx, InstrumentationLibrary,
		log.String(telemetry.ContentTypeAttr, "text/markdown"))
	defer stdio.Close()

	m := telemetry.Meter(ctx, InstrumentationLibrary)
	spanCtx := trace.SpanContextFromContext(ctx)
	// end instrument the call with telemetry

	var client = c.client

	var toolCalls []LLMToolCall

	content, err := client.Stdout(ctx)
	if err != nil {
		return nil, err
	}

	ghcpResponseMetadata, err := client.Stderr(ctx)
	if err != nil {
		return nil, err
	}

	client.WithEnvVariable("GITHUB_TOKEN", c.endpoint.Key)

	llmTokenUsage := parseLLMTokenUsage(ghcpResponseMetadata)

	return &LLMResponse{
		Content:    content,
		ToolCalls:  toolCalls,
		TokenUsage: llmTokenUsage,
	}, nil
}

// We're not implementing any retries at the moment
func (c *GhcpClient) IsRetryable(err error) bool {
	// There is no auto retry at GHCP CLI That I know of at the moment
	return false
}

func processTelemetryAttributes(ctx context.Context, endpoint LLMEndpoint) ([]attribute.KeyValue, error) {

	attrs := []attribute.KeyValue{
		attribute.String(telemetry.MetricsTraceIDAttr, spanCtx.TraceID().String()),
		attribute.String(telemetry.MetricsSpanIDAttr, spanCtx.SpanID().String()),
		attribute.String("model", c.endpoint.Model),
		attribute.String("provider", string(c.endpoint.Provider)),
	}

	return attrs, nil
}

// parseLLMTokenUsage parses the stderr output from GitHub Copilot CLI to extract token usage information
func parseLLMTokenUsage(output string) LLMTokenUsage {
	var tokenUsage LLMTokenUsage

	// Parse the usage line that contains model-specific token information
	// Example: "claude-sonnet-4.5    7.5k input, 52 output, 3.6k cache read (Est. 1 Premium request)"

	// Look for the pattern: model name followed by input, output, cache read values
	re := regexp.MustCompile(`(\d+(?:\.\d+)?)(k?)\s+input,\s*(\d+(?:\.\d+)?)(k?)\s+output,\s*(\d+(?:\.\d+)?)(k?)\s+cache read,\s*(\d+(?:\.\d+)?)(k?)\s+cache write`)
	matches := re.FindStringSubmatch(output)

	if len(matches) >= 7 {
		// Parse input tokens
		if inputVal, err := strconv.ParseFloat(matches[1], 64); err == nil {
			if strings.ToLower(matches[2]) == "k" {
				inputVal *= 1000
			}
			tokenUsage.InputTokens = int64(inputVal)
		}

		// Parse output tokens
		if outputVal, err := strconv.ParseFloat(matches[3], 64); err == nil {
			if strings.ToLower(matches[4]) == "k" {
				outputVal *= 1000
			}
			tokenUsage.OutputTokens = int64(outputVal)
		}

		// Parse cache read tokens
		if cacheVal, err := strconv.ParseFloat(matches[5], 64); err == nil {
			if strings.ToLower(matches[6]) == "k" {
				cacheVal *= 1000
			}
			tokenUsage.CachedTokenReads = int64(cacheVal)
		}

		// Parse cache write tokens
		if cacheVal, err := strconv.ParseFloat(matches[7], 64); err == nil {
			if strings.ToLower(matches[8]) == "k" {
				cacheVal *= 1000
			}
			tokenUsage.CachedTokenWrites = int64(cacheVal)
		}

		tokenUsage.TotalTokens = tokenUsage.InputTokens + tokenUsage.OutputTokens
	}

	return tokenUsage
}
