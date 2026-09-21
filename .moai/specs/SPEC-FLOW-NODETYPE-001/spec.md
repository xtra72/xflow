---
id: SPEC-FLOW-NODETYPE-001
version: "0.1.0"
status: implemented
created: 2026-09-21
updated: 2026-09-21
author: xtra
priority: P2
lifecycle_level: spec-first
title: "없는 타입의 이름을 말한다 — 그리고 한꺼번에 센다"
phase: run
module: internal/engine
tier: S
tags: "flow, node-type, diagnostics, deploy, defect"
---

# SPEC-FLOW-NODETYPE-001: 없는 타입의 이름을 말한다

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|----------|
| 0.1.0 | 2026-09-21 | xtra | 최초 작성 + 구현 — 사용자 신고(`failed to create node "Temperature": node: type not found`) |

## 개요 — 노드 이름은 있는데 타입이 없다

운영 로그는 이렇게 남았다.

```
Start handler: StartFlow failed
  error="flow start: deploy failed: engine: failed to create node \"Temperature\": node: type not found"
```

`Temperature` 는 노드 **이름**이고, 정작 **어느 타입이 없는지**는 메시지 어디에도 없다.
운영자는 저장된 플로우 JSON 을 열어 그 노드를 찾아야 했다. 등록된 빌트인 61종은 모두 조건
없이 등록되므로(모드·설정에 따라 빠지는 타입은 없다), 없는 타입이란 이 빌드에 존재하지
않는 이름이라는 뜻이다 — 다른/옛 버전에서 만든 플로우, 이름이 바뀐 타입, 손으로 넣은 JSON.

거기에 배포는 **첫 번째** 미등록 타입에서 멈췄다. 그런 노드가 셋이면 하나 고쳐 다시
시작하고 또 멈추기를 세 번 되풀이해야 했다.

## 요구사항 (Requirements — EARS)

### REQ-01 — 에러가 타입 이름을 싣는다 (Event-Driven)

**When** 레지스트리가 등록되지 않은 타입의 노드 생성을 요청받으면, 반환하는 에러는
`ErrNodeTypeNotFound` 를 감싸면서 **그 타입 이름**을 포함한다. `errors.Is` 판정과 API 404
매핑은 그대로 유지된다.

### REQ-02 — 배포 전에 한꺼번에 센다 (Event-Driven)

**When** 플로우를 배포하면, 엔진은 노드를 만들기 **전에** 등록되지 않은 타입을 모두 모아
하나의 에러로 보고한다. 메시지에는 없는 타입과 그 타입을 쓰는 노드 이름이 함께 들어간다.

### REQ-03 — 같은 타입은 한 번만 적는다 (Ubiquitous)

여러 노드가 같은 타입을 쓰면 타입은 한 번 적고 노드 이름을 모아 적는다. 노드 순서를 그대로
따르므로 로그가 실행마다 흔들리지 않는다.

### REQ-04 — 등록된 타입 목록은 싣지 않는다 (Unwanted)

61종이 넘어 로그를 덮는다. 목록은 `GET /api/v1/nodes` 로 조회한다.

## 명세 (Specifications)

### 결정 1 — 사전 검사는 노드 생성 앞에 둔다

만들다 멈추면 이미 만든 노드를 닫는 일도 매번 일어난다. 만들기 전에 전부 세면 그 낭비도
사라진다.

### 결정 2 — 레지스트리도 함께 고친다

엔진의 사전 검사가 대부분을 먼저 잡지만, 레지스트리를 직접 쓰는 다른 호출자(노드 어댑터)도
같은 질문에 답할 수 있어야 한다.

## 불변식 (Invariants)

- 미등록 타입으로 실패한 배포의 에러 메시지는 **무엇이 없는지**와 **어디를 고쳐야 하는지**를
  둘 다 담는다.
- `errors.Is(err, node.ErrNodeTypeNotFound)` 는 그대로 참이다.

## 범위 밖 (Non-Goals)

1. **저장 시점 검증** — `pkg/flow.Validate` 는 레지스트리를 모르는 순수 패키지라 노드 타입을
   검사하지 않는다. 지금도 모르는 타입을 가진 플로우를 저장·가져오기 할 수 있고, 실패는
   시작할 때에야 드러난다.
2. **이름이 바뀐 타입의 마이그레이션**.

## 검증

### 시험

| 파일 | 고정하는 것 |
|------|-------------|
| `internal/engine/nodetype_validation_test.go` (4) | 에러가 타입·노드 이름을 담는다, 여러 타입을 한꺼번에 보고, 같은 타입은 한 번, 등록된 타입은 끼지 않음 |
| `internal/node/registry_typename_test.go` (1) | 레지스트리 단독 호출에서도 타입 이름이 실린다 |

### 가드가 무는지

사전 검사를 빼면 "한꺼번에 보고" 2종이, 에러에서 타입 이름을 빼면 레지스트리 시험이 실패한다.

### 전량

Go 47 패키지 통과, vet·gofmt 0.

## 잔여 위험

신고한 환경에서 **무엇이 없는 타입인지**는 여전히 모른다. 이 변경은 다음 발생부터 로그가
스스로 답하게 만들 뿐이며, 지금 있는 플로우는 저장된 JSON 을 조회해 확인해야 한다.
