package system

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/xtra/xflow/internal/agent"
)

// FileWatchID is a unique identifier for a directory watch.
type FileWatchID string

// FileEventHandler is the function called when a file system event occurs.
type FileEventHandler func(event FileEvent)

// FileOp represents the type of file system operation.
type FileOp string

const (
	FileOpCreate FileOp = "create"
	FileOpWrite  FileOp = "write"
	FileOpRemove FileOp = "remove"
	FileOpRename FileOp = "rename"
	FileOpChmod  FileOp = "chmod"
)

// FileEvent represents a file system event.
type FileEvent struct {
	Path      string
	Op        FileOp
	Timestamp time.Time
}

// FileInfo holds metadata about a file or directory.
type FileInfo struct {
	Name    string
	Size    int64
	IsDir   bool
	ModTime time.Time
	Perm    os.FileMode
}

// FileOperator defines the interface for sandbox-restricted file operations.
type FileOperator interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	AppendFile(path string, data []byte) error
	FileExists(path string) bool
	RemoveFile(path string) error
	ListDir(dir string) ([]FileInfo, error)
	WatchDir(dir string, handler FileEventHandler) (FileWatchID, error)
	UnwatchDir(id FileWatchID) error
}

// fileWatch holds watch metadata.
type fileWatch struct {
	id      FileWatchID
	dir     string
	handler FileEventHandler
}

// fileAgentStats holds atomic counters for file agent statistics.
type fileAgentStats struct {
	filesRead          atomic.Int64
	filesWritten       atomic.Int64
	bytesRead          atomic.Int64
	bytesWritten       atomic.Int64
	activeWatches      atomic.Int64
	fileEventsReceived atomic.Int64
}

// FileAgentImpl is the system file I/O agent with sandbox enforcement.
// It embeds *agent.BaseAgent and implements agent.SystemAgent.
type FileAgentImpl struct {
	*agent.BaseAgent
	sandboxRoot string

	mu      sync.RWMutex
	watcher *fsnotify.Watcher
	watches sync.Map // map[FileWatchID]*fileWatch
	closed  bool

	fileStats   fileAgentStats
	nextWatchID atomic.Int64
	wg          sync.WaitGroup
}

// Compile-time interface checks.
var _ agent.SystemAgent = (*FileAgentImpl)(nil)
var _ FileOperator = (*FileAgentImpl)(nil)

// NewFileAgent creates a new FileAgentImpl with the given sandbox root.
func NewFileAgent(sandboxRoot string) *FileAgentImpl {
	return &FileAgentImpl{
		BaseAgent:   agent.NewBaseAgent(),
		sandboxRoot: sandboxRoot,
	}
}

// IsSystem returns true because FileAgent is a system agent.
func (f *FileAgentImpl) IsSystem() bool { return true }

// RequiresTransport returns false because FileAgent uses local file I/O.
func (f *FileAgentImpl) RequiresTransport() bool { return false }

// Init initializes the file agent, validates sandbox, and creates fsnotify watcher.
func (f *FileAgentImpl) Init(config agent.AgentConfig) error {
	if f.sandboxRoot == "" {
		return ErrSandboxNotConfigured
	}

	// Resolve sandbox root to absolute real path (follow symlinks)
	absRoot, err := filepath.Abs(f.sandboxRoot)
	if err != nil {
		return fmt.Errorf("file agent init: %w", err)
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return fmt.Errorf("file agent init: sandbox path resolution failed: %w", err)
	}
	f.sandboxRoot = realRoot

	if err := f.BaseAgent.Init(config); err != nil {
		return err
	}

	// Create fsnotify watcher
	w, err := fsnotify.NewWatcher()
	if err != nil {
		_ = f.BaseAgent.Stop(context.Background())
		return fmt.Errorf("file agent init: watcher creation failed: %w", err)
	}

	f.mu.Lock()
	f.watcher = w
	f.closed = false
	f.mu.Unlock()

	// Start watcher event loop
	f.wg.Add(1)
	go f.watchLoop()

	return nil
}

// Stop shuts down the file agent, closes the watcher, and waits for goroutines.
func (f *FileAgentImpl) Stop(ctx context.Context) error {
	f.mu.Lock()
	f.closed = true
	if f.watcher != nil {
		_ = f.watcher.Close()
	}
	f.mu.Unlock()

	f.wg.Wait()

	return f.BaseAgent.Stop(ctx)
}

// ReadFile reads the entire contents of a file within the sandbox.
func (f *FileAgentImpl) ReadFile(path string) ([]byte, error) {
	if err := f.checkClosed(); err != nil {
		return nil, err
	}
	safePath, err := f.resolveSandboxPath(path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(safePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrFileNotFound
		}
		return nil, fmt.Errorf("file agent read: %w", err)
	}

	f.fileStats.filesRead.Add(1)
	f.fileStats.bytesRead.Add(int64(len(data)))
	return data, nil
}

// WriteFile writes data to a file within the sandbox, creating intermediate directories.
func (f *FileAgentImpl) WriteFile(path string, data []byte, perm os.FileMode) error {
	if err := f.checkClosed(); err != nil {
		return err
	}
	safePath, err := f.resolveSandboxPath(path)
	if err != nil {
		return err
	}

	dir := filepath.Dir(safePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("file agent write: mkdir failed: %w", err)
	}

	if err := os.WriteFile(safePath, data, perm); err != nil {
		return fmt.Errorf("file agent write: %w", err)
	}

	f.fileStats.filesWritten.Add(1)
	f.fileStats.bytesWritten.Add(int64(len(data)))
	return nil
}

// AppendFile appends data to a file within the sandbox, creating it if needed.
func (f *FileAgentImpl) AppendFile(path string, data []byte) error {
	if err := f.checkClosed(); err != nil {
		return err
	}
	safePath, err := f.resolveSandboxPath(path)
	if err != nil {
		return err
	}

	dir := filepath.Dir(safePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("file agent append: mkdir failed: %w", err)
	}

	file, err := os.OpenFile(safePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("file agent append: %w", err)
	}
	defer file.Close()

	_, err = file.Write(data)
	if err != nil {
		return fmt.Errorf("file agent append: write failed: %w", err)
	}

	f.fileStats.filesWritten.Add(1)
	f.fileStats.bytesWritten.Add(int64(len(data)))
	return nil
}

// FileExists checks if a file exists within the sandbox.
func (f *FileAgentImpl) FileExists(path string) bool {
	safePath, err := f.resolveSandboxPath(path)
	if err != nil {
		return false
	}
	_, err = os.Stat(safePath)
	return err == nil
}

// RemoveFile removes a file within the sandbox.
func (f *FileAgentImpl) RemoveFile(path string) error {
	if err := f.checkClosed(); err != nil {
		return err
	}
	safePath, err := f.resolveSandboxPath(path)
	if err != nil {
		return err
	}

	if _, err := os.Stat(safePath); os.IsNotExist(err) {
		return ErrFileNotFound
	}

	if err := os.Remove(safePath); err != nil {
		return fmt.Errorf("file agent remove: %w", err)
	}
	return nil
}

// ListDir lists entries in a directory within the sandbox.
func (f *FileAgentImpl) ListDir(dir string) ([]FileInfo, error) {
	if err := f.checkClosed(); err != nil {
		return nil, err
	}
	safePath, err := f.resolveSandboxPath(dir)
	if err != nil {
		return nil, err
	}

	stat, err := os.Stat(safePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrDirNotFound
		}
		return nil, fmt.Errorf("file agent list: %w", err)
	}
	if !stat.IsDir() {
		return nil, ErrDirNotFound
	}

	entries, err := os.ReadDir(safePath)
	if err != nil {
		return nil, fmt.Errorf("file agent list: %w", err)
	}

	result := make([]FileInfo, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		result = append(result, FileInfo{
			Name:    e.Name(),
			Size:    info.Size(),
			IsDir:   e.IsDir(),
			ModTime: info.ModTime(),
			Perm:    info.Mode().Perm(),
		})
	}
	return result, nil
}

// WatchDir starts watching a directory for file system events.
func (f *FileAgentImpl) WatchDir(dir string, handler FileEventHandler) (FileWatchID, error) {
	if err := f.checkClosed(); err != nil {
		return "", err
	}
	safePath, err := f.resolveSandboxPath(dir)
	if err != nil {
		return "", err
	}

	stat, err := os.Stat(safePath)
	if err != nil || !stat.IsDir() {
		return "", ErrWatchPathInvalid
	}

	f.mu.RLock()
	w := f.watcher
	f.mu.RUnlock()

	if err := w.Add(safePath); err != nil {
		return "", fmt.Errorf("file agent watch: %w", err)
	}

	id := f.generateWatchID()
	fw := &fileWatch{
		id:      id,
		dir:     safePath,
		handler: handler,
	}
	f.watches.Store(id, fw)
	f.fileStats.activeWatches.Add(1)

	return id, nil
}

// UnwatchDir stops watching a directory.
func (f *FileAgentImpl) UnwatchDir(id FileWatchID) error {
	val, ok := f.watches.LoadAndDelete(id)
	if !ok {
		return ErrEventSubNotFound
	}

	fw := val.(*fileWatch)
	f.mu.RLock()
	w := f.watcher
	closed := f.closed
	f.mu.RUnlock()

	if !closed && w != nil {
		_ = w.Remove(fw.dir)
	}
	f.fileStats.activeWatches.Add(-1)

	return nil
}

// watchLoop reads from the fsnotify watcher and dispatches events to handlers.
func (f *FileAgentImpl) watchLoop() {
	defer f.wg.Done()

	f.mu.RLock()
	w := f.watcher
	f.mu.RUnlock()

	for {
		select {
		case event, ok := <-w.Events:
			if !ok {
				return
			}
			f.fileStats.fileEventsReceived.Add(1)

			fe := FileEvent{
				Path:      event.Name,
				Op:        fsOpToFileOp(event.Op),
				Timestamp: time.Now(),
			}

			// Dispatch to matching watch handlers
			f.watches.Range(func(_, val interface{}) bool {
				fw := val.(*fileWatch)
				if strings.HasPrefix(event.Name, fw.dir) {
					fw.handler(fe)
				}
				return true
			})

		case _, ok := <-w.Errors:
			if !ok {
				return
			}
			// Errors are logged but not propagated
		}
	}
}

// resolveSandboxPath validates and resolves a path within the sandbox.
// It walks up the directory tree to find the deepest existing ancestor,
// evaluates symlinks on that ancestor, and checks the sandbox prefix.
func (f *FileAgentImpl) resolveSandboxPath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("file agent: resolve path failed: %w", err)
	}

	// Walk up to find the deepest existing ancestor and evaluate symlinks
	current := absPath
	var tail []string
	for {
		realCurrent, err := filepath.EvalSymlinks(current)
		if err == nil {
			// Found an existing ancestor; reconstruct full path
			fullReal := realCurrent
			for i := len(tail) - 1; i >= 0; i-- {
				fullReal = filepath.Join(fullReal, tail[i])
			}
			if !strings.HasPrefix(fullReal, f.sandboxRoot) {
				return "", ErrPathOutsideSandbox
			}
			return fullReal, nil
		}
		// Move one level up
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root without finding existing dir
			break
		}
		tail = append(tail, filepath.Base(current))
		current = parent
	}

	// Fallback: use cleaned absolute path
	cleaned := filepath.Clean(absPath)
	if !strings.HasPrefix(cleaned, f.sandboxRoot) {
		return "", ErrPathOutsideSandbox
	}
	return cleaned, nil
}

// checkClosed returns ErrFileAgentClosed if the agent is stopped.
func (f *FileAgentImpl) checkClosed() error {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.closed {
		return ErrFileAgentClosed
	}
	return nil
}

// generateWatchID creates a unique watch ID.
func (f *FileAgentImpl) generateWatchID() FileWatchID {
	n := f.nextWatchID.Add(1)
	return FileWatchID(fmt.Sprintf("watch-%d", n))
}

// fsOpToFileOp converts an fsnotify operation to a FileOp.
func fsOpToFileOp(op fsnotify.Op) FileOp {
	switch {
	case op.Has(fsnotify.Create):
		return FileOpCreate
	case op.Has(fsnotify.Write):
		return FileOpWrite
	case op.Has(fsnotify.Remove):
		return FileOpRemove
	case op.Has(fsnotify.Rename):
		return FileOpRename
	case op.Has(fsnotify.Chmod):
		return FileOpChmod
	default:
		return FileOpWrite
	}
}
