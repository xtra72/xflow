package agent

import (
	"log/slog"
	"os"
	"testing"
)

func TestResolveLogger_NilFallsBackToDefault(t *testing.T) {
	config := AgentConfig{}
	logger := ResolveLogger(config)
	if logger == nil {
		t.Fatal("ResolveLogger must not return nil")
	}
	if logger != slog.Default() {
		t.Error("expected slog.Default() when config.Logger is nil")
	}
}

func TestResolveLogger_UsesInjectedLogger(t *testing.T) {
	injected := slog.New(slog.NewTextHandler(os.Stderr, nil))
	config := AgentConfig{Logger: injected}
	logger := ResolveLogger(config)
	if logger != injected {
		t.Error("expected the injected logger to be returned")
	}
}
