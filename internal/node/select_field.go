package node

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// onMissingMode 는 화이트리스트 경로가 메시지에 누락되었을 때의 처리 정책이다.
type onMissingMode string

const (
	// onMissingKeep 은 누락된 경로를 건너뛴다 (출력에 포함하지 않음). 기본값.
	// "ignore" 는 하위 호환 별칭이다.
	onMissingKeep onMissingMode = "keep"
	// onMissingDrop 은 화이트리스트 경로가 하나라도 누락되면 메시지 전체를
	// 드랍한다 (아무것도 emit 하지 않음 — drop_to_port=true 면 "drop" 포트로 emit).
	onMissingDrop onMissingMode = "drop"
	// onMissingFill 은 누락된 경로를 설정된 fill 기본값으로 채워 포함한다.
	onMissingFill onMissingMode = "fill"
)

// onRequiredMissingMode 는 required_fields 중 하나라도 누락되었을 때의 처리 정책이다.
type onRequiredMissingMode string

const (
	// onRequiredErrorPort 는 원본(미변형) 메시지를 엔진 "error" 포트로 라우팅한다 (기본값).
	// filter 의 on_reject=error_port 및 drop_to_port 의 _target_port 컨벤션을 미러링한다.
	// Process 는 Go 에러를 반환하지 않고 메시지를 emit 한다.
	onRequiredErrorPort onRequiredMissingMode = "error_port"
	// onRequiredDrop 은 메시지를 폐기한다 (출력 없음). on_missing=drop 의 폐기 경로 미러링.
	onRequiredDrop onRequiredMissingMode = "drop"
	// onRequiredError 는 Process 에서 누락 필드명을 담은 Go 에러를 반환한다 (엔진 에러 경로).
	onRequiredError onRequiredMissingMode = "error"
)

// legacySelectFieldKeys 는 v0.x 의 3-필터 설계에서 사용하던 제거된 설정 키이다.
// 이 중 하나라도 설정에 존재하면 Configure 가 명시적 마이그레이션 에러를 반환한다.
var legacySelectFieldKeys = []string{
	"payload_filter",
	"payload_fields",
	"metadata_filter",
	"metadata_fields",
	"message_filter",
	"message_fields",
}

// SelectFieldNode 는 메시지에서 지정한 경로만 남기는(화이트리스트) 처리 노드이다.
//
// 단일 통합 화이트리스트 `fields` (path → fill 기본값)를 사용한다. 경로 문법은
// store_write.go / mqtt 노드의 `$.` 템플릿 구문과 동일하다:
//
//   - `$.payload.<dotpath>` — 임의 깊이의 payload 필드.
//     `$.payload.temperature` → 최상위 temperature 유지.
//     `$.payload.state.mode`  → state 안의 mode 만 유지(형제 제거, 컨테이너 보존).
//     `$.payload.state`       → state 서브트리 전체 유지(컨테이너 경로).
//     같은 컨테이너에 전체 경로와 하위 필드 경로가 함께 있으면 전체(whole)가 우선한다.
//   - `$.metadata.<key>`         — top-level string 키 OR group 전체.
//     예) `$.metadata.node_id` → string 보존, `$.metadata.device` → device 그룹 전체.
//   - `$.metadata.<group>.<field>` — group 안의 단일 필드.
//     예) `$.metadata.device.id` → device 그룹에서 id 만 남김. 전체 키가 있으면 전체 우선.
//   - `$.type`                   — 메시지 type 유지. fields 에 없으면 type 은 비워진다("").
//   - `$.id`, `$.timestamp`      — 항상 보존되므로 나열은 no-op(허용).
//
// 단일 화이트리스트 의미: fields 가 비어있지 않으면 필터가 활성화되고, 각 도메인은
// 화이트리스트로 동작한다. `$.payload.*` 경로가 하나도 없으면 payload 전체가 비워지고,
// `$.metadata.*` 가 없으면 metadata 전체가 비워지며, `$.type` 이 없으면 type 이 비워진다.
// fields 가 완전히 비어있을 때만 pass-through(무필터)이다.
//
// 누락 처리(on_missing): keep(기본) / drop / fill 가 모든 화이트리스트 경로에 적용된다.
//   - keep: 누락 경로 건너뜀. fill: 기본값으로 채움(중첩 컨테이너/그룹 생성).
//   - drop: 화이트리스트 경로 중 하나라도 누락되면 메시지 드랍. type 은 droppable 이 아니다.
//
// message-level 에서 id 와 timestamp 는 구조적 필드이므로 항상 보존되며 드랍할 수 없다.
type SelectFieldNode struct {
	*BaseNode

	onMissing onMissingMode

	// dropToPort 가 true 면 drop 결정 시 메시지를 폐기하지 않고 "drop" 포트로 emit 한다.
	// on_missing == drop 일 때만 의미가 있다 (keep/fill 에서는 드랍이 없어 무시됨).
	dropToPort bool

	// active 는 fields 가 비어있지 않아 화이트리스트 필터가 활성화되었는지 여부이다.
	active bool

	// payloadRoot 는 payload 화이트리스트 트리의 루트이다 (children = 최상위 payload 키).
	payloadRoot *payloadNode

	// metadata 화이트리스트 선택 (parseMetadataFields 가 채운다).
	mdTopLevel map[string]string            // 점 없는 키 → fill 기본값 (string 또는 group 전체)
	mdPerGroup map[string]map[string]string // group → (field → fill 기본값)

	// typeKept 는 `$.type` 이 유효 화이트리스트(fields ∪ required_fields)에
	// 포함되어 있는지 여부이다.
	typeKept bool

	// onRequiredMissing 은 required_fields 중 하나라도 누락되었을 때의 처리 정책이다.
	onRequiredMissing onRequiredMissingMode

	// requiredPaths 는 반드시 존재해야 하는 정규화된 $.-경로 목록이다 (정렬됨).
	// 누락 보고의 결정성을 위해 정렬하며, 의미 없는 경로(bare/non-$.)는 제외한다.
	requiredPaths []string

	mu sync.RWMutex // 설정 보호 뮤텍스
}

// payloadNode 는 payload 화이트리스트 트리의 한 노드이다.
//   - whole=true: 이 경로가 컨테이너/leaf 로 명시됨 → 이 지점의 값(서브트리) 전체 유지.
//   - children 이 있으면: 재귀하여 나열된 자식만 유지.
//   - whole 이 children 보다 우선한다(전체 보존이 부분 선택을 이긴다).
type payloadNode struct {
	whole    bool
	fill     string
	children map[string]*payloadNode
}

// 인터페이스 컴파일 체크
var _ Node = (*SelectFieldNode)(nil)

// NewSelectFieldNode 는 새로운 SelectFieldNode를 생성하는 팩토리 함수이다.
func NewSelectFieldNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &SelectFieldNode{
		BaseNode:          base,
		onMissing:         onMissingKeep,       // 기본값
		onRequiredMissing: onRequiredErrorPort, // 기본값
		payloadRoot:       &payloadNode{},
		mdTopLevel:        map[string]string{},
		mdPerGroup:        map[string]map[string]string{},
	}
	return n, nil
}

// Init 은 SelectFieldNode를 초기화한다.
func (n *SelectFieldNode) Init(_ context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Shutdown 은 SelectFieldNode를 종료한다.
func (n *SelectFieldNode) Shutdown(_ context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Configure 는 SelectFieldNode의 설정을 적용한다.
//
// 지원 키:
//   - "fields": map[path]fillDefault (map[string]any / map[string]string 허용)
//   - "on_missing": "keep"(기본) | "drop" | "fill" ("ignore" 는 keep 별칭)
//   - "drop_to_port": bool / "true"|"false" (Web UI 호환)
//
// 제거된 레거시 키(payload_filter/payload_fields/metadata_filter/metadata_fields/
// message_filter/message_fields)가 하나라도 있으면 명시적 마이그레이션 에러를 반환한다.
func (n *SelectFieldNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	// 레거시 키 감지 → 명시적 마이그레이션 에러 (silent no-op 방지).
	for _, k := range legacySelectFieldKeys {
		if _, ok := config[k]; ok {
			return fmt.Errorf(
				"select-field: 제거된 설정 키 %q 가 사용되었습니다. 통합 화이트리스트 'fields' 로 "+
					"마이그레이션하세요 (경로는 $.-prefix). 예: "+
					"payload_fields:{temperature:\"\"} → fields:{\"$.payload.temperature\":\"\"}, "+
					"metadata_fields:{node_id:\"\"} → fields:{\"$.metadata.node_id\":\"\"}, "+
					"message_fields:[type] → fields:{\"$.type\":\"\"}",
				k)
		}
	}

	// on_missing 정책 (기본 keep, "ignore" 는 별칭).
	onMissing := onMissingKeep
	if v, ok := config["on_missing"]; ok {
		if s, ok := v.(string); ok {
			switch s {
			case string(onMissingDrop):
				onMissing = onMissingDrop
			case string(onMissingFill):
				onMissing = onMissingFill
			case string(onMissingKeep), "ignore":
				onMissing = onMissingKeep
			default:
				onMissing = onMissingKeep
			}
		}
	}

	// drop_to_port: bool / "true"|"false" 문자열 모두 허용. 기본 false.
	dropToPort, _ := configBool(config, "drop_to_port")

	// on_required_missing 정책 (기본 error_port).
	onRequiredMissing := onRequiredErrorPort
	if v, ok := config["on_required_missing"]; ok {
		if s, ok := v.(string); ok {
			switch s {
			case string(onRequiredDrop):
				onRequiredMissing = onRequiredDrop
			case string(onRequiredError):
				onRequiredMissing = onRequiredError
			case string(onRequiredErrorPort):
				onRequiredMissing = onRequiredErrorPort
			default:
				onRequiredMissing = onRequiredErrorPort
			}
		}
	}

	// fields(옵션) 와 required_fields(필수) 파싱. 값은 fill 기본값(required 는 무시).
	fields := parseStringMap(config["fields"])
	required := parseStringMap(config["required_fields"])

	// 유효 화이트리스트 = fields ∪ required_fields. projection 은 union 으로 수행한다.
	union := make(map[string]string, len(fields)+len(required))
	for k, v := range fields {
		union[k] = v
	}
	for k, v := range required {
		// 값이 비어있지 않은 fields 의 fill 기본값을 보존하기 위해, 이미 있으면 덮어쓰지 않는다.
		// (required 는 항상 존재해야 하므로 fill 값이 의미 없지만, 동일 경로가 fields 에도
		// 있으면 fields 의 fill 을 유지한다.)
		if _, exists := union[k]; !exists {
			union[k] = v
		}
	}

	// active: union 이 비어있지 않으면 화이트리스트 필터 활성화.
	active := len(union) > 0
	payloadRoot, mdRaw, typeKept := parseSelectFields(union)
	mdTopLevel, mdPerGroup := parseMetadataFields(mdRaw)

	// required presence 체크용 경로 목록 (정규화 + 정렬).
	requiredPaths := normalizeRequiredPaths(required)

	// 원자적 설정 적용.
	n.mu.Lock()
	n.onMissing = onMissing
	n.dropToPort = dropToPort
	n.onRequiredMissing = onRequiredMissing
	n.requiredPaths = requiredPaths
	n.active = active
	n.payloadRoot = payloadRoot
	n.mdTopLevel = mdTopLevel
	n.mdPerGroup = mdPerGroup
	n.typeKept = typeKept
	n.mu.Unlock()

	return nil
}

// normalizeRequiredPaths 는 required_fields 맵의 키를 presence 체크 가능한 정규화된
// $.-경로 목록으로 변환한다. `$.` prefix 가 없거나 bare $.payload / $.metadata 인
// 의미 없는 경로는 제외하며(fields 와 동일한 관대 처리), 누락 보고의 결정성을 위해
// 정렬한다. `$.id` / `$.timestamp` 는 구조적으로 항상 존재하므로 포함해도 무해하지만,
// 노이즈를 줄이기 위해 제외한다.
func normalizeRequiredPaths(required map[string]string) []string {
	paths := make([]string, 0, len(required))
	for path := range required {
		if !strings.HasPrefix(path, "$.") {
			continue // 관대: $. prefix 없으면 무시.
		}
		parts := strings.Split(path[2:], ".")
		switch parts[0] {
		case "payload", "metadata":
			if len(parts) < 2 {
				continue // bare $.payload / $.metadata → 무시.
			}
			paths = append(paths, path)
		case "type":
			paths = append(paths, path)
		case "id", "timestamp":
			// 항상 존재 → required 체크 불필요(무시).
		default:
			// 알 수 없는 root → 무시.
		}
	}
	sort.Strings(paths)
	return paths
}

// Process 는 통합 화이트리스트 정책에 따라 출력 메시지를 구성한다.
//
// id 와 timestamp 는 구조적 필드이므로 항상 원본을 보존한다. 입력 메시지를 in-place 로
// 재구성하여 id/timestamp 를 자연스럽게 보존한다(mapping 노드와 동일 패턴).
func (n *SelectFieldNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	onMissing := n.onMissing
	dropToPort := n.dropToPort
	onRequiredMissing := n.onRequiredMissing
	requiredPaths := n.requiredPaths
	active := n.active
	payloadRoot := n.payloadRoot
	mdTopLevel := n.mdTopLevel
	mdPerGroup := n.mdPerGroup
	typeKept := n.typeKept
	n.mu.RUnlock()

	// fields ∪ required_fields 모두 비어있음 → pass-through.
	if !active {
		return []message.Message{msg}, nil
	}

	// --- 필수 필드 presence 체크 (projection 보다 먼저 수행) ---
	// required 경로 중 하나라도 누락되면 on_required_missing 정책을 적용하고
	// projection / on_missing 으로 진행하지 않는다.
	if len(requiredPaths) > 0 {
		if missing, ok := firstMissingRequired(msg, requiredPaths); !ok {
			return n.handleRequiredMissing(msg, missing, onRequiredMissing)
		}
	}

	// --- 드랍 판정 (drop 정책) ---
	// 변형 전에 누락 여부를 먼저 확인하여, drop 이면 부분 변형 없이 즉시 처리한다.
	// drop_to_port 가 켜져 있으면 원본(미변형) 메시지를 "drop" 포트로 emit 한다.
	if onMissing == onMissingDrop {
		if payloadPathMissing(msg.Payload().ToMap(), payloadRoot) {
			return n.emitDrop(msg, dropToPort)
		}
		if metadataHasMissing(msg.Metadata().Raw(), mdTopLevel, mdPerGroup) {
			return n.emitDrop(msg, dropToPort)
		}
	}

	// --- payload 재구성 ---
	srcPayload := msg.Payload().ToMap()
	pruned := buildPrunedPayload(srcPayload, payloadRoot, onMissing)
	p := msg.Payload()
	for _, k := range p.Keys() {
		if _, keep := pruned[k]; !keep {
			p.Delete(k)
		}
	}
	for k, v := range pruned {
		p.Set(k, v)
	}

	// --- metadata 재구성 ---
	rebuildMetadata(msg.Metadata(), mdTopLevel, mdPerGroup, onMissing)

	// --- message-level (type) 재구성 ---
	// id 와 timestamp 는 항상 보존. type 은 $.type 미선택 시 비운다.
	if !typeKept {
		msg.SetType("")
	}

	return []message.Message{msg}, nil
}

// emitDrop 은 drop 결정 시의 출력을 결정한다.
//   - dropToPort 가 true 면 원본(미변형) 메시지의 Clone 에 _target_port="drop" 를
//     설정하여 "drop" 포트로 emit 한다 (filter.go 의 reject 포트 패턴 미러링).
//   - false 면 (nil, nil) 을 반환하여 메시지를 폐기한다 (기존 동작).
func (n *SelectFieldNode) emitDrop(msg message.Message, dropToPort bool) ([]message.Message, error) {
	if !dropToPort {
		return nil, nil
	}
	out := msg.Clone()
	out.Metadata().Set("_target_port", "drop")
	return []message.Message{out}, nil
}

// handleRequiredMissing 은 required 필드 누락 시 on_required_missing 정책을 적용한다.
//   - error_port (기본): 원본(미변형) 메시지의 Clone 에 _target_port="error" 와
//     에러 메타데이터(누락 필드명)를 설정하여 엔진 "error" 포트로 emit 한다
//     (filter 의 on_reject=error_port 패턴 미러링). Go 에러는 반환하지 않는다.
//   - drop: (nil, nil) 을 반환하여 메시지를 폐기한다 (on_missing=drop 폐기 경로 미러링).
//   - error: 누락 필드명을 담은 Go 에러를 반환한다 (엔진 에러 경로 → sendErrorToWires).
func (n *SelectFieldNode) handleRequiredMissing(
	msg message.Message, missingPath string, mode onRequiredMissingMode,
) ([]message.Message, error) {
	switch mode {
	case onRequiredDrop:
		return nil, nil
	case onRequiredError:
		return nil, fmt.Errorf("select-field: required field %q missing", missingPath)
	default: // onRequiredErrorPort
		out := msg.Clone()
		out.Metadata().Set(message.MetaKeyError,
			fmt.Sprintf("select-field: required field %q missing", missingPath))
		out.Metadata().Set(message.MetaKeyErrorNodeID, n.ID())
		out.Metadata().Set("_target_port", "error")
		return []message.Message{out}, nil
	}
}

// firstMissingRequired 는 정렬된 required 경로를 순서대로 평가하여 메시지에서
// 해석되지 않는 첫 경로를 반환한다. 모든 경로가 존재하면 ok=true 를 반환한다.
// (정렬된 경로 + 첫 누락 보고로 결정적 에러 메시지를 보장한다.)
//
// 경로 문법은 fields 와 동일하다:
//   - `$.payload.<dotpath>`        : 각 segment 를 따라 내려가 최종 키 존재 여부.
//   - `$.metadata.<key>`           : raw[key] 존재 여부 (string 또는 group).
//   - `$.metadata.<group>.<field>` : raw[group] 이 group(map[string]string) 이고
//     field 가 존재해야 함.
//   - `$.type`                     : msg.Type() 이 비어있지 않아야 함.
func firstMissingRequired(msg message.Message, requiredPaths []string) (missing string, ok bool) {
	for _, path := range requiredPaths {
		if !requiredPathPresent(msg, path) {
			return path, false
		}
	}
	return "", true
}

// requiredPathPresent 는 단일 required $.-경로가 메시지에 존재하는지 평가한다.
// normalizeRequiredPaths 가 의미 없는 경로를 미리 제거하므로, 여기서는 유효 경로만
// 들어온다고 가정한다.
func requiredPathPresent(msg message.Message, path string) bool {
	parts := strings.Split(path[2:], ".") // "$." 제거
	switch parts[0] {
	case "payload":
		return payloadPathPresent(msg.Payload().ToMap(), parts[1:])
	case "metadata":
		return metadataPathPresent(msg.Metadata().Raw(), parts[1:])
	case "type":
		return msg.Type() != ""
	default:
		// id / timestamp / 알 수 없는 root → 항상 존재로 간주 (체크 대상 아님).
		return true
	}
}

// payloadPathPresent 는 payload 맵에서 segment 경로가 해석되는지 확인한다.
// 중간 segment 는 map 이어야 더 깊이 내려갈 수 있으며, 최종 segment 는 키만 존재하면 된다.
func payloadPathPresent(src map[string]any, segs []string) bool {
	cur := src
	for i, seg := range segs {
		v, exists := cur[seg]
		if !exists {
			return false
		}
		if i == len(segs)-1 {
			return true // 최종 segment: 값 타입 무관, 존재만 하면 됨.
		}
		sub, ok := v.(map[string]any)
		if !ok {
			return false // 중간 경로가 map 이 아니면 더 내려갈 수 없음 → 누락.
		}
		cur = sub
	}
	return true
}

// metadataPathPresent 는 raw 메타데이터에서 metadata 경로가 해석되는지 확인한다.
//   - segs 길이 1 (`$.metadata.<key>`): raw[key] 존재 여부.
//   - segs 길이 2 (`$.metadata.<group>.<field>`): raw[group] 이 group 이고 field 존재.
//   - 길이 3+ : metadata 는 한 단계 깊이만 지원 → 누락으로 간주.
func metadataPathPresent(raw map[string]any, segs []string) bool {
	switch len(segs) {
	case 1:
		_, exists := raw[segs[0]]
		return exists
	case 2:
		g, ok := raw[segs[0]].(map[string]string)
		if !ok {
			return false
		}
		_, exists := g[segs[1]]
		return exists
	default:
		return false
	}
}

// parseSelectFields 는 통합 `fields` 화이트리스트를 도메인별 구조로 분해한다.
//
// 반환:
//   - payloadRoot: payload 화이트리스트 트리 루트 (children = 최상위 payload 키).
//   - mdRaw:       metadata 선택 (점 제거된 키 → fill). parseMetadataFields 입력용.
//     예) `$.metadata.device.id` → mdRaw["device.id"]="" .
//   - typeKept:    `$.type` 포함 여부.
//
// 모든 파싱은 관대하게(lenient) 처리한다:
//   - `$.` prefix 가 없는 경로는 무시한다.
//   - bare `$.payload` / `$.metadata` (sub-path 없음)는 무시한다.
//   - 알 수 없는 root 는 무시한다.
func parseSelectFields(fields map[string]string) (payloadRoot *payloadNode, mdRaw map[string]string, typeKept bool) {
	payloadRoot = &payloadNode{}
	mdRaw = make(map[string]string)

	for path, fill := range fields {
		if !strings.HasPrefix(path, "$.") {
			continue // 관대: $. prefix 없으면 무시.
		}
		parts := strings.Split(path[2:], ".")
		switch parts[0] {
		case "payload":
			segs := parts[1:]
			if len(segs) == 0 {
				continue // bare $.payload → 무시.
			}
			insertPayloadPath(payloadRoot, segs, fill)
		case "metadata":
			segs := parts[1:]
			if len(segs) == 0 {
				continue // bare $.metadata → 무시.
			}
			// 점으로 다시 합쳐 parseMetadataFields 입력(key 또는 group.field)으로 사용.
			// 한 단계 초과(group.field.x)는 parseMetadataFields 가 무시한다.
			mdRaw[strings.Join(segs, ".")] = fill
		case "type":
			typeKept = true
		case "id", "timestamp":
			// 항상 보존되므로 no-op (나열 허용).
		default:
			// 알 수 없는 root → 무시.
		}
	}
	return payloadRoot, mdRaw, typeKept
}

// insertPayloadPath 는 점-구분 segment 경로를 payload 트리에 삽입한다.
// 마지막 segment 노드를 whole=true 로 표시하고 fill 기본값을 저장한다.
// 같은 노드가 여러 경로에 의해 whole 로 표시되거나 children 을 가질 수 있으며,
// build/missing 단계에서 whole 이 우선한다.
func insertPayloadPath(root *payloadNode, segs []string, fill string) {
	node := root
	for i, seg := range segs {
		if node.children == nil {
			node.children = make(map[string]*payloadNode)
		}
		child := node.children[seg]
		if child == nil {
			child = &payloadNode{}
			node.children[seg] = child
		}
		if i == len(segs)-1 {
			child.whole = true
			child.fill = fill
		}
		node = child
	}
}

// buildPrunedPayload 는 src 맵을 payload 트리(node.children)에 따라 가지치기한 새 맵을 반환한다.
//
//   - whole 노드: src 에 키가 있으면 값 전체(서브트리) 유지. 없으면 fill 모드에서 기본값 채움.
//   - children 노드: src 의 값이 map 이면 재귀 가지치기, 아니면(부재/비맵) fill 모드에서 컨테이너 생성.
//   - keep/drop 모드에서는 누락 경로를 건너뛴다(drop 판정은 호출 전에 수행됨).
//
// node 가 children 없고 whole 도 아니면(루트 또는 빈 트리) 빈 맵을 반환한다 → payload 전체 비움.
func buildPrunedPayload(src map[string]any, node *payloadNode, mode onMissingMode) map[string]any {
	out := make(map[string]any)
	for key, child := range node.children {
		v, exists := src[key]
		if child.whole {
			if exists {
				out[key] = v
			} else if mode == onMissingFill {
				out[key] = child.fill
			}
			continue
		}
		// children 노드 → 재귀.
		sub, ok := v.(map[string]any)
		if !ok {
			// 중간 컨테이너 부재 또는 비-map.
			if mode == onMissingFill {
				out[key] = buildPrunedPayload(map[string]any{}, child, mode)
			}
			continue
		}
		prunedSub := buildPrunedPayload(sub, child, mode)
		if len(prunedSub) > 0 {
			out[key] = prunedSub
		}
		// keep/drop: 자식이 하나도 없으면 빈 중간 컨테이너는 만들지 않는다.
	}
	return out
}

// payloadPathMissing 은 payload 트리의 화이트리스트 경로 중 src 에서 해석되지 않는
// 경로가 하나라도 있으면 true 를 반환한다 (drop 정책 판정용).
//
//   - whole 노드: 키가 없으면 누락.
//   - children 노드: 키가 없거나 값이 map 이 아니면 누락(더 깊이 들어갈 수 없음).
//     map 이면 재귀.
func payloadPathMissing(src map[string]any, node *payloadNode) bool {
	for key, child := range node.children {
		v, exists := src[key]
		if child.whole {
			if !exists {
				return true
			}
			continue
		}
		sub, ok := v.(map[string]any)
		if !ok {
			return true
		}
		if payloadPathMissing(sub, child) {
			return true
		}
	}
	return false
}

// parseMetadataFields 는 metadata 선택(점 제거된 키 → fill)을 두 구조로 분해한다.
//   - topLevel:  점 없는 키 → fill 기본값. top-level string 키 또는 group 전체 키.
//   - perGroup:  group 키 → (field 이름 → fill 기본값). `group.field` 형식.
//
// `a.b.c` 처럼 점이 2개 이상인 엔트리는 무효(group 은 한 단계 깊이)이므로 무시한다.
func parseMetadataFields(fields map[string]string) (topLevel map[string]string, perGroup map[string]map[string]string) {
	topLevel = make(map[string]string)
	perGroup = make(map[string]map[string]string)
	for key, fillVal := range fields {
		group, field, hasDot := strings.Cut(key, ".")
		if !hasDot {
			// 점 없음 → top-level 키 (string 또는 group 전체).
			topLevel[key] = fillVal
			continue
		}
		if group == "" || field == "" || strings.Contains(field, ".") {
			// 빈 그룹/필드 또는 한 단계 초과(a.b.c) → 무효, 무시.
			continue
		}
		fset := perGroup[group]
		if fset == nil {
			fset = make(map[string]string)
			perGroup[group] = fset
		}
		fset[field] = fillVal
	}
	return topLevel, perGroup
}

// rebuildMetadata 는 화이트리스트(topLevel/perGroup)에 따라 메타데이터를 재구성한다.
//
// 현재 메타데이터를 Raw() 로 순회하며 각 top-level 키를 다음과 같이 처리한다:
//   - string 값: topLevel 에 선택되었으면 유지, 아니면 제거.
//   - group 값:
//   - topLevel 에 전체 선택(whole)되었으면 그룹 전체 보존.
//   - perGroup 에 필드 선택이 있으면 선택 필드만으로 그룹 재구성
//     (subset 이 비면 SetGroup 의 no-op delete 로 그룹 제거).
//   - 둘 다 아니면 그룹 제거.
//
// on_missing == fill 일 때 누락된 top-level string 키와 group 필드를 기본값으로 채운다.
// (group 자체가 없으면 group 을 생성한다.)
func rebuildMetadata(md message.Metadata, topLevel map[string]string, perGroup map[string]map[string]string, mode onMissingMode) {
	raw := md.Raw()

	for k, v := range raw {
		switch val := v.(type) {
		case map[string]string:
			// group 값.
			if _, whole := topLevel[k]; whole {
				// 전체 선택 → 그룹 보존(전체 키 우선; perGroup 무시).
				continue
			}
			fieldSel, ok := perGroup[k]
			if !ok {
				// 선택되지 않은 그룹 → 제거.
				md.Remove(k)
				continue
			}
			// 선택 필드만으로 그룹 재구성.
			subset := make(map[string]string, len(fieldSel))
			for field, fillVal := range fieldSel {
				if fv, exists := val[field]; exists {
					subset[field] = fv
				} else if mode == onMissingFill {
					subset[field] = fillVal
				}
				// keep/drop: 건너뜀.
			}
			// subset 이 비면 SetGroup 이 키를 제거한다(no-op delete).
			md.SetGroup(k, subset)
		default:
			// string 값.
			if _, sel := topLevel[k]; !sel {
				md.Remove(k)
			}
			// 선택된 string 은 그대로 둔다.
		}
	}

	if mode != onMissingFill {
		return
	}

	// fill: 누락된 top-level string 키 채움.
	// (부재 키는 string/group 구분이 불가능하므로 string fill 로 처리한다.)
	for k, fillVal := range topLevel {
		if _, exists := raw[k]; !exists {
			md.Set(k, fillVal)
		}
	}

	// fill: 그룹 자체가 부재인 perGroup 선택은 그룹을 생성하여 필드를 채운다.
	for group, fieldSel := range perGroup {
		if _, whole := topLevel[group]; whole {
			continue // 전체 키 우선.
		}
		if _, exists := raw[group]; exists {
			continue // 위 루프에서 처리됨.
		}
		subset := make(map[string]string, len(fieldSel))
		for field, fillVal := range fieldSel {
			subset[field] = fillVal
		}
		md.SetGroup(group, subset)
	}
}

// metadataHasMissing 은 화이트리스트(topLevel/perGroup) 중 raw 메타데이터에
// 존재하지 않는 항목이 하나라도 있으면 true 를 반환한다 (drop 정책 판정용).
//
//   - topLevel 키: raw 에 해당 키(string 또는 group)가 없으면 누락.
//   - perGroup `group.field`: group 이 없거나, group 에 해당 field 가 없으면 누락.
//     (단, 같은 group 이 topLevel 에 전체 선택되어 있으면 perGroup 판정은 건너뛴다.)
func metadataHasMissing(raw map[string]any, topLevel map[string]string, perGroup map[string]map[string]string) bool {
	for key := range topLevel {
		if _, ok := raw[key]; !ok {
			return true
		}
	}
	for group, fields := range perGroup {
		if _, whole := topLevel[group]; whole {
			continue // 전체 키 우선 — perGroup 판정 생략.
		}
		g, ok := raw[group].(map[string]string)
		if !ok {
			// 그룹 부재 또는 group 이 아님(string) → 누락.
			return true
		}
		for field := range fields {
			if _, ok := g[field]; !ok {
				return true
			}
		}
	}
	return false
}

// parseStringMap 은 fields 설정을 map[string]string 으로 관대하게 변환한다.
// map[string]string / map[string]any 모두 허용하며, 값은 문자열로 정규화한다
// (fill 기본값으로 사용). nil 이거나 다른 타입이면 빈 맵을 반환한다.
func parseStringMap(v any) map[string]string {
	result := make(map[string]string)
	switch m := v.(type) {
	case map[string]string:
		for k, val := range m {
			result[k] = val
		}
	case map[string]any:
		for k, val := range m {
			if s, ok := val.(string); ok {
				result[k] = s
			} else if val == nil {
				result[k] = ""
			} else {
				result[k] = fmt.Sprintf("%v", val)
			}
		}
	}
	return result
}
