package system

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TextFileOperator defines text-mode file operation interface.
type TextFileOperator interface {
	ReadFileText(path string) (string, error)
	WriteFileText(path string, content string, perm os.FileMode) error
	AppendFileText(path string, content string) error
	ReadLines(path string) ([]string, error)
	ReadLine(path string, lineNum int) (string, error) // 1-based
	WriteLines(path string, lines []string, perm os.FileMode) error
	AppendLine(path string, line string) error
}

// Compile-time interface check.
var _ TextFileOperator = (*FileAgentImpl)(nil)

// checkTextMode returns ErrTextModeRequired if the agent is not in text mode.
func (f *FileAgentImpl) checkTextMode() error {
	if f.mode != "text" {
		return ErrTextModeRequired
	}
	return nil
}

// ReadFileText reads a file as a UTF-8 text string.
func (f *FileAgentImpl) ReadFileText(path string) (string, error) {
	if err := f.checkClosed(); err != nil {
		return "", err
	}
	if err := f.checkTextMode(); err != nil {
		return "", err
	}
	safePath, err := f.resolveSandboxPath(path)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(safePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrFileNotFound
		}
		return "", fmt.Errorf("file agent read text: %w", err)
	}

	f.fileStats.filesRead.Add(1)
	f.fileStats.bytesRead.Add(int64(len(data)))
	return string(data), nil
}

// WriteFileText writes a text string to a file, creating intermediate directories.
func (f *FileAgentImpl) WriteFileText(path string, content string, perm os.FileMode) error {
	if err := f.checkClosed(); err != nil {
		return err
	}
	if err := f.checkTextMode(); err != nil {
		return err
	}
	safePath, err := f.resolveSandboxPath(path)
	if err != nil {
		return err
	}

	dir := filepath.Dir(safePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("file agent write text: mkdir failed: %w", err)
	}

	data := []byte(content)
	if err := os.WriteFile(safePath, data, perm); err != nil {
		return fmt.Errorf("file agent write text: %w", err)
	}

	f.fileStats.filesWritten.Add(1)
	f.fileStats.bytesWritten.Add(int64(len(data)))
	return nil
}

// AppendFileText appends a text string to a file, creating it if needed.
func (f *FileAgentImpl) AppendFileText(path string, content string) error {
	if err := f.checkClosed(); err != nil {
		return err
	}
	if err := f.checkTextMode(); err != nil {
		return err
	}
	safePath, err := f.resolveSandboxPath(path)
	if err != nil {
		return err
	}

	dir := filepath.Dir(safePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("file agent append text: mkdir failed: %w", err)
	}

	file, err := os.OpenFile(safePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("file agent append text: %w", err)
	}
	defer file.Close()

	data := []byte(content)
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("file agent append text: write failed: %w", err)
	}

	f.fileStats.filesWritten.Add(1)
	f.fileStats.bytesWritten.Add(int64(len(data)))
	return nil
}

// ReadLines reads a file and returns its lines as a string slice.
func (f *FileAgentImpl) ReadLines(path string) ([]string, error) {
	if err := f.checkClosed(); err != nil {
		return nil, err
	}
	if err := f.checkTextMode(); err != nil {
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
		return nil, fmt.Errorf("file agent read lines: %w", err)
	}

	f.fileStats.filesRead.Add(1)
	f.fileStats.bytesRead.Add(int64(len(data)))

	content := string(data)
	if content == "" {
		return []string{}, nil
	}
	lines := strings.Split(content, "\n")
	// Remove trailing empty element from final newline.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines, nil
}

// ReadLine reads a single line from a file (1-based line number).
func (f *FileAgentImpl) ReadLine(path string, lineNum int) (string, error) {
	if err := f.checkClosed(); err != nil {
		return "", err
	}
	if err := f.checkTextMode(); err != nil {
		return "", err
	}

	lines, err := f.ReadLines(path)
	if err != nil {
		return "", err
	}
	if lineNum < 1 || lineNum > len(lines) {
		return "", ErrLineOutOfRange
	}
	return lines[lineNum-1], nil
}

// WriteLines writes a slice of strings as lines to a file.
func (f *FileAgentImpl) WriteLines(path string, lines []string, perm os.FileMode) error {
	if err := f.checkClosed(); err != nil {
		return err
	}
	if err := f.checkTextMode(); err != nil {
		return err
	}

	content := strings.Join(lines, "\n")
	if len(lines) > 0 {
		content += "\n"
	}
	return f.WriteFileText(path, content, perm)
}

// AppendLine appends a single line to a file.
func (f *FileAgentImpl) AppendLine(path string, line string) error {
	if err := f.checkClosed(); err != nil {
		return err
	}
	if err := f.checkTextMode(); err != nil {
		return err
	}

	return f.AppendFileText(path, line+"\n")
}
