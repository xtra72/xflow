package system

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// newTestAgentWithTargetFiles creates a FileAgentImpl with target_files config.
func newTestAgentWithTargetFiles(t *testing.T) (*FileAgentImpl, string) {
	t.Helper()
	sandbox := t.TempDir()

	// Create target files/directories.
	dataDir := filepath.Join(sandbox, "data")
	require.NoError(t, os.MkdirAll(dataDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "config.json"), []byte(`{}`), 0644))

	fa := NewFileAgent(sandbox)
	cfg := agent.AgentConfig{
		ID:   "file-config-test",
		Name: "file-config-test-agent",
		Type: string(agent.SystemAgentFile),
		Metadata: map[string]string{
			"mode":         "text",
			"target_files": `{"config": "` + filepath.Join(sandbox, "data", "config.json") + `"}`,
		},
	}
	require.NoError(t, fa.Init(cfg))
	t.Cleanup(func() {
		_ = fa.Stop(context.Background())
	})
	return fa, sandbox
}

// ---------------------------------------------------------------------------
// resolveAlias
// ---------------------------------------------------------------------------

func TestResolveAlias_Found(t *testing.T) {
	fa, sandbox := newTestAgentWithTargetFiles(t)

	path, err := fa.resolveAlias("config")
	require.NoError(t, err)
	assert.Contains(t, path, filepath.Join(sandbox, "data", "config.json"))
}

func TestResolveAlias_NotFound(t *testing.T) {
	fa, _ := newTestAgentWithTargetFiles(t)

	_, err := fa.resolveAlias("nonexistent")
	assert.ErrorIs(t, err, ErrAliasNotFound)
}

// ---------------------------------------------------------------------------
// resolvePath
// ---------------------------------------------------------------------------

func TestResolvePath_WithAlias(t *testing.T) {
	fa, _ := newTestAgentWithTargetFiles(t)

	path, err := fa.resolvePath(fileCommandParams{Alias: "config"})
	require.NoError(t, err)
	assert.NotEmpty(t, path)
}

func TestResolvePath_WithDirectPath(t *testing.T) {
	fa, sandbox := newTestAgentWithTargetFiles(t)

	path, err := fa.resolvePath(fileCommandParams{Path: filepath.Join(sandbox, "data", "config.json")})
	require.NoError(t, err)
	assert.Contains(t, path, "config.json")
}

func TestResolvePath_NoAliasNoPath(t *testing.T) {
	fa, _ := newTestAgentWithTargetFiles(t)

	_, err := fa.resolvePath(fileCommandParams{})
	assert.ErrorIs(t, err, ErrInvalidCommandFormat)
}

func TestResolvePath_AliasNotFound(t *testing.T) {
	fa, _ := newTestAgentWithTargetFiles(t)

	_, err := fa.resolvePath(fileCommandParams{Alias: "nope"})
	assert.ErrorIs(t, err, ErrAliasNotFound)
}

func TestResolvePath_DirectPathOutsideSandbox(t *testing.T) {
	fa, _ := newTestAgentWithTargetFiles(t)

	_, err := fa.resolvePath(fileCommandParams{Path: "/etc/passwd"})
	assert.ErrorIs(t, err, ErrPathOutsideSandbox)
}

// ---------------------------------------------------------------------------
// Init target_files outside sandbox
// ---------------------------------------------------------------------------

func TestInit_TargetFileOutsideSandbox(t *testing.T) {
	sandbox := t.TempDir()
	fa := NewFileAgent(sandbox)
	cfg := agent.AgentConfig{
		ID:   "file-config-fail",
		Name: "file-config-fail-agent",
		Type: string(agent.SystemAgentFile),
		Metadata: map[string]string{
			"target_files": `{"evil": "/etc/passwd"}`,
		},
	}
	err := fa.Init(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "target file")
}

// ---------------------------------------------------------------------------
// Init with watch_events filter
// ---------------------------------------------------------------------------

func TestInit_WatchEventsFilter(t *testing.T) {
	sandbox := t.TempDir()
	fa := NewFileAgent(sandbox)
	cfg := agent.AgentConfig{
		ID:   "file-events-test",
		Name: "file-events-test-agent",
		Type: string(agent.SystemAgentFile),
		Metadata: map[string]string{
			"watch_events": `["create","write"]`,
		},
	}
	require.NoError(t, fa.Init(cfg))
	t.Cleanup(func() {
		_ = fa.Stop(context.Background())
	})

	assert.Equal(t, []string{"create", "write"}, fa.watchEvents)
}

// ---------------------------------------------------------------------------
// shouldEmitEvent
// ---------------------------------------------------------------------------

func TestShouldEmitEvent(t *testing.T) {
	tests := []struct {
		name        string
		watchEvents []string
		op          FileOp
		want        bool
	}{
		{"nil filter allows all", nil, FileOpCreate, true},
		{"empty filter allows all", []string{}, FileOpWrite, true},
		{"match create", []string{"create", "write"}, FileOpCreate, true},
		{"match write", []string{"create", "write"}, FileOpWrite, true},
		{"no match remove", []string{"create", "write"}, FileOpRemove, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fa := &FileAgentImpl{watchEvents: tc.watchEvents}
			assert.Equal(t, tc.want, fa.shouldEmitEvent(tc.op))
		})
	}
}

// ---------------------------------------------------------------------------
// makeRelativePath
// ---------------------------------------------------------------------------

func TestMakeRelativePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		root string
		want string
	}{
		{"simple relative", "/sandbox/data/file.txt", "/sandbox", "data/file.txt"},
		{"same dir", "/sandbox/file.txt", "/sandbox", "file.txt"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := makeRelativePath(tc.path, tc.root)
			assert.Equal(t, tc.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// Init with event_buffer_size
// ---------------------------------------------------------------------------

func TestInit_EventBufferSize(t *testing.T) {
	sandbox := t.TempDir()
	fa := NewFileAgent(sandbox)
	cfg := agent.AgentConfig{
		ID:   "file-buf-test",
		Name: "file-buf-test-agent",
		Type: string(agent.SystemAgentFile),
		Metadata: map[string]string{
			"event_buffer_size": "64",
		},
	}
	require.NoError(t, fa.Init(cfg))
	t.Cleanup(func() {
		_ = fa.Stop(context.Background())
	})

	assert.Equal(t, 64, cap(fa.eventCh))
}

func TestInit_DefaultBufferSize(t *testing.T) {
	sandbox := t.TempDir()
	fa := NewFileAgent(sandbox)
	cfg := agent.AgentConfig{
		ID:   "file-default-buf",
		Name: "file-default-buf-agent",
		Type: string(agent.SystemAgentFile),
	}
	require.NoError(t, fa.Init(cfg))
	t.Cleanup(func() {
		_ = fa.Stop(context.Background())
	})

	assert.Equal(t, 256, cap(fa.eventCh))
}
