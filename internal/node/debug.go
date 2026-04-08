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

// DebugSink 는 에디터 등 외부 출력 대상으로 디버그 메시지를 전송하는 인터페이스이다.
type DebugSink interface {
	SendDebug(nodeID string, message string) error
}

// DebugNode 는 메시지 내용을 로깅하는 디버그 노드이다.
// 메시지를 포맷팅하여 지정된 출력 대상으로 전송한 뒤 그대로 통과시킨다 (pass-through).
// Output 노드의 기능(템플릿, 프리픽스, 포맷팅)을 통합하여 제공한다.
type DebugNode struct {
	*BaseNode
	logLevel   string // "debug", "info", "warn"
	prefix     string // 출력 프리픽스 (기본값: 노드 이름)
	tmpl       *template.Template
	outputDest string // "slog", "logger", "terminal", "file", "editor"
	fields     []string
	filePath   string // 파일 출력 경로 (빈 문자열이면 파일 출력 안 함)
	file       *os.File
	sink       DebugSink
	resolver   AgentResolver  // 에이전트 resolver (엔진에서 주입)
	transport  AgentTransport // output=logger 시 에이전트 transport
	agentRef   string         // config["agent_ref"] 에이전트 참조
	mu         sync.RWMutex
}

// NewDebugNode 는 새로운 DebugNode를 생성하는 팩토리 함수이다.
func NewDebugNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &DebugNode{
		BaseNode:   base,
		logLevel:   "debug",  // 기본 레벨
		outputDest: "slog",   // 기본 출력 대상
		prefix:     base.Name(), // 기본 프리픽스는 노드 이름
	}

	// 엔진에서 주입된 AgentResolver 추출
	if base.config != nil {
		if r, ok := base.config["_agent_resolver"]; ok {
			if resolver, ok := r.(AgentResolver); ok {
				n.resolver = resolver
			}
		}
	}

	return n, nil
}

// Init 은 DebugNode를 초기화한다.
func (n *DebugNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	n.mu.Lock()
	if n.filePath != "" {
		f, err := os.OpenFile(n.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			n.mu.Unlock()
			return fmt.Errorf("debug: open file %q: %w", n.filePath, err)
		}
		n.file = f
	}

	// output=logger 시 에이전트 resolve
	if n.outputDest == "logger" && n.agentRef != "" && n.resolver != nil {
		ref := flow.AgentRef{AgentName: n.agentRef}
		transport, err := n.resolver.ResolveAgent(ctx, ref)
		if err != nil {
			n.mu.Unlock()
			return fmt.Errorf("debug: resolve agent %q: %w", n.agentRef, err)
		}
		n.transport = transport
	}
	n.mu.Unlock()

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 메시지를 포맷팅하여 출력 대상으로 전송한 뒤 그대로 통과시킨다.
// 로거가 nil이면 로깅을 건너뛰고 메시지만 통과시킨다.
func (n *DebugNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	level := n.logLevel
	prefix := n.prefix
	tmpl := n.tmpl
	dest := n.outputDest
	fields := n.fields
	sink := n.sink
	transport := n.transport
	n.mu.RUnlock()

	// 필드 필터링: fields가 설정되어 있으면 페이로드에서 지정된 키만 추출
	payload := msg.Payload().ToMap()
	if len(fields) > 0 {
		filtered := make(map[string]any, len(fields))
		for _, key := range fields {
			if v, ok := payload[key]; ok {
				filtered[key] = v
			}
		}
		payload = filtered
	}

	// 메시지 포맷팅
	var formatted string
	if tmpl != nil {
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, payload); err != nil {
			// 템플릿 실행 실패 시 경고 후 기본 포맷으로 폴백
			logger := n.BaseNode.Logger()
			if logger != nil {
				logger.Warn("debug: template execution failed, falling back to default format",
					"error", err)
			}
			formatted = fmt.Sprintf("message id=%s payload=%v metadata=%v",
				msg.ID(), payload, msg.Metadata().All())
		} else {
			formatted = buf.String()
		}
	} else {
		formatted = fmt.Sprintf("message id=%s payload=%v metadata=%v",
			msg.ID(), payload, msg.Metadata().All())
	}

	// 프리픽스 적용
	var output string
	if prefix != "" {
		output = fmt.Sprintf("%s %s", prefix, formatted)
	} else {
		output = formatted
	}

	// 출력 대상으로 전송
	switch dest {
	case "terminal":
		fmt.Fprintln(os.Stdout, output)
	case "file":
		n.mu.RLock()
		f := n.file
		n.mu.RUnlock()
		if f != nil {
			fmt.Fprintln(f, output)
		}
	case "editor":
		if sink != nil {
			_ = sink.SendDebug(n.BaseNode.ID(), output)
		} else {
			// sink가 nil이면 slog로 폴백
			n.logAtLevel(level, output)
		}
	case "logger":
		if transport != nil {
			_ = transport.Send(ctx, msg)
		} else {
			// transport가 nil이면 slog로 폴백
			n.logAtLevel(level, output)
		}
	default: // "slog"
		n.logAtLevel(level, output)
	}

	return []message.Message{msg}, nil
}

// SetDebugSink 는 에디터 출력용 DebugSink를 설정한다.
func (n *DebugNode) SetDebugSink(sink DebugSink) {
	n.mu.Lock()
	n.sink = sink
	n.mu.Unlock()
}

// logAtLevel 은 지정된 레벨에 따라 로그를 출력한다.
func (n *DebugNode) logAtLevel(level, msg string) {
	logger := n.BaseNode.Logger()
	if logger == nil {
		return
	}
	switch level {
	case "info":
		logger.Info(msg)
	case "warn":
		logger.Warn(msg)
	default:
		logger.Debug(msg)
	}
}

// Shutdown 은 DebugNode를 종료한다.
func (n *DebugNode) Shutdown(ctx context.Context) error {
	n.mu.Lock()
	if n.file != nil {
		n.file.Close()
		n.file = nil
	}
	n.mu.Unlock()
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Configure 는 DebugNode의 설정을 적용한다.
// 지원하는 설정 키:
//   - "level": 로그 레벨 (string: "debug", "info", "warn")
//   - "file": 파일 출력 경로 (string)
//   - "prefix": 출력 프리픽스 (string, 기본값: 노드 이름)
//   - "template": Go text/template 포맷 문자열 (string)
//   - "output": 출력 대상 (string: "slog", "logger", "terminal", "file", "editor")
//   - "fields": 페이로드에서 출력할 필드 목록 ([]any of string)
//   - "agent_ref": 에이전트 참조 (string, output=logger 시 필요)
func (n *DebugNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}
	n.mu.Lock()
	defer n.mu.Unlock()

	if lvl, ok := config["level"]; ok {
		if levelStr, ok := lvl.(string); ok {
			n.logLevel = levelStr
		}
	}

	if fp, ok := config["file"].(string); ok && fp != "" {
		n.filePath = fp
	}

	if v, ok := config["prefix"].(string); ok && v != "" {
		n.prefix = v
	}

	if v, ok := config["template"].(string); ok && v != "" {
		tmpl, err := template.New("debug").Parse(v)
		if err != nil {
			return fmt.Errorf("debug: invalid template: %w", err)
		}
		n.tmpl = tmpl
	} else if _, exists := config["template"]; exists {
		// template 키가 존재하지만 빈 문자열이면 템플릿 해제
		n.tmpl = nil
	}

	if v, ok := config["output"].(string); ok && v != "" {
		n.outputDest = v
	}

	if v, ok := config["agent_ref"].(string); ok && v != "" {
		n.agentRef = v
	}

	if v, ok := config["fields"]; ok {
		if fieldList, ok := v.([]any); ok {
			fields := make([]string, 0, len(fieldList))
			for _, f := range fieldList {
				if s, ok := f.(string); ok {
					fields = append(fields, s)
				}
			}
			n.fields = fields
		}
	}

	return nil
}
