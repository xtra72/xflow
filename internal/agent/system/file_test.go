package system

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

// newTestFileAgent creates an initialized FileAgentImpl with a temp sandbox.
func newTestFileAgent(t *testing.T) (*FileAgentImpl, string) {
	t.Helper()
	sandbox := t.TempDir()
	fa := NewFileAgent(sandbox)
	cfg := agent.AgentConfig{
		ID:   "file-test",
		Name: "file-test-agent",
		Type: string(agent.SystemAgentFile),
	}
	require.NoError(t, fa.Init(cfg))
	t.Cleanup(func() {
		_ = fa.Stop(context.Background())
	})
	return fa, sandbox
}

// ---------------------------------------------------------------------------
// Construction tests
// ---------------------------------------------------------------------------

func TestFileAgent_NewFileAgent(t *testing.T) {
	fa := NewFileAgent("/tmp/sandbox")
	assert.NotNil(t, fa)
	assert.Equal(t, lifecycle.StateCreated, fa.CurrentState())
}

func TestFileAgent_NewFileAgent_EmptySandbox(t *testing.T) {
	fa := NewFileAgent("")
	assert.NotNil(t, fa)
}

// ---------------------------------------------------------------------------
// SystemAgent interface compliance
// ---------------------------------------------------------------------------

func TestFileAgent_IsSystem(t *testing.T) {
	fa := NewFileAgent("/tmp")
	assert.True(t, fa.IsSystem())
}

func TestFileAgent_RequiresTransport(t *testing.T) {
	fa := NewFileAgent("/tmp")
	assert.False(t, fa.RequiresTransport())
}

// ---------------------------------------------------------------------------
// Lifecycle tests
// ---------------------------------------------------------------------------

func TestFileAgent_Init_TransitionsToRunning(t *testing.T) {
	sandbox := t.TempDir()
	fa := NewFileAgent(sandbox)
	cfg := agent.AgentConfig{
		ID:   "file-init",
		Name: "file-init-agent",
		Type: string(agent.SystemAgentFile),
	}
	err := fa.Init(cfg)
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, fa.CurrentState())
	_ = fa.Stop(context.Background())
}

func TestFileAgent_Init_EmptySandbox(t *testing.T) {
	fa := NewFileAgent("")
	cfg := agent.AgentConfig{
		ID:   "file-no-sandbox",
		Name: "file-no-sandbox-agent",
		Type: string(agent.SystemAgentFile),
	}
	err := fa.Init(cfg)
	assert.ErrorIs(t, err, ErrSandboxNotConfigured)
}

func TestFileAgent_Stop_TransitionsToStopped(t *testing.T) {
	sandbox := t.TempDir()
	fa := NewFileAgent(sandbox)
	cfg := agent.AgentConfig{
		ID:   "file-stop",
		Name: "file-stop-agent",
		Type: string(agent.SystemAgentFile),
	}
	require.NoError(t, fa.Init(cfg))
	err := fa.Stop(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopped, fa.CurrentState())
}

// ---------------------------------------------------------------------------
// Sandbox enforcement tests
// ---------------------------------------------------------------------------

func TestFileAgent_ReadFile_OutsideSandbox(t *testing.T) {
	fa, _ := newTestFileAgent(t)
	_, err := fa.ReadFile("/etc/passwd")
	assert.ErrorIs(t, err, ErrPathOutsideSandbox)
}

func TestFileAgent_WriteFile_OutsideSandbox(t *testing.T) {
	fa, _ := newTestFileAgent(t)
	err := fa.WriteFile("/tmp/outside.txt", []byte("data"), 0644)
	assert.ErrorIs(t, err, ErrPathOutsideSandbox)
}

func TestFileAgent_RemoveFile_OutsideSandbox(t *testing.T) {
	fa, _ := newTestFileAgent(t)
	err := fa.RemoveFile("/tmp/outside.txt")
	assert.ErrorIs(t, err, ErrPathOutsideSandbox)
}

func TestFileAgent_Sandbox_SymlinkEscape(t *testing.T) {
	fa, sandbox := newTestFileAgent(t)

	// Create a symlink inside sandbox pointing outside
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	require.NoError(t, os.WriteFile(outsideFile, []byte("secret"), 0644))

	link := filepath.Join(sandbox, "escape")
	require.NoError(t, os.Symlink(outsideDir, link))

	_, err := fa.ReadFile(filepath.Join(link, "secret.txt"))
	assert.ErrorIs(t, err, ErrPathOutsideSandbox)
}

// ---------------------------------------------------------------------------
// ReadFile / WriteFile tests
// ---------------------------------------------------------------------------

func TestFileAgent_WriteAndReadFile(t *testing.T) {
	fa, sandbox := newTestFileAgent(t)

	filePath := filepath.Join(sandbox, "test.txt")
	data := []byte("hello world")

	err := fa.WriteFile(filePath, data, 0644)
	require.NoError(t, err)

	read, err := fa.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, data, read)
}

func TestFileAgent_WriteFile_CreatesIntermediateDirs(t *testing.T) {
	fa, sandbox := newTestFileAgent(t)

	filePath := filepath.Join(sandbox, "a", "b", "c", "deep.txt")
	data := []byte("deep content")

	err := fa.WriteFile(filePath, data, 0644)
	require.NoError(t, err)

	read, err := fa.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, data, read)
}

func TestFileAgent_ReadFile_NotFound(t *testing.T) {
	fa, sandbox := newTestFileAgent(t)
	_, err := fa.ReadFile(filepath.Join(sandbox, "nonexistent.txt"))
	assert.ErrorIs(t, err, ErrFileNotFound)
}

func TestFileAgent_ReadFile_Closed(t *testing.T) {
	sandbox := t.TempDir()
	fa := NewFileAgent(sandbox)
	cfg := agent.AgentConfig{
		ID:   "file-read-closed",
		Name: "file-read-closed-agent",
		Type: string(agent.SystemAgentFile),
	}
	require.NoError(t, fa.Init(cfg))
	require.NoError(t, fa.Stop(context.Background()))

	_, err := fa.ReadFile(filepath.Join(sandbox, "test.txt"))
	assert.ErrorIs(t, err, ErrFileAgentClosed)
}

// ---------------------------------------------------------------------------
// AppendFile tests
// ---------------------------------------------------------------------------

func TestFileAgent_AppendFile(t *testing.T) {
	fa, sandbox := newTestFileAgent(t)

	filePath := filepath.Join(sandbox, "append.txt")
	require.NoError(t, fa.WriteFile(filePath, []byte("hello"), 0644))
	require.NoError(t, fa.AppendFile(filePath, []byte(" world")))

	read, err := fa.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello world"), read)
}

func TestFileAgent_AppendFile_CreatesNew(t *testing.T) {
	fa, sandbox := newTestFileAgent(t)

	filePath := filepath.Join(sandbox, "new-append.txt")
	require.NoError(t, fa.AppendFile(filePath, []byte("created")))

	read, err := fa.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, []byte("created"), read)
}

// ---------------------------------------------------------------------------
// FileExists tests
// ---------------------------------------------------------------------------

func TestFileAgent_FileExists(t *testing.T) {
	fa, sandbox := newTestFileAgent(t)

	filePath := filepath.Join(sandbox, "exists.txt")
	assert.False(t, fa.FileExists(filePath))

	require.NoError(t, fa.WriteFile(filePath, []byte("data"), 0644))
	assert.True(t, fa.FileExists(filePath))
}

// ---------------------------------------------------------------------------
// RemoveFile tests
// ---------------------------------------------------------------------------

func TestFileAgent_RemoveFile(t *testing.T) {
	fa, sandbox := newTestFileAgent(t)

	filePath := filepath.Join(sandbox, "remove.txt")
	require.NoError(t, fa.WriteFile(filePath, []byte("data"), 0644))

	err := fa.RemoveFile(filePath)
	require.NoError(t, err)
	assert.False(t, fa.FileExists(filePath))
}

func TestFileAgent_RemoveFile_NotFound(t *testing.T) {
	fa, sandbox := newTestFileAgent(t)
	err := fa.RemoveFile(filepath.Join(sandbox, "gone.txt"))
	assert.ErrorIs(t, err, ErrFileNotFound)
}

// ---------------------------------------------------------------------------
// ListDir tests
// ---------------------------------------------------------------------------

func TestFileAgent_ListDir(t *testing.T) {
	fa, sandbox := newTestFileAgent(t)

	require.NoError(t, fa.WriteFile(filepath.Join(sandbox, "a.txt"), []byte("a"), 0644))
	require.NoError(t, fa.WriteFile(filepath.Join(sandbox, "b.txt"), []byte("b"), 0644))
	require.NoError(t, os.Mkdir(filepath.Join(sandbox, "subdir"), 0755))

	entries, err := fa.ListDir(sandbox)
	require.NoError(t, err)
	assert.Len(t, entries, 3)

	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name] = true
	}
	assert.True(t, names["a.txt"])
	assert.True(t, names["b.txt"])
	assert.True(t, names["subdir"])
}

func TestFileAgent_ListDir_NotFound(t *testing.T) {
	fa, sandbox := newTestFileAgent(t)
	_, err := fa.ListDir(filepath.Join(sandbox, "nope"))
	assert.ErrorIs(t, err, ErrDirNotFound)
}

func TestFileAgent_ListDir_OutsideSandbox(t *testing.T) {
	fa, _ := newTestFileAgent(t)
	_, err := fa.ListDir("/etc")
	assert.ErrorIs(t, err, ErrPathOutsideSandbox)
}

// ---------------------------------------------------------------------------
// WatchDir / UnwatchDir tests
// ---------------------------------------------------------------------------

func TestFileAgent_WatchDir(t *testing.T) {
	fa, sandbox := newTestFileAgent(t)

	watchDir := filepath.Join(sandbox, "watched")
	require.NoError(t, os.Mkdir(watchDir, 0755))

	var received atomic.Value
	done := make(chan struct{})
	watchID, err := fa.WatchDir(watchDir, func(evt FileEvent) {
		received.Store(evt)
		select {
		case <-done:
		default:
			close(done)
		}
	})
	require.NoError(t, err)
	assert.NotEmpty(t, string(watchID))

	// Trigger file creation event
	require.NoError(t, os.WriteFile(filepath.Join(watchDir, "new.txt"), []byte("data"), 0644))

	select {
	case <-done:
		evt := received.Load().(FileEvent)
		assert.Contains(t, evt.Path, "new.txt")
		assert.NotZero(t, evt.Timestamp)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for file watch event")
	}

	// Unwatch
	require.NoError(t, fa.UnwatchDir(watchID))
}

func TestFileAgent_WatchDir_InvalidPath(t *testing.T) {
	fa, sandbox := newTestFileAgent(t)
	_, err := fa.WatchDir(filepath.Join(sandbox, "nonexistent"), func(_ FileEvent) {})
	assert.ErrorIs(t, err, ErrWatchPathInvalid)
}

func TestFileAgent_WatchDir_OutsideSandbox(t *testing.T) {
	fa, _ := newTestFileAgent(t)
	_, err := fa.WatchDir("/tmp", func(_ FileEvent) {})
	assert.ErrorIs(t, err, ErrPathOutsideSandbox)
}

func TestFileAgent_UnwatchDir_NotFound(t *testing.T) {
	fa, _ := newTestFileAgent(t)
	err := fa.UnwatchDir(FileWatchID("nonexistent"))
	assert.ErrorIs(t, err, ErrEventSubNotFound)
}

// ---------------------------------------------------------------------------
// Stats tests
// ---------------------------------------------------------------------------

func TestFileAgent_Stats_FilesWritten(t *testing.T) {
	fa, sandbox := newTestFileAgent(t)

	require.NoError(t, fa.WriteFile(filepath.Join(sandbox, "s1.txt"), []byte("a"), 0644))
	require.NoError(t, fa.WriteFile(filepath.Join(sandbox, "s2.txt"), []byte("bb"), 0644))

	assert.Equal(t, int64(2), fa.fileStats.filesWritten.Load())
	assert.Equal(t, int64(3), fa.fileStats.bytesWritten.Load())
}

func TestFileAgent_Stats_FilesRead(t *testing.T) {
	fa, sandbox := newTestFileAgent(t)

	filePath := filepath.Join(sandbox, "r.txt")
	require.NoError(t, fa.WriteFile(filePath, []byte("hello"), 0644))

	_, err := fa.ReadFile(filePath)
	require.NoError(t, err)

	assert.Equal(t, int64(1), fa.fileStats.filesRead.Load())
	assert.Equal(t, int64(5), fa.fileStats.bytesRead.Load())
}

// ---------------------------------------------------------------------------
// Concurrency test
// ---------------------------------------------------------------------------

func TestFileAgent_Concurrent_ReadWrite(t *testing.T) {
	fa, sandbox := newTestFileAgent(t)

	const goroutines = 10
	done := make(chan struct{})

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			filePath := filepath.Join(sandbox, "concurrent.txt")
			_ = fa.WriteFile(filePath, []byte("data"), 0644)
			_, _ = fa.ReadFile(filePath)
		}(i)
	}

	go func() {
		time.Sleep(500 * time.Millisecond)
		close(done)
	}()

	<-done
}
