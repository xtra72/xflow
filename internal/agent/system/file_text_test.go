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

// newTestTextAgent creates an initialized FileAgentImpl in text mode.
func newTestTextAgent(t *testing.T) (*FileAgentImpl, string) {
	t.Helper()
	sandbox := t.TempDir()
	fa := NewFileAgent(sandbox)
	cfg := agent.AgentConfig{
		ID:   "file-text-test",
		Name: "file-text-test-agent",
		Type: string(agent.SystemAgentFile),
		Metadata: map[string]string{
			"mode":     "text",
			"encoding": "utf-8",
		},
	}
	require.NoError(t, fa.Init(cfg))
	t.Cleanup(func() {
		_ = fa.Stop(context.Background())
	})
	return fa, sandbox
}

// ---------------------------------------------------------------------------
// Text mode rejection in binary mode
// ---------------------------------------------------------------------------

func TestTextOps_BinaryModeRejected(t *testing.T) {
	fa, sandbox := newTestFileAgent(t) // binary mode by default
	path := filepath.Join(sandbox, "test.txt")

	tests := []struct {
		name string
		fn   func() error
	}{
		{"ReadFileText", func() error { _, err := fa.ReadFileText(path); return err }},
		{"WriteFileText", func() error { return fa.WriteFileText(path, "hi", 0644) }},
		{"AppendFileText", func() error { return fa.AppendFileText(path, "hi") }},
		{"ReadLines", func() error { _, err := fa.ReadLines(path); return err }},
		{"ReadLine", func() error { _, err := fa.ReadLine(path, 1); return err }},
		{"WriteLines", func() error { return fa.WriteLines(path, []string{"a"}, 0644) }},
		{"AppendLine", func() error { return fa.AppendLine(path, "a") }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			assert.ErrorIs(t, err, ErrTextModeRequired)
		})
	}
}

// ---------------------------------------------------------------------------
// ReadFileText / WriteFileText
// ---------------------------------------------------------------------------

func TestTextOps_ReadWriteFileText(t *testing.T) {
	fa, sandbox := newTestTextAgent(t)
	path := filepath.Join(sandbox, "hello.txt")

	// Write text.
	err := fa.WriteFileText(path, "hello world", 0644)
	require.NoError(t, err)

	// Read text back.
	content, err := fa.ReadFileText(path)
	require.NoError(t, err)
	assert.Equal(t, "hello world", content)
}

func TestTextOps_ReadFileText_NotFound(t *testing.T) {
	fa, sandbox := newTestTextAgent(t)
	_, err := fa.ReadFileText(filepath.Join(sandbox, "nope.txt"))
	assert.ErrorIs(t, err, ErrFileNotFound)
}

// ---------------------------------------------------------------------------
// AppendFileText
// ---------------------------------------------------------------------------

func TestTextOps_AppendFileText(t *testing.T) {
	fa, sandbox := newTestTextAgent(t)
	path := filepath.Join(sandbox, "append.txt")

	require.NoError(t, fa.WriteFileText(path, "line1\n", 0644))
	require.NoError(t, fa.AppendFileText(path, "line2\n"))

	content, err := fa.ReadFileText(path)
	require.NoError(t, err)
	assert.Equal(t, "line1\nline2\n", content)
}

// ---------------------------------------------------------------------------
// ReadLines / WriteLines
// ---------------------------------------------------------------------------

func TestTextOps_ReadWriteLines(t *testing.T) {
	fa, sandbox := newTestTextAgent(t)
	path := filepath.Join(sandbox, "lines.txt")

	lines := []string{"alpha", "beta", "gamma"}
	require.NoError(t, fa.WriteLines(path, lines, 0644))

	got, err := fa.ReadLines(path)
	require.NoError(t, err)
	assert.Equal(t, lines, got)
}

func TestTextOps_ReadLines_EmptyFile(t *testing.T) {
	fa, sandbox := newTestTextAgent(t)
	path := filepath.Join(sandbox, "empty.txt")
	require.NoError(t, os.WriteFile(path, []byte(""), 0644))

	lines, err := fa.ReadLines(path)
	require.NoError(t, err)
	assert.Empty(t, lines)
}

// ---------------------------------------------------------------------------
// ReadLine
// ---------------------------------------------------------------------------

func TestTextOps_ReadLine(t *testing.T) {
	fa, sandbox := newTestTextAgent(t)
	path := filepath.Join(sandbox, "numbered.txt")
	require.NoError(t, fa.WriteLines(path, []string{"one", "two", "three"}, 0644))

	tests := []struct {
		name    string
		lineNum int
		want    string
		wantErr error
	}{
		{"first line", 1, "one", nil},
		{"second line", 2, "two", nil},
		{"third line", 3, "three", nil},
		{"zero index", 0, "", ErrLineOutOfRange},
		{"too large", 4, "", ErrLineOutOfRange},
		{"negative", -1, "", ErrLineOutOfRange},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			line, err := fa.ReadLine(path, tc.lineNum)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.want, line)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AppendLine
// ---------------------------------------------------------------------------

func TestTextOps_AppendLine(t *testing.T) {
	fa, sandbox := newTestTextAgent(t)
	path := filepath.Join(sandbox, "append_line.txt")
	require.NoError(t, fa.WriteLines(path, []string{"first"}, 0644))
	require.NoError(t, fa.AppendLine(path, "second"))

	lines, err := fa.ReadLines(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"first", "second"}, lines)
}

// ---------------------------------------------------------------------------
// Sandbox enforcement in text mode
// ---------------------------------------------------------------------------

func TestTextOps_SandboxEnforcement(t *testing.T) {
	fa, _ := newTestTextAgent(t)

	_, err := fa.ReadFileText("/etc/passwd")
	assert.ErrorIs(t, err, ErrPathOutsideSandbox)

	err = fa.WriteFileText("/etc/evil", "bad", 0644)
	assert.ErrorIs(t, err, ErrPathOutsideSandbox)
}

// ---------------------------------------------------------------------------
// Stats tracking in text mode
// ---------------------------------------------------------------------------

func TestTextOps_StatsTracking(t *testing.T) {
	fa, sandbox := newTestTextAgent(t)
	path := filepath.Join(sandbox, "stats.txt")

	require.NoError(t, fa.WriteFileText(path, "hello", 0644))
	assert.Equal(t, int64(1), fa.fileStats.filesWritten.Load())
	assert.Equal(t, int64(5), fa.fileStats.bytesWritten.Load())

	_, err := fa.ReadFileText(path)
	require.NoError(t, err)
	assert.Equal(t, int64(1), fa.fileStats.filesRead.Load())
	assert.Equal(t, int64(5), fa.fileStats.bytesRead.Load())
}
