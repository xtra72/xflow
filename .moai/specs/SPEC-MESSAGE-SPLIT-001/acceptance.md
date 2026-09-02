# SPEC-MESSAGE-SPLIT-001 — 인수 기준 (acceptance.md)

Given-When-Then 형식. 모든 시나리오는 `internal/node/split_test.go` 의 테이블 기반 테스트로 검증한다.

## AC-1 — 케이스 A (payloads 모드, 요소=payload)

- **Given** payload `{"items": [ {"a":1}, {"b":2}, {"c":3} ]}` 를 담은 입력 메시지(type=`T`, timestamp=`ts`), config `path="items"`, `mode="payloads"`
- **When** `Process` 호출
- **Then** 정확히 **3개** 메시지 반환. 각 메시지의 payload 는 순서대로 `{"a":1}`, `{"b":2}`, `{"c":3}`. 모든 메시지가 부모 메타데이터를 공유하고, `Type()==T`, `Timestamp()==ts`, 각 `ID()` 는 서로 다른 UUID(부모와도 다름). 입력 배열 순서 보존.

## AC-2 — 케이스 B (messages 모드, 요소=완전한 메시지)

- **Given** payload `{"messages": [ {"metadata":{"x":"1"},"payload":{"p":10}}, {"metadata":{"y":"2"},"payload":{"p":20}} ]}`, 부모 메타 `{"x":"parent","z":"keep"}`, config `path="messages"`, `mode="messages"`, `share_metadata=true`
- **When** `Process` 호출
- **Then** **2개** 메시지 반환. 각 메시지 metadata = `merge(parent, element)` 이고 **충돌 키는 요소 우선** — 1번 메시지 metadata 는 `x=1`(요소가 부모 `x=parent` override), `z=keep`(부모 유지), `y` 없음; 2번은 `y=2`, `x=parent`, `z=keep`. 각 payload = 요소의 `payload`(`{"p":10}`, `{"p":20}`).

## AC-3 — auto 모드 자동 감지 (혼합)

- **Given** `mode="auto"`, 배열이 요소별로 타입 상이: `[ {"metadata":{"m":"1"},"payload":{"v":1}}, {"plain":true}, 42 ]`
- **When** `Process` 호출
- **Then** **3개** 메시지. (1) `metadata`/`payload` 키 보유 → 케이스 B(payload=`{"v":1}`, metadata 병합). (2) `{"plain":true}` 는 message 키 미보유 → 케이스 A(payload=`{"plain":true}`). (3) 스칼라 `42` → 케이스 A + 비객체 래핑(payload=`{"value":42}`, 기본 `scalar_key`).

## AC-4 — 엣지: path 누락 → passthrough

- **Given** config `path="nonexistent"`(또는 `on_missing` 미지정 기본), 입력 payload 에 해당 키 없음
- **When** `Process` 호출
- **Then** 입력 메시지 **그대로 1개** 반환(`[]message.Message{input}`), 크래시 없음, WARN 로그(로거 존재 시).

## AC-5 — 엣지: 값이 배열 아님 → passthrough

- **Given** payload `{"items": "not-an-array"}`, config `path="items"`, 기본 `on_missing=passthrough`
- **When** `Process` 호출
- **Then** 입력 메시지 그대로 1개 반환, 크래시 없음.

## AC-6 — 엣지: 빈 배열 → 0 메시지

- **Given** payload `{"items": []}`, config `path="items"`, 기본 `on_empty=emit_none`
- **When** `Process` 호출
- **Then** **0개** 메시지 반환(`len(results)==0`, err==nil).

## AC-7 — 비객체 요소 (payloads 모드 기본 동작)

- **Given** payload `{"nums":[1,2,3]}`, `mode="payloads"`, `scalar_key` 미지정(기본 `"value"`)
- **When** `Process` 호출
- **Then** 3개 메시지, 각 payload = `{"value":1}`, `{"value":2}`, `{"value":3}`.

## AC-8 — 등록/팩토리 구성 가능

- **Given** `NewRegistry()` 로 생성한 노드 레지스트리
- **When** `registry.Types()`/`registry.Create(def)` 로 `Type:"split"` 노드 생성
- **Then** `"split"` 이 등록 목록에 존재(category `processing`), `Create` 가 `*SplitNode`(Node 구현)를 반환, `TypeMeta("split")` 존재. (근거: `internal/node/registry.go` `registerBuiltins()` 테이블, `registry_test.go` 등록 검증 패턴과 동일.)

## AC-9 — JSONPath 경로 지정 (message-rooted)

- **Given** payload `{"items":[{"a":1},{"b":2}]}`, config `path="$.payload.items[*]"`(또는 `$.payload.items`)
- **When** `Process` 호출
- **Then** 평면 키 `"items"` 와 동일하게 2개 메시지로 팬아웃한다. `$.` 경로는 **메시지 루트**로 해석되어 `$.payload.items` = `msg.payload.items`(메시지 루트 리졸버 `messageToMap`/`resolveTemplateExpr` 사용, `mapping.go:111` 과 동일 패턴).

## AC-13 — path 는 메시지 루트 (버그 회귀 방지)

- **Given** payload `{"items":[{"a":1},{"b":2}]}` 를 담은 메시지
- **When** config `path="$.payload.items[*]"` 로 `Process` 호출
- **Then** payload 의 `items` 배열이 해석되어 **2개** 메시지로 팬아웃한다(`$.payload.items` = `msg.payload.items`).
- **And (음성 검증)** config `path="$.items[*]"`(payload-상대 경로)는 메시지 루트에서 `msg.items` 를 찾으므로 **배열을 해석하지 못하고**, `on_missing=passthrough`(기본)에 따라 입력 1개를 그대로 반환한다(payload-rooted 아님을 증명).
- **And** `$.metadata.<key>` 형태의 경로는 `msg.metadata.<key>` 로 해석된다(메시지 루트 일관성).

## AC-10 — 타임스탬프/타입 보존 및 요소 override

- **Given** `mode="messages"`, 부모 type=`T`, timestamp=`ts`. 요소 1: type/timestamp 미지정; 요소 2: `type="U"` 지정
- **When** `Process` 호출
- **Then** 메시지 1 은 `Type()==T`, `Timestamp()==ts`(부모 보존); 메시지 2 는 `Type()=="U"`(요소 override). payload 내 epoch-ms 값은 재포맷 없이 그대로.

## AC-11 — (폐지 / VOID, v0.3.0) correlation id

correlation-id 기능은 v0.3.0 에서 **제거**되었다(엔진이 노드 홉마다 `msg.Clone()` 으로 새 UUID 발급 → split-input id 는 일시적 내부 clone id → 안정적·가시적 참조 없음, 사용자 결정). 본 AC 는 폐지되며 검증 대상이 아니다. 다른 AC 는 재번호하지 않는다(AC-12/AC-13 번호 유지). 순서 보존(REQ-17)은 AC-1 에서 검증된다.

## AC-12 — share_metadata=false

- **Given** `mode="payloads"`, `share_metadata=false`, 부모 메타 `{"x":"1"}`
- **When** `Process` 호출
- **Then** split 메시지들은 부모 메타를 복사하지 않음(요소 파생 메타만, payloads 모드에서는 비어 있음).

## 완료 정의 (Definition of Done)

- [ ] AC-1 ~ AC-10 전부 통과(AC-11 은 폐지/VOID, AC-12·AC-13 포함).
- [ ] `go test ./internal/node/...` GREEN.
- [ ] `internal/node/split.go` 커버리지 ≥ 85%.
- [ ] `golangci-lint run` 클린(신규 파일 0 findings).
- [ ] `registerBuiltins()` 에 `split` 등록, `output.json`(`payload.items`) 형태 실데이터 통합 테스트 통과.
- [ ] TRUST 5 게이트 통과, `code_comments: ko` 관례 준수, `@MX` 태그(신규 export 함수 NOTE/ANCHOR) 적절 부착.

## 품질 게이트 & 검증 방법

- 도구: `go test -race -coverprofile`, `golangci-lint run`, 테이블 기반 유닛 테스트.
- 검증: 각 AC 를 최소 1개 테스트 케이스로 매핑. 팬아웃 개수/순서/payload/metadata/id 유일성을 명시 assert.
