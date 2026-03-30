package node

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sync"
	"text/template"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// OutputNode 는 메시지를 포맷팅하여 로그로 출력하는 노드이다.
// 템플릿이 지정되면 Go text/template으로 메시지를 구성하고,
// 미지정 시 전체 페이로드를 JSON으로 출력한다. 메시지는 그대로 통과한다 (pass-through).
type OutputNode struct {
	*BaseNode
	prefix   string
	tmpl     *template.Template // nil이면 전체 페이로드 JSON
	filePath string             // 파일 출력 경로 (빈 문자열이면 파일 출력 안 함)
	file     *os.File
	mu       sync.RWMutex
}

// NewOutputNode 는 새로운 OutputNode를 생성하는 팩토리 함수이다.
func NewOutputNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &OutputNode{
		BaseNode: base,
		prefix:   "[output]",
	}
	return n, nil
}

// Init 은 OutputNode를 초기화한다.
func (n *OutputNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	n.mu.Lock()
	if n.filePath != "" {
		f, err := os.OpenFile(n.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			n.mu.Unlock()
			return fmt.Errorf("output: open file %q: %w", n.filePath, err)
		}
		n.file = f
	}
	n.mu.Unlock()

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Configure 는 OutputNode의 설정을 적용한다.
func (n *OutputNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if v, ok := config["prefix"].(string); ok && v != "" {
		n.prefix = v
	}

	if v, ok := config["template"].(string); ok && v != "" {
		tmpl, err := template.New("output").Parse(v)
		if err != nil {
			return fmt.Errorf("output: invalid template: %w", err)
		}
		n.tmpl = tmpl
	} else {
		n.tmpl = nil
	}

	if fp, ok := config["file"].(string); ok && fp != "" {
		n.filePath = fp
	}

	return nil
}

// Process 는 메시지를 포맷팅하여 로그에 출력한 뒤 그대로 통과시킨다.
func (n *OutputNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	prefix := n.prefix
	tmpl := n.tmpl
	n.mu.RUnlock()

	var formatted string

	if tmpl != nil {
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, msg.Payload().ToMap()); err != nil {
			// 템플릿 실행 실패 시 경고 후 페이로드 JSON으로 폴백
			logger := n.BaseNode.Logger()
			if logger != nil {
				logger.Warn("output: template execution failed, falling back to JSON",
					"error", err)
			}
			data, _ := msg.Payload().ToJSON()
			formatted = string(data)
		} else {
			formatted = buf.String()
		}
	} else {
		data, err := msg.Payload().ToJSON()
		if err != nil {
			formatted = fmt.Sprintf("%v", msg.Payload().ToMap())
		} else {
			formatted = string(data)
		}
	}

	logger := n.BaseNode.Logger()
	if logger != nil {
		logger.Info(formatted, "prefix", prefix)
	}

	n.mu.RLock()
	f := n.file
	n.mu.RUnlock()
	if f != nil {
		fmt.Fprintf(f, "%s %s\n", prefix, formatted)
	}

	return []message.Message{msg}, nil
}

// Shutdown 은 OutputNode를 종료한다.
func (n *OutputNode) Shutdown(ctx context.Context) error {
	n.mu.Lock()
	if n.file != nil {
		n.file.Close()
		n.file = nil
	}
	n.mu.Unlock()
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}
