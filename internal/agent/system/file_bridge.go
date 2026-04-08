package system

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"github.com/xtra/xflow/internal/agent"
)

// Compile-time interface check for MessageReceiver.
var _ agent.MessageReceiver = (*FileAgentImpl)(nil)

// fileCommand represents an incoming bridge command.
type fileCommand struct {
	Command string            `json:"command"`
	Params  fileCommandParams `json:"params"`
}

// fileCommandParams holds the parameters for a file bridge command.
type fileCommandParams struct {
	Path    string   `json:"path,omitempty"`
	Alias   string   `json:"alias,omitempty"`
	Data    string   `json:"data,omitempty"`    // base64 for binary, plain for text
	Perm    uint32   `json:"perm,omitempty"`
	LineNum int      `json:"line_num,omitempty"`
	Lines   []string `json:"lines,omitempty"`
}

// fileResponse is the JSON response returned by Process.
type fileResponse struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

// fileEventMessage is the JSON structure pushed to eventCh for bridge events.
type fileEventMessage struct {
	Event     string `json:"event"`
	Path      string `json:"path"`
	Alias     string `json:"alias,omitempty"`
	Timestamp string `json:"timestamp"`
}

// Process overrides BaseAgent.Process to handle file bridge commands.
func (f *FileAgentImpl) Process(data []byte) ([]byte, error) {
	if err := f.checkClosed(); err != nil {
		return nil, err
	}

	var cmd fileCommand
	if err := json.Unmarshal(data, &cmd); err != nil {
		return marshalResponse(fileResponse{Error: ErrInvalidCommandFormat.Error()})
	}

	resp := f.dispatch(cmd)
	return marshalResponse(resp)
}

// ReceiveMessage implements agent.MessageReceiver. It blocks until a file
// system event is available on the internal event channel or the context
// is cancelled.
func (f *FileAgentImpl) ReceiveMessage(ctx context.Context) ([]byte, error) {
	select {
	case data, ok := <-f.eventCh:
		if !ok {
			return nil, ErrFileAgentClosed
		}
		return data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// dispatch routes a command to the appropriate handler and returns a response.
func (f *FileAgentImpl) dispatch(cmd fileCommand) fileResponse {
	switch cmd.Command {
	// Binary file operations.
	case "read_file":
		return f.cmdReadFile(cmd.Params)
	case "write_file":
		return f.cmdWriteFile(cmd.Params)
	case "append_file":
		return f.cmdAppendFile(cmd.Params)
	case "file_exists":
		return f.cmdFileExists(cmd.Params)
	case "remove_file":
		return f.cmdRemoveFile(cmd.Params)
	case "list_dir":
		return f.cmdListDir(cmd.Params)

	// Text file operations.
	case "read_text":
		return f.cmdReadText(cmd.Params)
	case "write_text":
		return f.cmdWriteText(cmd.Params)
	case "append_text":
		return f.cmdAppendText(cmd.Params)
	case "read_lines":
		return f.cmdReadLines(cmd.Params)
	case "read_line":
		return f.cmdReadLine(cmd.Params)
	case "write_lines":
		return f.cmdWriteLines(cmd.Params)
	case "append_line":
		return f.cmdAppendLine(cmd.Params)

	default:
		return fileResponse{Error: fmt.Sprintf("%s: %s", ErrUnsupportedCommand.Error(), cmd.Command)}
	}
}

// ---------------------------------------------------------------------------
// Binary command handlers
// ---------------------------------------------------------------------------

func (f *FileAgentImpl) cmdReadFile(p fileCommandParams) fileResponse {
	path, err := f.resolvePath(p)
	if err != nil {
		return fileResponse{Error: err.Error()}
	}
	data, err := f.ReadFile(path)
	if err != nil {
		return fileResponse{Error: err.Error()}
	}
	return fileResponse{Success: true, Data: base64.StdEncoding.EncodeToString(data)}
}

func (f *FileAgentImpl) cmdWriteFile(p fileCommandParams) fileResponse {
	path, err := f.resolvePath(p)
	if err != nil {
		return fileResponse{Error: err.Error()}
	}
	data, err := base64.StdEncoding.DecodeString(p.Data)
	if err != nil {
		return fileResponse{Error: fmt.Sprintf("base64 decode: %s", err.Error())}
	}
	perm := os.FileMode(p.Perm)
	if perm == 0 {
		perm = 0644
	}
	if err := f.WriteFile(path, data, perm); err != nil {
		return fileResponse{Error: err.Error()}
	}
	return fileResponse{Success: true}
}

func (f *FileAgentImpl) cmdAppendFile(p fileCommandParams) fileResponse {
	path, err := f.resolvePath(p)
	if err != nil {
		return fileResponse{Error: err.Error()}
	}
	data, err := base64.StdEncoding.DecodeString(p.Data)
	if err != nil {
		return fileResponse{Error: fmt.Sprintf("base64 decode: %s", err.Error())}
	}
	if err := f.AppendFile(path, data); err != nil {
		return fileResponse{Error: err.Error()}
	}
	return fileResponse{Success: true}
}

func (f *FileAgentImpl) cmdFileExists(p fileCommandParams) fileResponse {
	path, err := f.resolvePath(p)
	if err != nil {
		return fileResponse{Error: err.Error()}
	}
	exists := f.FileExists(path)
	return fileResponse{Success: true, Data: exists}
}

func (f *FileAgentImpl) cmdRemoveFile(p fileCommandParams) fileResponse {
	path, err := f.resolvePath(p)
	if err != nil {
		return fileResponse{Error: err.Error()}
	}
	if err := f.RemoveFile(path); err != nil {
		return fileResponse{Error: err.Error()}
	}
	return fileResponse{Success: true}
}

func (f *FileAgentImpl) cmdListDir(p fileCommandParams) fileResponse {
	path, err := f.resolvePath(p)
	if err != nil {
		return fileResponse{Error: err.Error()}
	}
	entries, err := f.ListDir(path)
	if err != nil {
		return fileResponse{Error: err.Error()}
	}
	return fileResponse{Success: true, Data: entries}
}

// ---------------------------------------------------------------------------
// Text command handlers
// ---------------------------------------------------------------------------

func (f *FileAgentImpl) cmdReadText(p fileCommandParams) fileResponse {
	path, err := f.resolvePath(p)
	if err != nil {
		return fileResponse{Error: err.Error()}
	}
	text, err := f.ReadFileText(path)
	if err != nil {
		return fileResponse{Error: err.Error()}
	}
	return fileResponse{Success: true, Data: text}
}

func (f *FileAgentImpl) cmdWriteText(p fileCommandParams) fileResponse {
	path, err := f.resolvePath(p)
	if err != nil {
		return fileResponse{Error: err.Error()}
	}
	perm := os.FileMode(p.Perm)
	if perm == 0 {
		perm = 0644
	}
	if err := f.WriteFileText(path, p.Data, perm); err != nil {
		return fileResponse{Error: err.Error()}
	}
	return fileResponse{Success: true}
}

func (f *FileAgentImpl) cmdAppendText(p fileCommandParams) fileResponse {
	path, err := f.resolvePath(p)
	if err != nil {
		return fileResponse{Error: err.Error()}
	}
	if err := f.AppendFileText(path, p.Data); err != nil {
		return fileResponse{Error: err.Error()}
	}
	return fileResponse{Success: true}
}

func (f *FileAgentImpl) cmdReadLines(p fileCommandParams) fileResponse {
	path, err := f.resolvePath(p)
	if err != nil {
		return fileResponse{Error: err.Error()}
	}
	lines, err := f.ReadLines(path)
	if err != nil {
		return fileResponse{Error: err.Error()}
	}
	return fileResponse{Success: true, Data: lines}
}

func (f *FileAgentImpl) cmdReadLine(p fileCommandParams) fileResponse {
	path, err := f.resolvePath(p)
	if err != nil {
		return fileResponse{Error: err.Error()}
	}
	line, err := f.ReadLine(path, p.LineNum)
	if err != nil {
		return fileResponse{Error: err.Error()}
	}
	return fileResponse{Success: true, Data: line}
}

func (f *FileAgentImpl) cmdWriteLines(p fileCommandParams) fileResponse {
	path, err := f.resolvePath(p)
	if err != nil {
		return fileResponse{Error: err.Error()}
	}
	perm := os.FileMode(p.Perm)
	if perm == 0 {
		perm = 0644
	}
	if err := f.WriteLines(path, p.Lines, perm); err != nil {
		return fileResponse{Error: err.Error()}
	}
	return fileResponse{Success: true}
}

func (f *FileAgentImpl) cmdAppendLine(p fileCommandParams) fileResponse {
	path, err := f.resolvePath(p)
	if err != nil {
		return fileResponse{Error: err.Error()}
	}
	if err := f.AppendLine(path, p.Data); err != nil {
		return fileResponse{Error: err.Error()}
	}
	return fileResponse{Success: true}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// marshalResponse marshals a fileResponse to JSON bytes.
func marshalResponse(resp fileResponse) ([]byte, error) {
	data, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("file agent: marshal response: %w", err)
	}
	return data, nil
}
