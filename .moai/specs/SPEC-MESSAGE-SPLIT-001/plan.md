# SPEC-MESSAGE-SPLIT-001 — 구현 계획 (plan.md)

## 기술 접근 (Technical Approach)

`split` 은 외부 의존성이 없는 순수 payload 변환 팬아웃 노드이다. `InventoryNode` 의 팬아웃 뼈대(수집 → 변환 → N 메시지)를 재사용하되, "수집"이 레지스트리 스냅샷 대신 **입력 메시지 payload 의 배열 추출**로 바뀐다.

### 인용 코드 근거 (검증 완료)

- **Node 계약** — `internal/node/base.go:15-32`: `Process(ctx, msg) ([]message.Message, error)`(line 25). 노드는 `*BaseNode` 임베드, `NewBaseNode(def flow.NodeDef, opts ...NodeOption)`(base.go:103). `Configure` 는 config 를 `b.config` 로 복사(base.go:172-183).
- **팬아웃 템플릿** — `internal/node/inventory.go:430-472`: `Process` 가 `results := make([]message.Message, 0, ...)` 로 청크별 메시지를 append 하여 반환. 팩토리 `NewInventoryNode(def, opts...) (Node, error)`(inventory.go:209) 는 `cfg := def.Config` 파싱 후 노드 구조체 반환, 컴파일 타임 체크 `var _ Node = (*InventoryNode)(nil)`(inventory.go:188). 프레임 팬아웃은 `internal/node/framer.go:305-319`.
- **등록 메커니즘** — `internal/node/registry.go:73-147` `registerBuiltins()`: `builtins` 테이블(`{typeName, factory, category, description}`)을 순회하여 `r.factories[typeName]`/`r.metadata[typeName]` 등록. `NodeFactory = func(def flow.NodeDef, opts ...NodeOption) (Node, error)`(registry.go:11). `inventory`(registry.go:131), `framer`(registry.go:128), `filter`(registry.go:81) 모두 이 테이블 항목이다. **Split 도 이 테이블에 항목 1줄 추가만 하면 됨** — `cmd/xflowd/main.go` 배선 불필요(agent 타입만 `main.go` `RegisterXxxTypes` 로 등록됨).
- **메시지 구성** — `pkg/message/message.go`: `message.New(opts...)`(line 122), 옵션 `WithPayload`/`WithMetadata`/`WithType`/`WithTimestamp`/`WithID`(line 54-106). `Clone()`(line 221-246)은 새 UUID + type/timestamp 보존 + payload/metadata 깊은 복사.
- **Payload** — `pkg/message/payload.go`: `NewPayload(data ...map[string]any)`(line 37), `Set/Get`(line 57,65), `GetPath(jsonpath)`(line 70). 루트는 `map[string]any`.
- **Metadata** — `pkg/message/metadata.go`: 값 계약 = string(flat) 또는 map[string]string(group)(line 29-38). `Set(k,v string)`(line 94), `All() map[string]string`(line 107), `Raw() map[string]any`(line 147), `SetGroup`(line 118). (correlation-id 기능 폐지 — `MetaKeyCorrelationID` 미사용.)
- **JSONPath (메시지 루트)** — `pkg/message/path.go`: `evaluatePath`(line 11), `$.` 접두사 필수(line 13), `$.items[*]` 와일드카드는 배열 반환(line 116-135), `$.items[0]` 인덱스 지원. **`$.` 경로는 메시지 전체를 루트로 해석해야 함** — 전역 관례 리졸버 `messageToMap(msg)`(`internal/node/expression.go:92`) → `message.NewPayload(msgMap)` → `GetPath(path)`, 또는 `resolveTemplateExpr(path, msg)`(`internal/node/store_write.go:899`); 소비 예 `internal/node/mapping.go:111`. 따라서 `$.payload.items[*]` = `msg.payload.items`. 평면 키 `"items"` 는 하위호환 편의로 `Payload().Get("items")`(= top-level payload 키) 로 해석한다.
- **예시** — `output.json`: `payload.items` 는 9개 디바이스 객체(`map[string]any`) 배열, `payload.count=9`, 메시지 `type="inventory.event"`, `timestamp=1779794368319`(epoch ms).

## 파일 구조

```
internal/node/split.go        # SplitNode 구조체 + NewSplitNode 팩토리 + Process + config 파서
internal/node/split_test.go   # 테이블 기반 TDD 테스트
internal/node/registry.go     # registerBuiltins() 빌트인 테이블에 {"split", NewSplitNode, "processing", "..."} 1줄 추가
```

## 구현 스케치 (설계 지침 — 구현 시 확정)

### 구조체 & 팩토리

```
type SplitNode struct {
    *BaseNode
    path          string   // "items"(payload 키 단축) 또는 "$.payload.items[*]"(메시지 루트)
    isJSONPath    bool      // path 가 "$." 로 시작하는지 → 메시지 루트 리졸버 사용
    mode          string    // "auto" | "payloads" | "messages"
    shareMetadata bool
    scalarKey     string
    onMissing     string    // "passthrough" | "error"
    onEmpty       string    // "emit_none" | "passthrough"
}
var _ Node = (*SplitNode)(nil)

func NewSplitNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
    base := NewBaseNode(def, opts...)
    cfg := def.Config
    // path(필수), mode/share_metadata/scalar_key/on_missing/on_empty(기본값) 파싱
    // 잘못된 enum/타입 → sentinel error(ErrInvalidConfig 계열)
}
```

### Process 로직 (수집 → 분기 → 팬아웃)

1. **배열 추출 (메시지 루트)**: `path` 가 `$.` 접두면 **메시지 루트 리졸버**로 해석 — `msgMap := messageToMap(msg)`; `message.NewPayload(msgMap).GetPath(path)`(또는 `resolveTemplateExpr(path, msg)`), `mapping.go:111` 과 동일 패턴(payload 직접 `GetPath` 금지). 비-`$.` 평면 키는 `msg.Payload().Get(path)`(top-level payload 키 단축). 결과를 `toAnySlice` 로 슬라이스 변환. → `$.payload.items[*]` = `msg.payload.items`, `$.metadata.x` = `msg.metadata.x`.
2. **엣지 — 누락/비배열** (REQ-18): 추출 실패 또는 비슬라이스 → `on_missing`. `passthrough`: `return []message.Message{msg}, nil` + WARN 로그. `error`: `return nil, err`.
3. **엣지 — 빈 배열** (REQ-19): `len(arr)==0` → `on_empty`. `emit_none`: `return nil, nil`(0 메시지). `passthrough`: `return []message.Message{msg}, nil`.
4. **요소별 팬아웃**: `results := make([]message.Message, 0, len(arr))`; 각 요소 `elem`(인덱스 `i`)에 대해:
   - **모드 판정** (REQ-08/09/10): `auto` 면 `elem` 이 `map[string]any` 이고 `metadata`/`payload` 키를 가지면 messages, 아니면 payloads. 명시 모드는 그대로.
   - **케이스 A (payloads)**: payload 맵 = `elem` 이 `map[string]any` 면 그 자체, 스칼라면 `map[string]any{scalarKey: elem}`(REQ-16). `out := message.New(WithType(parent.Type()), WithTimestamp(parent.Timestamp()), WithPayload(message.NewPayload(payloadMap)))`. `shareMetadata` 면 부모 메타 복사(아래).
   - **케이스 B (messages)**: `elem.(map[string]any)`; `payload = elem["payload"]`(map), `type = elem["type"]` 있으면 그 값 else parent.Type(), `timestamp = elem` 값 있으면 파싱 else parent.Timestamp(). `out := message.New(WithType(...), WithTimestamp(...), WithPayload(...))`. 메타 병합(아래).
   - ~~correlation id (REQ-17, SHOULD)~~ **폐지(v0.3.0)**: correlationID 설정 안 함(엔진이 노드 홉마다 `msg.Clone()` 으로 새 UUID → split-input id 무용). `strconv` import·`itoa` 헬퍼 불필요.
   - `results = append(results, out)`(입력 순서 = 출력 순서, REQ-17 order 절 유지).
5. `return results, nil`.

### 메타데이터 공유/병합 헬퍼

- **공유(케이스 A, share_metadata=true)** (REQ-11): 부모 `Raw()` 순회 — string 값은 `out.Metadata().Set(k, v)`, `map[string]string`(group) 값은 `out.Metadata().SetGroup(k, v)`. flat+group 모두 보존.
- **병합(케이스 B)** (REQ-12): 먼저 부모 메타(share 시)를 base 로 적용, 그 다음 요소 `elem["metadata"]`(map[string]any) 를 순회하여 override — 값이 string 이 아니면 `fmt.Sprintf("%v", v)` 로 문자열화 후 `Set`(값 계약: string only). 요소 우선.

### 등록 (registry.go)

`registerBuiltins()` 의 `builtins` 슬라이스에 한 줄 추가(inventory 항목 근처):

```
{"split", NewSplitNode, "processing", "입력 메시지 payload 의 배열을 요소별 N개 메시지로 팬아웃"},
```

## 마일스톤 (우선순위 기반, 시간 예측 없음)

- **Primary Goal (M1) — Priority High**: 구조체/팩토리/config 파서 + payloads 모드 팬아웃 + registerBuiltins 등록. RED→GREEN(REQ-01,02,04,05,06,07,08,11,14,15,16).
- **Secondary Goal (M2) — Priority High**: messages 모드 + 메타데이터 병합(element wins) + auto 감지(REQ-09,10,12,13).
- **Tertiary Goal (M3) — Priority Medium**: 엣지 케이스(on_missing/on_empty passthrough·error·emit_none) + 순서 보존(REQ-17 order 절, REQ-18, REQ-19, REQ-03). (correlation id 폐지 — v0.3.0.)
- **Final Goal (M4) — Priority Medium**: `output.json` 형태 실데이터 기반 통합 테스트 + 커버리지 85%+ + golangci-lint 클린.

## 개발 방법론

`quality.yaml` `development_mode: hybrid` → 신규 코드는 **TDD**(RED-GREEN-REFACTOR). 테이블 기반 테스트 우선 작성, 요소 타입/모드/엣지 케이스를 케이스로 커버.

## 리스크 & 대응

| 리스크 | 대응 |
|--------|------|
| Metadata 값 계약(string only)과 요소의 비문자열 메타 값 충돌 | REQ-12: `fmt.Sprintf("%v", v)` 문자열화를 명시적 규칙으로 인코딩. |
| 평면 키 vs JSONPath 혼동(`"items"` 는 JSONPath 아님) | `$.` 접두 판정으로 (평면)`Get` vs (메시지 루트)리졸버 분기(구현 스케치 1단계). |
| **payload-rooted 오해로 인한 회귀** — `$.payload.items` 를 payload 에 직접 `GetPath` 하면 `payload.payload.items` 를 찾아 실패(초기 버그) | 전역 관례대로 **메시지 루트 리졸버**(`messageToMap`→`NewPayload`→`GetPath` / `resolveTemplateExpr`) 사용. AC-13 음성 검증(`$.items[*]` 는 배열 미해석)으로 회귀 고정. |
| `$.payload.items[*]` 와일드카드가 요소 배열이 아닌 투영 결과 반환 가능성 | path.go:124 — 남은 토큰 없으면 배열 그대로 반환. `$.payload.items` 와 `$.payload.items[*]` 모두 배열 반환 확인 필요. 테스트로 고정. |
| auto 모드 오분류(payload 키를 가진 정상 데이터 객체를 message 로 오인) | `metadata`/`payload` 키 **동시/존재** 휴리스틱은 best-effort; 모호 시 명시 모드 권장을 문서화(REQ-10). |
