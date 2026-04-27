package system

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// newTestBridgeAgent creates a FileAgentImpl configured for bridge testing.
func newTestBridgeAgent(t *testing.T, mode string) (*FileAgentImpl, string) {
	t.Helper()
	sandbox := t.TempDir()
	fa := NewFileAgent(sandbox)
	meta := map[string]string{
		"mode": mode,
	}
	cfg := agent.AgentConfig{
		ID:       "file-bridge-test",
		Name:     "file-bridge-test-agent",
		Type:     string(agent.SystemAgentFile),
		Metadata: meta,
	}
	require.NoError(t, fa.Init(cfg))
	t.Cleanup(func() {
		_ = fa.Stop(context.Background())
	})
	return fa, sandbox
}

// execCmd is a helper that sends a JSON command to Process and returns the parsed response.
func execCmd(t *testing.T, fa *FileAgentImpl, cmd fileCommand) fileResponse {
	t.Helper()
	data, err := json.Marshal(cmd)
	require.NoError(t, err)

	respData, err := fa.Process(data)
	require.NoError(t, err)

	var resp fileResponse
	require.NoError(t, json.Unmarshal(respData, &resp))
	return resp
}

// ---------------------------------------------------------------------------
// Process: invalid JSON
// ---------------------------------------------------------------------------

func TestProcess_InvalidJSON(t *testing.T) {
	fa, _ := newTestBridgeAgent(t, "binary")

	respData, err := fa.Process([]byte(`{invalid json}`))
	require.NoError(t, err)

	var resp fileResponse
	require.NoError(t, json.Unmarshal(respData, &resp))
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "invalid command format")
}

// ---------------------------------------------------------------------------
// Process: unsupported command
// ---------------------------------------------------------------------------

func TestProcess_UnsupportedCommand(t *testing.T) {
	fa, _ := newTestBridgeAgent(t, "binary")

	resp := execCmd(t, fa, fileCommand{Command: "fly_to_moon"})
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "unsupported command")
}

// ---------------------------------------------------------------------------
// Binary commands: read_file, write_file, file_exists, remove_file
// ---------------------------------------------------------------------------

func TestProcess_WriteAndReadFile_Binary(t *testing.T) {
	fa, sandbox := newTestBridgeAgent(t, "binary")
	path := filepath.Join(sandbox, "binary.dat")
	payload := []byte{0x00, 0xFF, 0xAB}

	// write_file
	resp := execCmd(t, fa, fileCommand{
		Command: "write_file",
		Params: fileCommandParams{
			Path: path,
			Data: base64.StdEncoding.EncodeToString(payload),
			Perm: 0644,
		},
	})
	assert.True(t, resp.Success)

	// read_file
	resp = execCmd(t, fa, fileCommand{
		Command: "read_file",
		Params:  fileCommandParams{Path: path},
	})
	assert.True(t, resp.Success)
	decoded, err := base64.StdEncoding.DecodeString(resp.Data.(string))
	require.NoError(t, err)
	assert.Equal(t, payload, decoded)

	// file_exists
	resp = execCmd(t, fa, fileCommand{
		Command: "file_exists",
		Params:  fileCommandParams{Path: path},
	})
	assert.True(t, resp.Success)
	assert.Equal(t, true, resp.Data)

	// remove_file
	resp = execCmd(t, fa, fileCommand{
		Command: "remove_file",
		Params:  fileCommandParams{Path: path},
	})
	assert.True(t, resp.Success)

	// file_exists after remove
	resp = execCmd(t, fa, fileCommand{
		Command: "file_exists",
		Params:  fileCommandParams{Path: path},
	})
	assert.True(t, resp.Success)
	assert.Equal(t, false, resp.Data)
}

// ---------------------------------------------------------------------------
// Binary command: append_file
// ---------------------------------------------------------------------------

func TestProcess_AppendFile_Binary(t *testing.T) {
	fa, sandbox := newTestBridgeAgent(t, "binary")
	path := filepath.Join(sandbox, "append.dat")
	part1 := []byte("AB")
	part2 := []byte("CD")

	execCmd(t, fa, fileCommand{
		Command: "write_file",
		Params:  fileCommandParams{Path: path, Data: base64.StdEncoding.EncodeToString(part1)},
	})
	resp := execCmd(t, fa, fileCommand{
		Command: "append_file",
		Params:  fileCommandParams{Path: path, Data: base64.StdEncoding.EncodeToString(part2)},
	})
	assert.True(t, resp.Success)

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, []byte("ABCD"), got)
}

// ---------------------------------------------------------------------------
// Binary command: list_dir
// ---------------------------------------------------------------------------

func TestProcess_ListDir(t *testing.T) {
	fa, sandbox := newTestBridgeAgent(t, "binary")
	require.NoError(t, os.WriteFile(filepath.Join(sandbox, "a.txt"), []byte("a"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(sandbox, "subdir"), 0755))

	resp := execCmd(t, fa, fileCommand{
		Command: "list_dir",
		Params:  fileCommandParams{Path: sandbox},
	})
	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Data)
}

// ---------------------------------------------------------------------------
// Text commands via Process
// ---------------------------------------------------------------------------

func TestProcess_TextCommands(t *testing.T) {
	fa, sandbox := newTestBridgeAgent(t, "text")
	path := filepath.Join(sandbox, "text.txt")

	// write_text
	resp := execCmd(t, fa, fileCommand{
		Command: "write_text",
		Params:  fileCommandParams{Path: path, Data: "hello\nworld\n"},
	})
	assert.True(t, resp.Success)

	// read_text
	resp = execCmd(t, fa, fileCommand{
		Command: "read_text",
		Params:  fileCommandParams{Path: path},
	})
	assert.True(t, resp.Success)
	assert.Equal(t, "hello\nworld\n", resp.Data)

	// read_lines
	resp = execCmd(t, fa, fileCommand{
		Command: "read_lines",
		Params:  fileCommandParams{Path: path},
	})
	assert.True(t, resp.Success)

	// read_line
	resp = execCmd(t, fa, fileCommand{
		Command: "read_line",
		Params:  fileCommandParams{Path: path, LineNum: 1},
	})
	assert.True(t, resp.Success)
	assert.Equal(t, "hello", resp.Data)

	// append_text
	resp = execCmd(t, fa, fileCommand{
		Command: "append_text",
		Params:  fileCommandParams{Path: path, Data: "appended\n"},
	})
	assert.True(t, resp.Success)

	// write_lines
	resp = execCmd(t, fa, fileCommand{
		Command: "write_lines",
		Params:  fileCommandParams{Path: path, Lines: []string{"a", "b", "c"}},
	})
	assert.True(t, resp.Success)

	// append_line
	resp = execCmd(t, fa, fileCommand{
		Command: "append_line",
		Params:  fileCommandParams{Path: path, Data: "d"},
	})
	assert.True(t, resp.Success)

	// Verify final content.
	lines, err := fa.ReadLines(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c", "d"}, lines)
}

// ---------------------------------------------------------------------------
// Text commands rejected in binary mode
// ---------------------------------------------------------------------------

func TestProcess_TextCommandsRejectedInBinaryMode(t *testing.T) {
	fa, sandbox := newTestBridgeAgent(t, "binary")
	path := filepath.Join(sandbox, "test.txt")

	textCmds := []string{"read_text", "write_text", "append_text", "read_lines", "read_line", "write_lines", "append_line"}
	for _, cmd := range textCmds {
		t.Run(cmd, func(t *testing.T) {
			resp := execCmd(t, fa, fileCommand{
				Command: cmd,
				Params:  fileCommandParams{Path: path, Data: "x", Lines: []string{"x"}, LineNum: 1},
			})
			assert.False(t, resp.Success)
			assert.Contains(t, resp.Error, "text mode required")
		})
	}
}

// ---------------------------------------------------------------------------
// Process with alias
// ---------------------------------------------------------------------------

func TestProcess_WithAlias(t *testing.T) {
	sandbox := t.TempDir()
	dataDir := filepath.Join(sandbox, "data")
	require.NoError(t, os.MkdirAll(dataDir, 0755))
	cfgFile := filepath.Join(dataDir, "config.json")
	require.NoError(t, os.WriteFile(cfgFile, []byte(`{"key":"value"}`), 0644))

	fa := NewFileAgent(sandbox)
	cfg := agent.AgentConfig{
		ID:   "file-alias-test",
		Name: "file-alias-test-agent",
		Type: string(agent.SystemAgentFile),
		Metadata: map[string]string{
			"mode":         "text",
			"target_files": `{"cfg": "` + cfgFile + `"}`,
		},
	}
	require.NoError(t, fa.Init(cfg))
	t.Cleanup(func() {
		_ = fa.Stop(context.Background())
	})

	// Read by alias.
	resp := execCmd(t, fa, fileCommand{
		Command: "read_text",
		Params:  fileCommandParams{Alias: "cfg"},
	})
	assert.True(t, resp.Success)
	assert.Equal(t, `{"key":"value"}`, resp.Data)

	// Non-existent alias.
	resp = execCmd(t, fa, fileCommand{
		Command: "read_text",
		Params:  fileCommandParams{Alias: "nope"},
	})
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "alias not found")
}

// ---------------------------------------------------------------------------
// ReceiveMessage
// ---------------------------------------------------------------------------

func TestReceiveMessage_ContextCancelled(t *testing.T) {
	fa, _ := newTestBridgeAgent(t, "binary")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := fa.ReceiveMessage(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestReceiveMessage_EventDelivered(t *testing.T) {
	fa, _ := newTestBridgeAgent(t, "binary")

	// Push an event directly.
	testEvent := []byte(`{"event":"create","path":"test.txt","timestamp":"2024-01-01T00:00:00Z"}`)
	fa.eventCh <- testEvent

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	data, err := fa.ReceiveMessage(ctx)
	require.NoError(t, err)
	assert.Equal(t, testEvent, data)
}

func TestReceiveMessage_ChannelClosed(t *testing.T) {
	sandbox := t.TempDir()
	fa := NewFileAgent(sandbox)
	cfg := agent.AgentConfig{
		ID:   "file-recv-close-test",
		Name: "file-recv-close-test-agent",
		Type: string(agent.SystemAgentFile),
	}
	require.NoError(t, fa.Init(cfg))

	// Stop to close eventCh.
	require.NoError(t, fa.Stop(context.Background()))

	_, err := fa.ReceiveMessage(context.Background())
	assert.ErrorIs(t, err, ErrFileAgentClosed)
}

// ---------------------------------------------------------------------------
// Process on closed agent
// ---------------------------------------------------------------------------

func TestProcess_ClosedAgent(t *testing.T) {
	sandbox := t.TempDir()
	fa := NewFileAgent(sandbox)
	cfg := agent.AgentConfig{
		ID:   "file-closed-test",
		Name: "file-closed-test-agent",
		Type: string(agent.SystemAgentFile),
	}
	require.NoError(t, fa.Init(cfg))
	require.NoError(t, fa.Stop(context.Background()))

	_, err := fa.Process([]byte(`{"command":"read_file","params":{"path":"x"}}`))
	assert.ErrorIs(t, err, ErrFileAgentClosed)
}

// ---------------------------------------------------------------------------
// fileEventMessage JSON structure
// ---------------------------------------------------------------------------

func TestFileEventMessage_JSON(t *testing.T) {
	msg := fileEventMessage{
		Event:     "create",
		Path:      "data/file.txt",
		Alias:     "myalias",
		Timestamp: "2024-01-01T00:00:00Z",
	}
	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var parsed map[string]string
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Equal(t, "create", parsed["event"])
	assert.Equal(t, "data/file.txt", parsed["path"])
	assert.Equal(t, "myalias", parsed["alias"])
	assert.Equal(t, "2024-01-01T00:00:00Z", parsed["timestamp"])
}
