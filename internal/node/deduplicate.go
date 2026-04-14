package node

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// DeduplicateNode 는 지정 시간 창(window) 내에서 동일한 메시지를 제거하는 노드이다.
//
// 설정:
//   - key: 메시지 그룹핑 키 (페이로드 필드명). 빈 문자열이면 전체 메시지 기준.
//   - window: 중복 억제 시간 (예: "30s"). 기본값 30초.
//   - compare_fields: 비교 대상 필드 목록 (콤마 구분). 비어있으면 전체 페이로드 비교.
//   - on_duplicate: 중복 시 처리. "drop"(기본, 폐기) 또는 "reject_port"(reject 포트로 전달).
//
// 동작:
//   - key별로 마지막 통과 메시지의 지문(fingerprint)과 시각을 저장한다.
//   - 새 메시지의 지문이 동일하고 window 내이면 중복으로 판정한다.
//   - window 초과 시 값이 동일해도 강제 통과 (주기적 갱신 보장).
type DeduplicateNode struct {
	*BaseNode

	mu            sync.RWMutex
	key           string        // 그룹핑 키 필드명
	window        time.Duration // 중복 억제 시간
	compareFields []string      // 비교 대상 필드 (비어있으면 전체)
	onDuplicate   string        // "drop" 또는 "reject_port"

	// 키별 마지막 통과 상태
	state map[string]*deduplicateEntry
}

// deduplicateEntry 는 키별 마지막 통과 메시지 상태이다.
type deduplicateEntry struct {
	fingerprint string
	lastSeen    time.Time
}

// NewDeduplicateNode 는 새 DeduplicateNode를 생성한다.
func NewDeduplicateNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &DeduplicateNode{
		BaseNode:    base,
		window:      30 * time.Second,
		onDuplicate: "drop",
		state:       make(map[string]*deduplicateEntry),
	}
	return n, nil
}

// Init 은 노드를 초기화한다.
func (n *DeduplicateNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 메시지의 중복 여부를 판정한다.
func (n *DeduplicateNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	keyField := n.key
	window := n.window
	fields := n.compareFields
	onDup := n.onDuplicate
	n.mu.RUnlock()

	// 그룹핑 키 추출
	groupKey := "_all"
	if keyField != "" {
		if v, ok := msg.Payload().Get(keyField); ok {
			groupKey = fmt.Sprintf("%v", v)
		}
	}

	// 지문 생성
	fp := fingerprint(msg.Payload(), fields)

	now := time.Now()

	n.mu.Lock()
	entry, exists := n.state[groupKey]

	if exists && entry.fingerprint == fp && now.Sub(entry.lastSeen) < window {
		// 중복: window 내 동일 지문
		n.mu.Unlock()

		switch onDup {
		case "reject_port":
			out := msg.Clone()
			out.Metadata().Set("_target_port", "reject")
			return []message.Message{out}, nil
		default:
			// drop: 빈 결과 반환 (폐기)
			return []message.Message{}, nil
		}
	}

	// 신규 또는 변경 또는 window 초과 → 통과
	n.state[groupKey] = &deduplicateEntry{
		fingerprint: fp,
		lastSeen:    now,
	}
	n.mu.Unlock()

	return []message.Message{msg}, nil
}

// Shutdown 은 노드를 종료한다.
func (n *DeduplicateNode) Shutdown(ctx context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Configure 는 노드 설정을 적용한다.
func (n *DeduplicateNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if v, ok := config["key"]; ok {
		if s, ok := v.(string); ok {
			n.key = s
		}
	}

	if v, ok := config["window"]; ok {
		if s, ok := v.(string); ok {
			d, err := time.ParseDuration(s)
			if err != nil {
				return fmt.Errorf("deduplicate: invalid window %q: %w", s, err)
			}
			n.window = d
		}
	}

	if v, ok := config["compare_fields"]; ok {
		if s, ok := v.(string); ok && s != "" {
			parts := strings.Split(s, ",")
			fields := make([]string, 0, len(parts))
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					fields = append(fields, p)
				}
			}
			n.compareFields = fields
		}
	}

	if v, ok := config["on_duplicate"]; ok {
		if s, ok := v.(string); ok {
			n.onDuplicate = s
		}
	}

	return nil
}

// fingerprint 는 페이로드에서 비교 대상 필드의 해시를 생성한다.
func fingerprint(payload message.Payload, fields []string) string {
	h := sha256.New()

	if len(fields) == 0 {
		// 전체 페이로드 비교: 키를 정렬하여 결정적 해시
		m := payload.ToMap()
		keys := make([]string, 0, len(m))
		for k := range m {
			// timestamp 등 항상 변하는 필드 제외
			if k == "timestamp" || k == "seq" || k == "raw_hex" {
				continue
			}
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(h, "%s=%v;", k, m[k])
		}
	} else {
		// 지정 필드만 비교
		for _, f := range fields {
			v, _ := payload.Get(f)
			fmt.Fprintf(h, "%s=%v;", f, v)
		}
	}

	return hex.EncodeToString(h.Sum(nil))
}
