// Package node 의 framer_factory.go 는 framer 노드의 Registry 팩토리 함수와
// 옵션 파싱 로직을 제공한다 (SPEC-NODE-002 Phase 3).
//
// 팩토리는 NodeDef.Config 맵에서 framing 모드와 관련 옵션을 추출하고 검증한
// 후 NewFramerNode 를 호출하여 완전히 구성된 FramerNode 를 반환한다.
package node

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/framing"
)

// validFramingModes 는 framer 노드에서 허용하는 프레이밍 모드 목록이다.
var validFramingModes = map[string]bool{
	framing.ModeRaw:          true,
	framing.ModeNewline:      true,
	framing.ModeLengthPrefix: true,
	framing.ModeFixedSize:    true,
	framing.ModeStream:       true,
	framing.ModeFrame:        true,
}

// framerFactory 는 Registry 에 등록되는 framer 노드 팩토리 함수이다.
// NodeDef.Config 에서 모든 framing 옵션과 노드 고유 옵션을 추출하고 검증한다.
func framerFactory(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	cfg := def.Config

	// 1) framing 모드 (필수)
	mode, ok := configString(cfg, "framing")
	if !ok || mode == "" {
		return nil, fmt.Errorf("framer: 'framing' option is required")
	}
	if !validFramingModes[mode] {
		return nil, fmt.Errorf("framer: invalid framing mode %q; valid modes: raw, newline, length_prefix, fixed_size, stream, frame", mode)
	}

	// 2) framing.Options 구성
	fopts := framing.Options{}

	// buffer_size
	if v, ok := configInt(cfg, "buffer_size"); ok {
		if v < 0 {
			return nil, fmt.Errorf("framer: buffer_size must be non-negative, got %d", v)
		}
		fopts.BufferSize = v
	}

	// fixed_size
	if v, ok := configInt(cfg, "fixed_size"); ok {
		fopts.FixedSize = v
	}

	// max_message_size
	if v, ok := configInt(cfg, "max_message_size"); ok {
		if v < 0 {
			return nil, fmt.Errorf("framer: max_message_size must be non-negative, got %d", v)
		}
		fopts.MaxMessageSize = v
	}

	// delimiter (string → byte)
	if s, ok := configString(cfg, "delimiter"); ok && len(s) > 0 {
		fopts.Delimiter = s[0]
	}

	// stx (hex string → []byte)
	if s, ok := configString(cfg, "stx"); ok {
		b, err := hex.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("framer: invalid hex for stx: %w", err)
		}
		fopts.STX = b
	}

	// etx (hex string → []byte)
	if s, ok := configString(cfg, "etx"); ok {
		b, err := hex.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("framer: invalid hex for etx: %w", err)
		}
		fopts.ETX = b
	}

	// length_offset
	if v, ok := configInt(cfg, "length_offset"); ok {
		fopts.LengthOffset = v
	}

	// length_size
	if v, ok := configInt(cfg, "length_size"); ok {
		fopts.LengthSize = v
	}

	// length_endian
	if s, ok := configString(cfg, "length_endian"); ok {
		if s != "big" && s != "little" {
			return nil, fmt.Errorf("framer: length_endian must be \"big\" or \"little\", got %q", s)
		}
		fopts.LengthEndian = s
	}

	// length_includes_header
	if v, ok := configBool(cfg, "length_includes_header"); ok {
		fopts.LengthIncludesHeader = v
	}

	// length_adjustment
	if v, ok := configInt(cfg, "length_adjustment"); ok {
		fopts.LengthAdjustment = v
	}

	// checksum
	if s, ok := configString(cfg, "checksum"); ok {
		fopts.Checksum = s
	}

	// 3) framing 엔진 검증 (인스턴스는 스트림 생성 시 만들어지므로 여기서는 검증만)
	if _, err := framing.New(mode, fopts); err != nil {
		return nil, fmt.Errorf("framer: %w", err)
	}

	// 4) framerNodeOptions 파싱
	nodeOpts := framerNodeOptions{
		StreamKeyMetadata: defaultStreamKeyMetadata,
	}

	if s, ok := configString(cfg, "stream_key_metadata"); ok && s != "" {
		nodeOpts.StreamKeyMetadata = s
	}

	if s, ok := configString(cfg, "stream_idle_timeout"); ok && s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			return nil, fmt.Errorf("framer: invalid stream_idle_timeout %q: %w", s, err)
		}
		nodeOpts.StreamIdleTimeout = d
	}

	if v, ok := configInt(cfg, "max_streams"); ok {
		if v < 0 {
			return nil, fmt.Errorf("framer: max_streams must be non-negative, got %d", v)
		}
		nodeOpts.MaxStreams = v
	}

	// 5) FramerNode 구성 (NewFramerNode 의 내부 로직을 재구성하여 검증된 옵션 사용)
	base := NewBaseNode(def, opts...)
	n := &FramerNode{
		BaseNode:    base,
		framingMode: mode,
		options:     fopts,
		nodeOptions: nodeOpts,
		streams:     make(map[string]*streamBuffer),
		nowFn:       time.Now,
	}

	return n, nil
}

// --- Config 맵에서 타입 안전한 값 추출 헬퍼 ---

// configString 은 맵에서 string 값을 추출한다.
func configString(cfg map[string]any, key string) (string, bool) {
	v, ok := cfg[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// configInt 는 맵에서 int 값을 추출한다. JSON 디코딩에서 오는 float64,
// Web UI 폼에서 오는 string ("1", "256") 도 처리한다.
func configInt(cfg map[string]any, key string) (int, bool) {
	v, ok := cfg[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	case int64:
		return int(n), true
	case string:
		if n == "" {
			return 0, false
		}
		i, err := strconv.Atoi(n)
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

// configBool 은 맵에서 bool 값을 추출한다. Web UI 에서 오는 string
// ("true", "false") 도 처리한다.
func configBool(cfg map[string]any, key string) (bool, bool) {
	v, ok := cfg[key]
	if !ok {
		return false, false
	}
	switch b := v.(type) {
	case bool:
		return b, true
	case string:
		return strings.EqualFold(b, "true"), true
	default:
		return false, false
	}
}
