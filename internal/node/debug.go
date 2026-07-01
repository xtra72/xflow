package node

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
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
//
// 출력 필드 지정 (property):
//   - "" (미지정): 메시지 전체
//   - ".payload": payload 맵 전체
//   - ".payload.name": payload 내 특정 필드 (GetPath 사용)
//   - ".metadata": metadata 맵 전체
//   - ".id": 메시지 ID
//   - ".timestamp": 타임스탬프
//
// 출력 형식 (format):
//   - "json" (기본값): JSON 포맷
//   - "plain": 읽기 쉬운 텍스트 (숫자→문자열, 문자열→그대로, []byte→hex)
//   - "raw": 바이너리 그대로 ([]byte→바이트, 그 외→JSON 바이트)
//   - "text": plain의 별칭 (하위 호환)
//
// 표시 항목 (displayFields):
//   - 미지정 시 전체 출력
//   - "time,level,name,message" 등 쉼표 구분 항목 지정
type DebugNode struct {
	*BaseNode
	logLevel      string // "debug", "info", "warn"
	prefix        string // 출력 프리픽스 (기본값: 노드 이름)
	tmpl          *template.Template
	outputDest    string // "slog", "logger", "terminal", "file", "editor"
	fields        []string
	property      string   // 메시지 경로 (예: ".payload", ".payload.name")
	format        string   // 출력 형식 ("json", "plain", "raw", "text")
	displayFields []string // 표시 항목 (예: ["time", "level", "name", "message"])
	filePath      string   // 파일 출력 경로 (빈 문자열이면 파일 출력 안 함)
	file          *os.File
	sink          DebugSink
	resolver      AgentResolver  // 에이전트 resolver (엔진에서 주입)
	transport     AgentTransport // output=logger 시 에이전트 transport
	agentRef      string         // config["agent_ref"] 에이전트 참조
	// outputEnabled (v0.18.9): 출력 활성화 여부. false 시 emit 을 건너뛰고
	// 메시지는 그대로 통과시킨다. 에디터에서 패널 펼치지 않고 ON/OFF 토글 가능.
	outputEnabled bool
	mu            sync.RWMutex
}

// NewDebugNode 는 새로운 DebugNode를 생성하는 팩토리 함수이다.
func NewDebugNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &DebugNode{
		BaseNode:      base,
		logLevel:      "debug",     // 기본 레벨
		outputDest:    "slog",      // 기본 출력 대상
		prefix:        base.Name(), // 기본 프리픽스는 노드 이름
		outputEnabled: true,        // v0.18.9: 기본 활성화
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
//
// 출력 우선순위:
//  1. format이 설정되어 있으면 format 모드로 동작 (json/plain/raw)
//  2. template이 설정되어 있으면 Go template 모드 (하위 호환)
//  3. 그 외 기본 포맷: "message id=... payload=... metadata=..."
func (n *DebugNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	level := n.logLevel
	prefix := n.prefix
	tmpl := n.tmpl
	dest := n.outputDest
	fields := n.fields
	prop := n.property
	format := n.format
	dispFields := n.displayFields
	sink := n.sink
	transport := n.transport
	outputEnabled := n.outputEnabled
	n.mu.RUnlock()

	// v0.18.9: 출력이 비활성화되어 있으면 emit 을 skip 하고 메시지만 통과.
	if !outputEnabled {
		return []message.Message{msg}, nil
	}

	// format이 설정되어 있으면 새 포맷 모드
	if format != "" {
		output := n.formatProcess(msg, prop, format, dispFields)

		if prefix != "" {
			output = prefix + " " + output
		}

		n.emitOutput(ctx, dest, level, output, msg, sink, transport)
		return []message.Message{msg}, nil
	}

	// 하위 호환: 기존 fields 필터링 + template 로직
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

	var formatted string
	if tmpl != nil {
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, payload); err != nil {
			logger := n.BaseNode.Logger()
			if logger != nil {
				logger.Warn("debug: template execution failed, falling back to default format",
					"error", err)
			}
			formatted = fmt.Sprintf("message id=%s payload=%v metadata=%v",
				msg.ID(), payload, slimEgressMetadata(msg))
		} else {
			formatted = buf.String()
		}
	} else {
		formatted = fmt.Sprintf("message id=%s payload=%v metadata=%v",
			msg.ID(), payload, msg.Metadata().All())
	}

	var output string
	if prefix != "" {
		output = fmt.Sprintf("%s %s", prefix, formatted)
	} else {
		output = formatted
	}

	n.emitOutput(ctx, dest, level, output, msg, sink, transport)
	return []message.Message{msg}, nil
}

// formatProcess 는 format 모드에서 메시지를 포맷팅한다.
//
// property가 있으면 해당 경로의 값만 직렬화하여 반환한다.
// property가 없으면 display_fields에 따라 로그 라인을 구성한다:
//   - display_fields 미지정: "시간 레벨 노드이름 메시지값" (plain) 또는 JSON 전체 (json)
//   - display_fields 지정: 지정된 항목만 "key=value" 형태로
func (n *DebugNode) formatProcess(msg message.Message, prop, format string, dispFields []string) string {
	if prop != "" {
		target := extractProperty(msg, prop)

		// displayFields로 map 필터링 (property 결과가 map일 때)
		if len(dispFields) > 0 {
			if m, ok := target.(map[string]any); ok {
				filtered := make(map[string]any, len(dispFields))
				for _, key := range dispFields {
					if v, exists := m[key]; exists {
						filtered[key] = v
					}
				}
				target = filtered
			}
		}

		switch format {
		case "json":
			return formatValue(target, format)
		default: // "plain", "text", "raw"
			// display_fields 미지정: 기본 로그 라인 (시간 레벨 이름 메시지값)
			// 메시지값 = property로 추출한 값
			if len(dispFields) == 0 {
				// P2: property=.metadata(전체) 는 단일 키여도 전체 맵(키 포함)을 보여준다.
				// flattenValue 의 단일 키 평탄화는 .payload.X 같은 값 추출용이며,
				// metadata 루트에는 적용하지 않는다(이전엔 map[string]string 타입이라 우연히
				// 평탄화를 피했음 — Raw() 로 전환하며 동작 보존을 명시화).
				if prop == ".metadata" {
					return buildLogLineWithValue(n, msg, format, formatValue(target, format))
				}
				return buildLogLineWithValue(n, msg, format, flattenValue(target, format))
			}
			return formatValue(target, format)
		}
	}

	// property 없음: 메시지 전체 출력
	switch format {
	case "json":
		return formatJSON(n.buildMessageMap(msg, dispFields))
	default: // "plain", "text", "raw"
		return buildLogLine(n, msg, format, dispFields)
	}
}

// flattenValue 는 map에 키가 하나뿐이면 그 값만 추출하여 포맷팅한다.
// 키가 여러 개이면 전체를 포맷팅한다.
func flattenValue(v any, format string) string {
	if m, ok := v.(map[string]any); ok && len(m) == 1 {
		for _, val := range m {
			return formatValue(val, format)
		}
	}
	return formatValue(v, format)
}

// buildLogLineWithValue 는 기본 로그 라인에 지정된 메시지값을 사용한다.
// 형식: "시간 레벨 이름 메시지값"
func buildLogLineWithValue(n *DebugNode, msg message.Message, _ string, value string) string {
	n.mu.RLock()
	lvl := n.logLevel
	n.mu.RUnlock()

	return msg.Timestamp().Format("2006-01-02T15:04:05.999Z07:00") + " " +
		strings.ToUpper(lvl) + " " +
		n.Name() + " " +
		value
}

// buildMessageMap 은 메시지를 맵으로 구성한다.
// displayFields가 지정되면 해당 항목만 포함한다.
// 지원 항목: time, level, name, id, payload, metadata 및 payload 내 키
func (n *DebugNode) buildMessageMap(msg message.Message, dispFields []string) map[string]any {
	n.mu.RLock()
	lvl := n.logLevel
	n.mu.RUnlock()

	// v0.16.1: "timestamp" alias 추가 — display_fields 에서 time/timestamp 모두 동작.
	// v0.16.2: timestamp 는 epoch ms (int64) — time 은 RFC3339 string (human-readable) 유지.
	all := map[string]any{
		"id":        msg.ID(),
		"type":      msg.Type(), // v0.12.0
		"time":      msg.Timestamp().Format("2006-01-02T15:04:05.999Z07:00"),
		"timestamp": msg.Timestamp().UnixMilli(),
		"level":     lvl,
		"name":      n.Name(),
		"payload":   msg.Payload().ToMap(),
		// P2: Raw() 로 nested group 을 중첩 객체로 노출(flat 키는 문자열 유지).
		// message-slim-metadata: 외부 egress 이므로 agent/device 그룹은 id-only 슬림.
		"metadata": slimEgressMetadata(msg),
	}

	if len(dispFields) == 0 {
		return all
	}

	filtered := make(map[string]any, len(dispFields))
	for _, key := range dispFields {
		if v, exists := all[key]; exists {
			filtered[key] = v
		}
		// payload 내 키도 직접 참조 가능 (예: "message" → payload["message"])
		if _, exists := filtered[key]; !exists {
			if v, ok := msg.Payload().Get(key); ok {
				filtered[key] = v
			}
		}
	}
	return filtered
}

// buildLogLine 은 plain/text 포맷에서 로그 라인을 구성한다.
//
// display_fields 미지정(기본): "시간 레벨 노드이름 payload metadata"
//   - v0.7.12: payload 외에 metadata 도 함께 출력 (메시지 전체 노출).
//     이전 동작은 payload 만 — 사용자 요구: "출력 필드 미지정 시 메시지 전체".
//   - metadata 가 비어 있으면 ({} 또는 길이 0) 끝의 공백 + 빈 객체 노이즈를
//     피하기 위해 추가하지 않는다.
//
// display_fields 지정: 지정된 항목을 순서대로 출력
//   - 지원 항목: time, level, name, id, payload, metadata 및 payload 내 키
func buildLogLine(n *DebugNode, msg message.Message, format string, dispFields []string) string {
	// 항목 값 참조 테이블
	resolveField := func(key string) string {
		switch key {
		case "time":
			// human-readable RFC3339.
			return msg.Timestamp().Format("2006-01-02T15:04:05.999Z07:00")
		case "timestamp":
			// v0.16.2: epoch ms (int64).
			return strconv.FormatInt(msg.Timestamp().UnixMilli(), 10)
		case "level":
			n.mu.RLock()
			lvl := n.logLevel
			n.mu.RUnlock()
			return strings.ToUpper(lvl)
		case "name":
			return n.Name()
		case "id":
			return msg.ID()
		case "type":
			return msg.Type()
		case "payload":
			return formatValue(msg.Payload().ToMap(), format)
		case "metadata":
			// P2: Raw() 로 nested group 포함.
			// message-slim-metadata: 외부 egress 이므로 agent/device 그룹 id-only 슬림.
			return formatValue(slimEgressMetadata(msg), format)
		default:
			// payload 내 키 직접 참조
			if v, ok := msg.Payload().Get(key); ok {
				return formatValue(v, format)
			}
			return ""
		}
	}

	if len(dispFields) == 0 {
		// v0.7.13: 기본 = "time level name + 단일 JSON 객체" 형식.
		// v0.12.0: msg.Type() 추가 (metadata.message_type 에서 top-level 로 promote).
		// v0.16.1: msg.Timestamp() 도 top-level 로 포함 (이전엔 prefix 의 time 만).
		// v0.16.2: timestamp 는 epoch ms (int64) — payload 의 last_seen_ms 와 동일 형식.
		body := map[string]any{
			"id":        msg.ID(),
			"type":      msg.Type(),
			"timestamp": msg.Timestamp().UnixMilli(),
			"payload":   msg.Payload().ToMap(),
			// P2: Raw() 로 nested group 포함.
			// message-slim-metadata: 외부 egress 이므로 agent/device 그룹 id-only 슬림.
			"metadata": slimEgressMetadata(msg),
		}
		return resolveField("time") + " " +
			resolveField("level") + " " +
			resolveField("name") + " " +
			formatJSON(body)
	}

	// display_fields 지정: 순서대로 공백 구분 출력
	parts := make([]string, 0, len(dispFields))
	for _, key := range dispFields {
		if v := resolveField(key); v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " ")
}

// extractProperty 는 메시지에서 property 경로에 해당하는 값을 추출한다.
// property 형식: ".payload", ".payload.name", ".metadata", ".id", ".timestamp"
func extractProperty(msg message.Message, prop string) any {
	// 선행 점(.) 제거 후 파트 분리
	path := strings.TrimPrefix(prop, ".")
	parts := strings.SplitN(path, ".", 2)
	root := parts[0]

	switch root {
	case "payload":
		if len(parts) == 1 {
			return msg.Payload().ToMap()
		}
		// 나머지 경로를 JSONPath로 조회
		val, err := msg.Payload().GetPath("$." + parts[1])
		if err != nil {
			return nil
		}
		return val
	case "metadata":
		if len(parts) == 1 {
			// P2: Raw() 로 nested group 까지 노출(property=.metadata 가 group 을 보여준다).
			// message-slim-metadata: 외부 egress 이므로 agent/device 그룹 id-only 슬림.
			return slimEgressMetadata(msg)
		}
		// 특정 메타데이터 키
		if v, ok := msg.Metadata().Get(parts[1]); ok {
			return v
		}
		return nil
	case "id":
		return msg.ID()
	case "type":
		// v0.12.0: message.type 직접 접근
		return msg.Type()
	case "timestamp":
		return msg.Timestamp().Format("2006-01-02T15:04:05.999999999Z07:00")
	default:
		// 알 수 없는 경로: payload에서 시도
		val, err := msg.Payload().GetPath("$." + path)
		if err != nil {
			return nil
		}
		return val
	}
}

// formatValue 는 값을 지정된 포맷으로 문자열 변환한다.
//   - "json": JSON 포맷 (들여쓰기 없음)
//   - "plain", "text": 숫자→문자열, 문자열→그대로, []byte→hex, map/slice→JSON
//   - "raw": []byte→그대로, 그 외→JSON
func formatValue(v any, format string) string {
	if v == nil {
		return ""
	}

	switch format {
	case "json":
		return formatJSON(v)
	case "raw":
		return formatRaw(v)
	default: // "plain", "text"
		return formatText(v)
	}
}

// formatJSON 은 값을 JSON 문자열로 변환한다.
func formatJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// formatRaw 는 값을 raw 포맷으로 변환한다.
// []byte는 바이트 그대로 문자열화, 그 외는 JSON 직렬화.
func formatRaw(v any) string {
	switch val := v.(type) {
	case []byte:
		return string(val)
	case string:
		return val
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

// formatText 는 값을 읽기 쉬운 텍스트 포맷으로 변환한다.
// 숫자→문자열, 문자열→그대로(바이너리 포함 시 hex), []byte→hex 문자열, map/slice→JSON.
func formatText(v any) string {
	switch val := v.(type) {
	case []byte:
		return hex.EncodeToString(val)
	case string:
		if hasBinaryContent(val) {
			return hex.EncodeToString([]byte(val))
		}
		return val
	case bool:
		return fmt.Sprintf("%t", val)
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", val)
	case float32:
		return fmt.Sprintf("%g", val)
	case float64:
		return fmt.Sprintf("%g", val)
	case map[string]any:
		sanitized := sanitizeBinaryInMap(val)
		b, err := json.Marshal(sanitized)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(b)
	case []any:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(b)
	case map[string]string:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(b)
	default:
		// 기타 타입: JSON 시도 후 실패하면 Sprint
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(b)
	}
}

// hasBinaryContent 는 문자열에 비출력 바이너리 문자가 포함되어 있는지 확인한다.
// 탭, 개행 등 일반 제어 문자는 허용하고, 나머지 제어 문자가 있으면 바이너리로 판단한다.
func hasBinaryContent(s string) bool {
	for _, b := range []byte(s) {
		if b < 0x20 && b != '\t' && b != '\n' && b != '\r' {
			return true
		}
	}
	return false
}

// sanitizeBinaryInMap 은 map 내 바이너리 문자열 값을 hex 문자열로 변환한다.
func sanitizeBinaryInMap(m map[string]any) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case string:
			if hasBinaryContent(val) {
				result[k] = hex.EncodeToString([]byte(val))
			} else {
				result[k] = val
			}
		case []byte:
			result[k] = hex.EncodeToString(val)
		case map[string]any:
			result[k] = sanitizeBinaryInMap(val)
		default:
			result[k] = val
		}
	}
	return result
}

// emitOutput 는 포맷팅된 출력을 지정된 대상으로 전송한다.
func (n *DebugNode) emitOutput(ctx context.Context, dest, level, output string, msg message.Message, sink DebugSink, transport AgentTransport) {
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
			n.logAtLevel(level, output)
		}
	case "logger":
		if transport != nil {
			_ = transport.Send(ctx, msg)
		} else {
			n.logAtLevel(level, output)
		}
	default: // "slog"
		n.logAtLevel(level, output)
	}
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
//   - "property": 메시지 경로 (string: ".payload", ".payload.name", ".metadata", ".id", ".timestamp")
//   - "format": 출력 포맷 (string: "text", "raw")
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

	// v0.18.9: output_enabled — 출력 활성화 토글. 에디터에서 패널 펼치지 않고
	// 노드 카드의 ON/OFF 버튼으로 직접 토글 가능. false 시 emit 만 skip,
	// 메시지는 그대로 통과.
	if v, ok := config["output_enabled"].(bool); ok {
		n.outputEnabled = v
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

	if v, ok := config["property"].(string); ok {
		n.property = v
	}

	if v, ok := config["format"].(string); ok {
		switch v {
		case "json", "plain", "raw", "text":
			n.format = v
		}
	}

	if v, ok := config["display_fields"].(string); ok && v != "" {
		parts := strings.Split(v, ",")
		df := make([]string, 0, len(parts))
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				df = append(df, trimmed)
			}
		}
		n.displayFields = df
	} else if v, ok := config["display_fields"].([]any); ok {
		df := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				df = append(df, s)
			}
		}
		n.displayFields = df
	}

	return nil
}
