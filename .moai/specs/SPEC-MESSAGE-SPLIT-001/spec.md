---
id: SPEC-MESSAGE-SPLIT-001
title: "Split 노드 — 배열 페이로드를 N개 메시지로 팬아웃하는 파이프라인 노드"
version: "0.3.0"
status: in-progress
created: 2026-08-11
updated: 2026-08-11
author: xtra
priority: P2
phase: "v0.19.0"
module: "internal/node"
lifecycle: spec-anchored
tier: M
tags: "node, split, fan-out, message, payload, array, pipeline, tdd"
---

# SPEC-MESSAGE-SPLIT-001 — Split 노드 (배열 → N 메시지 팬아웃)

## HISTORY

| 일자 | 버전 | 변경 | 작성자 |
|------|------|------|--------|
| 2026-08-11 | 0.1.0 | 최초 작성. 배열을 담은 payload 를 요소별 N개 메시지로 팬아웃하는 신규 `split` 파이프라인 노드. `InventoryNode`(청크 팬아웃)·`FramerNode`(프레임 팬아웃) 패턴을 모델로 함. `payloads`/`messages`/`auto` 3 모드, `path`(키/JSONPath) 배열 위치 지정, 메타데이터 공유·병합(element wins), 엣지 케이스(passthrough/empty) 정의. EARS 6모듈 17 REQ. Tier M(3 파일). | xtra |
| 2026-08-11 | 0.2.0 | **`path` 시맨틱 정정(버그 수정)**: `$.`-접두 JSONPath 를 **메시지 루트(message-rooted)** 로 해석하도록 변경 — 프로젝트 전역 관례(`mapping.go:111`·`store_write.go:288`·`enrich.go:368` 는 모두 `$.` 를 payload 가 아닌 **메시지** 에 루팅)와 일치. 현행 `extractArray`(`split.go:290-307`)가 `msg.Payload().GetPath(n.path)`(payload-rooted)를 써서 사용자의 `$.payload.items[*]` 가 `payload.payload.items` 를 찾아 실패("path 누락 또는 비배열, passthrough" 경고)하던 버그를 수정. 정정 후 `$.payload.items[*]` = `msg.payload.items`, `$.metadata.x` = `msg.metadata.x`. 다른 노드와 동일한 리졸버(`resolveTemplateExpr(path, msg)` / `messageToMap`) 재사용. 평면(비-`$.`) 키는 하위호환 편의로 top-level **payload** 키 단축(`items` → `msg.payload.items`) 유지. REQ-04/REQ-06/REQ-18, config 예시, AC-9 를 `$.payload.items[*]` 로 갱신 + 메시지 루트 검증 AC 추가. | xtra |
| 2026-08-11 | 0.3.0 | **correlation-id 기능 제거(사용자 결정)**: REQ-17 의 `_correlationID = <parentID>#<index>` SHOULD 절을 폐지(VOID). 근거 — 엔진이 노드 홉마다 메시지를 복제(`internal/engine/engine.go:1301` `msg.Clone()` → 새 UUID)하므로 split 이 심은 correlationID 는 사용자가 볼 수 없는 **일시적 내부 clone id** 를 참조하여 추적에 무용했다(안정적/가시적 참조 대상 없음). REQ-17 의 **순서 보존(order preserved)** 절은 유지. 다른 REQ/AC 재번호 없음. 코드 반영 완료(`split.go` correlationID set 제거, `strconv` import 삭제, `AC-11` 테스트·`itoa` 헬퍼 삭제, `AC-12` correlation 단언 삭제; build/vet/test green). AC-11 폐지, AC-12 단언 축소, DoD 갱신. | xtra |

## 개요 (Overview)

`split` 노드는 **하나의 입력 메시지**를 받아, 그 payload 안의 **배열(array)** 을 꺼내 **요소마다 한 개씩 N개의 메시지**로 팬아웃(fan-out)하는 신규 파이프라인 처리 노드이다.
팬아웃은 이미 파이프라인 계약이다 — `Node.Process(ctx, msg) ([]message.Message, error)`(`internal/node/base.go:25`)는 0..N개의 결과 메시지를 반환하며,
`InventoryNode`(`internal/node/inventory.go:430-472`)가 스냅샷을 `max_items` 청크로 나눠 N개 메시지를 방출하고, `FramerNode`(`internal/node/framer.go:305-319`)가 바이트 스트림에서 분리된 프레임마다 메시지를 방출하는 것과 동일한 계약을 사용한다.

대표 입력 예시는 `output.json` — `InventoryNode` 출력으로, `payload.items` 가 9개 디바이스 객체의 배열이다. `split` 노드는 이런 `items` 배열을 받아 디바이스 하나당 메시지 1개로 펼친다.

핵심 설계 결정(요구사항으로 인코딩):

1. **신규 파이프라인 노드** — `internal/node/split.go`(+`split_test.go`). `Node` 인터페이스 구현, `[]message.Message` 반환. **빌트인 노드 레지스트리**에 다른 처리 노드와 동일한 방식으로 등록(`internal/node/registry.go` `registerBuiltins()` 빌트인 테이블). Split 노드는 외부 의존성(레지스트리/매니저)이 없으므로 `cmd/xflowd/main.go` 의 `NodeOption` 배선이 **불필요**하다(참고: agent 타입은 `main.go` 의 `RegisterXxxTypes` 로 등록되지만, `inventory`/`framer`/`filter` 등 빌트인 **처리** 노드는 `registerBuiltins()` 테이블 항목으로 등록된다).
2. **배열 위치 지정 (메시지 루트)** — 필수 `path` config 로 배열 위치를 지정. `$.`-접두 JSONPath 는 **메시지 전체**를 루트로 해석한다(payload 아님) — 프로젝트 전역 관례와 일치(`mapping.go:111`·`store_write.go:288`·`enrich.go:368`). 따라서 `$.payload.items[*]` = `msg.payload.items`, `$.metadata.x` = `msg.metadata.x`. 평면(비-`$.`) 키는 하위호환 편의로 top-level **payload** 키 단축(`items` → `msg.payload.items`)이다.
3. **요소 타입 — 양쪽 모두 지원** — `mode` config(`auto`/`payloads`/`messages`), 기본 `auto` 자동 감지.
4. **메타데이터 병합** — `messages` 모드에서 부모(공유) 메타데이터를 base 로, 요소 자신의 메타데이터가 키 충돌 시 override(요소 우선). `payloads` 모드에서는 부모 메타데이터를 공유(변경 없음).

## 환경 (Environment)

- 언어/런타임: Go 1.23+, 패키지 `internal/node` (package `node`).
- 계약: `Node` 인터페이스(`internal/node/base.go:15-32`), 팩토리 타입 `NodeFactory = func(def flow.NodeDef, opts ...NodeOption) (Node, error)`(`internal/node/registry.go:11`).
- 메시지 API: `pkg/message` — `message.New(...Option)`, `message.Clone()`(`pkg/message/message.go:122,221`), `Payload`(map 기반, `pkg/message/payload.go`), `Metadata`(값은 string 또는 map[string]string, `pkg/message/metadata.go`), JSONPath(`pkg/message/path.go`, `$.` 접두사 필수).
- 메시지 루트 리졸버(전역 관례): `resolveTemplateExpr(expr, msg)`(`internal/node/store_write.go:899`) 및 `messageToMap(msg)`(`internal/node/expression.go:92`) — `$.` JSONPath 를 **메시지 전체**(`{metadata, payload, type, timestamp, ...}`)에 루팅한다. `mapping.go:111` 은 `messageToMap(msg)` → `NewPayload` → `GetPath("$.payload.status")` 패턴으로 이를 사용한다. Split 노드도 이 리졸버를 재사용해야 한다(payload 직접 `GetPath` 금지).

## 가정 (Assumptions)

- `Payload` 루트는 항상 `map[string]any` 이다(`pkg/message/payload.go`).
- `Metadata` 값은 문자열(flat) 또는 `map[string]string`(group) 두 종류만 저장 가능하다(`pkg/message/metadata.go:29-38`) — 요소 메타데이터의 비문자열 값은 문자열화가 필요하다.
- 배열 요소는 JSON 역직렬화 결과이므로 `map[string]any`, 스칼라(string/float64/bool), 또는 다른 슬라이스일 수 있다.
- 메시지 레벨 `Timestamp()` 은 `time.Time`; payload 내부에 임베드된 디바이스 시각은 epoch milliseconds(int64, `UnixMilli`) 관례를 따른다 — split 은 요소 값을 재포맷하지 않고 그대로 보존한다.

## 요구사항 (Requirements — EARS)

### 모듈 1 — 노드 등록 및 계약 (Node Registration & Contract)

- **REQ-01** (Ubiquitous): The system **shall** register a `split` node type in the builtin node registry (`internal/node/registry.go` `registerBuiltins()`) with category `processing`, constructible via `NodeFactory`.
- **REQ-02** (Ubiquitous): The `split` node **shall** implement the `Node` interface (`internal/node/base.go:15-32`) by embedding `*BaseNode` and **shall** return `[]message.Message` from `Process(ctx, msg)`.
- **REQ-03** (Unwanted): If `Configure`/factory receives a `nil` or invalid config, **then** the node **shall** return a config error (following `ErrInvalidConfig` convention) and **shall not** panic.

### 모듈 2 — 배열 위치 지정 (Array Location)

- **REQ-04** (Ubiquitous): The system **shall** require a `path` config field (string) naming the array to split. A `$.`-prefixed JSONPath **shall** be resolved **MESSAGE-rooted** (against the whole message, `$` = the entire `{metadata, payload, type, timestamp, ...}`) using the same resolver the other nodes use (`resolveTemplateExpr(path, msg)` / `messageToMap(msg)` → `NewPayload().GetPath`, cf. `mapping.go:111`), so `$.payload.items[*]` = `msg.payload.items` and `$.metadata.x` = `msg.metadata.x`. A plain (non-`$.`) key **shall** stay a backward-compat shorthand for a top-level **payload** key (예: `"items"` = `msg.payload.items`, via `Payload().Get`). The node **shall NOT** resolve a `$.` path payload-rooted (the prior payload-rooted `msg.Payload().GetPath` behavior is a bug — it makes `$.payload.items` look for `payload.payload.items`).
- **REQ-05** (Unwanted): If `path` is missing/empty at factory time, **then** the factory **shall** return a config error.
- **REQ-06** (State-Driven): While the value resolved at `path` (message-rooted for `$.` paths, payload-shorthand for plain keys per REQ-04) is a slice (`toAnySlice`-compatible: `[]any`/`[]map[string]any`/기타 슬라이스), the node **shall** treat that slice as the array to fan out.

### 모듈 3 — 요소 타입 & 모드 (Element Type & Mode)

- **REQ-07** (Ubiquitous): The system **shall** support a `mode` config enum (`auto` | `payloads` | `messages`), default `auto`.
- **REQ-08** (Event-Driven): When `mode` is `payloads` (케이스 A), the node **shall** make each array element the new message's payload, sharing parent metadata to every split message.
- **REQ-09** (Event-Driven): When `mode` is `messages` (케이스 B), the node **shall** treat each array element as a full message object (`{metadata, payload, type?, timestamp?}`) and reconstruct a message from it.
- **REQ-10** (State-Driven): While `mode` is `auto` (default), the node **shall** auto-detect per element — if the element is a `map[string]any` carrying a `metadata` and/or `payload` key, treat it as a message (케이스 B); otherwise treat it as a payload (케이스 A).

### 모듈 4 — 메타데이터 병합 (Metadata Merge)

- **REQ-11** (State-Driven): While `mode` is `payloads` and `share_metadata` is `true` (default), each split message **shall** carry the parent metadata (flat string keys via `All()`/`Set`, and groups via `Raw()`/`SetGroup`) unchanged.
- **REQ-12** (Event-Driven): When `mode` is `messages`, the node **shall** merge metadata with parent (shared) as the BASE and the element's own metadata OVERRIDING on key conflict (element wins). Non-string element metadata values **shall** be stringified (`fmt.Sprintf("%v", v)`) before `Metadata.Set` (값 계약: string only).
- **REQ-13** (Optional): Where `share_metadata` is `false`, the node **shall** NOT copy parent metadata (split messages start with only element-derived metadata, empty in payloads mode).

### 모듈 5 — 메시지 구성 규칙 (Message Construction)

- **REQ-14** (Ubiquitous): Each split message **shall** receive a NEW id (UUID) — via `message.New(...)` (auto UUID) or `message.Clone()` (`pkg/message/message.go`).
- **REQ-15** (State-Driven): While constructing a split message, the node **shall** preserve `type` and `timestamp` from the parent, UNLESS the element (케이스 B) specifies its own `type`/`timestamp`, in which case the element value wins. Payload-embedded epoch-ms device times **shall** be preserved verbatim (no reformat).
- **REQ-16** (Unwanted): The payload root is always `map[string]any`. If a non-object (scalar/non-map) element is used as a payload (케이스 A), **then** the node **shall** wrap it under a configurable key `scalar_key` (default `"value"`) rather than error, preserving fan-out.
- **REQ-17** (Ubiquitous): Order of the input array **shall** be preserved in the output slice (요소 인덱스 순서 = 출력 메시지 순서).
  - ~~correlation-id (VOID, v0.3.0)~~: 초기 SHOULD 절(각 split 메시지에 `_correlationID = <parentID>#<index>` 설정)은 **폐지**되었다. 엔진이 노드 홉마다 `msg.Clone()`(`internal/engine/engine.go:1301`)으로 새 UUID 를 발급하므로 split-input id 는 일시적 내부 clone id 로, 사용자에게 안정적·가시적 참조 대상이 없어 추적에 무용했다(사용자 결정). `_correlationID` 를 설정하지 **않는다**.

### 모듈 6 — 엣지 케이스 (Edge Cases)

- **REQ-18** (Unwanted): If `path` is missing at runtime OR the resolved value is not an array, **then** the node **shall** apply `on_missing` (enum `passthrough` | `error`, default `passthrough`): `passthrough` emits the single input message unchanged with a WARN log; `error` returns an error.
- **REQ-19** (State-Driven): While the resolved array is empty, the node **shall** apply `on_empty` (enum `emit_none` | `passthrough`, default `emit_none`): `emit_none` returns 0 output messages; `passthrough` emits the input unchanged.

## 명세 (Specifications) — Config Schema

| 키 | 타입 | 필수 | 기본값 | 설명 |
|----|------|------|--------|------|
| `path` | string | 예 | — | 배열 위치. **메시지 루트**. `$.`-접두 JSONPath 는 메시지 전체 기준(`$.payload.items[*]` = `msg.payload.items`, `$.metadata.x` = `msg.metadata.x`). 평면(비-`$.`) 키는 top-level **payload** 키 단축(`"items"` = `msg.payload.items`). |
| `mode` | enum | 아니오 | `auto` | `auto` \| `payloads` \| `messages`. |
| `share_metadata` | bool | 아니오 | `true` | split 메시지에 부모 메타데이터 공유 여부. |
| `scalar_key` | string | 아니오 | `"value"` | payloads 모드에서 비객체 요소를 감쌀 payload 키. |
| `on_missing` | enum | 아니오 | `passthrough` | `passthrough` \| `error`. path 누락/비배열 시 동작. |
| `on_empty` | enum | 아니오 | `emit_none` | `emit_none` \| `passthrough`. 빈 배열 시 동작. |

### `path` 해석 (message-rooted resolver 재사용)

- `extractArray`(`internal/node/split.go:290-307`)는 `$.`-접두 경로에 대해 payload 직접 `GetPath` 대신 **메시지 루트 리졸버**를 사용해야 한다: `messageToMap(msg)` → `message.NewPayload(msgMap)` → `GetPath(path)`(또는 동등하게 `resolveTemplateExpr(path, msg)`), `internal/node/mapping.go:111` 과 동일 패턴. 이로써 `$.payload.items[*]` 는 `msg.payload.items` 로, `$.metadata.x` 는 `msg.metadata.x` 로 해석된다.
- 평면(비-`$.`) 키는 하위호환을 위해 top-level payload 키 단축을 유지한다(`Payload().Get(key)`).
- 본 변경은 초기 payload-rooted 구현(`msg.Payload().GetPath` → `$.payload.items` 가 `payload.payload.items` 를 찾던 버그)을 **정정**하여 코드베이스 전역 관례(모든 노드가 `$.` 를 메시지에 루팅)와 정렬한다.

## 추적성 (Traceability)

- 구현: `internal/node/split.go` (신규), 테스트 `internal/node/split_test.go` (신규).
- 등록: `internal/node/registry.go` `registerBuiltins()` 빌트인 테이블 항목 `{"split", NewSplitNode, "processing", "..."}`.
- 모델 참조: `internal/node/inventory.go:430-472`(청크 팬아웃), `internal/node/framer.go:305-319`(프레임 팬아웃), `internal/node/base.go:15-32`(Node 계약).
- 메시지 API: `pkg/message/message.go`, `pkg/message/payload.go`, `pkg/message/metadata.go`, `pkg/message/path.go`.
- 메시지 루트 리졸버(재사용, REQ-04): `resolveTemplateExpr`(`internal/node/store_write.go:899`), `messageToMap`(`internal/node/expression.go:92`), 소비 패턴 `internal/node/mapping.go:111`. 정정 대상 구현: `internal/node/split.go:290-307` `extractArray`(현행 `msg.Payload().GetPath` → 메시지 루트 리졸버로 교체). 전역 관례 근거: `store_write.go:288`(`$.payload.state.mode`), `enrich.go:368`(`$.metadata.agent.id`).
- 예시 데이터: `output.json`(`payload.items` 9-요소 배열).
- 상세 계획: `plan.md`. 인수 기준: `acceptance.md`.

## 비목표 (Non-Goals)

- 배열이 아닌 재귀적/중첩 구조의 다단계 flatten (단일 배열 1레벨 팬아웃만).
- 요소별 조건 필터링(그것은 `filter`/`inventory condition` 의 몫).
- 팬아웃 결과의 재집계(그것은 `aggregate` 노드의 몫).
