---
id: SPEC-FLOW-NODECONFIG-001
version: "0.1.0"
status: implemented
created: 2026-09-21
updated: 2026-09-21
author: xtra
priority: P1
lifecycle_level: spec-first
title: "주입한 의존성이 API 로 나가려다 목록 전체가 죽었다"
phase: run
module: internal/engine
tier: S
tags: "flow, node-config, api, json, defect"
---

# SPEC-FLOW-NODECONFIG-001: 내부 주입 키는 밖으로 나가지 않는다

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|----------|
| 0.1.0 | 2026-09-21 | xtra | 최초 작성 + 구현 — 사용자 신고(`json: unsupported type: node.AgentLookupFunc`, 500) |

## 개요

```
ERROR API error code=INTERNAL_ERROR
  message="json: unsupported type: node.AgentLookupFunc"
  method=GET path=/api/v1/flows/<id>/nodes
```

노드 옵션들은 의존성을 **`_` 로 시작하는 config 키**에 넣는다 — `_enrich_agent_lookup`,
`_enrich_device_lookup`, `_agent_resolver`, 인벤토리 리졸버 등. 노드가 자기 의존성을 읽어
가는 통로다.

그런데 `buildNodeInstanceInfo` 가 그 map 을 **그대로** `NodeInstanceInfo.Config` 에 담아 API
로 내보냈고, 그 안에는 함수와 인터페이스가 들어 있다. `encoding/json` 은 함수를 직렬화하지
못하므로 **요청 하나가 통째로 500** 이 된다. 노드 하나가 아니라 목록 전체가 죽는다 —
직렬화는 전부 아니면 전무다.

그 옵션들은 `WithNodeOptions(...)` 로 **모든 노드**에 걸리므로 이 결함은 한 플로우의 문제가
아니다.

## 요구사항 (Requirements — EARS)

### REQ-01 — 노출 경계에서 내부 키를 건다 (Ubiquitous)

`NodeInstanceInfo.Config` 는 `_` 로 시작하는 키를 담지 않는다. 이 키들은 사용자가 편집기에서
만든 설정이 아니라 데몬이 배선한 의존성이며, 화면이 쓸 일도 없다.

### REQ-02 — 사용자 설정은 그대로 나간다 (Ubiquitous)

편집기에서 넣은 설정 키와 값은 변형 없이 응답에 실린다 — 화면이 그 값을 읽는다.

### REQ-03 — 설정이 없으면 nil 이다 (Unwanted)

설정이 없는 노드의 Config 는 빈 map 이 아니라 nil 이다(응답 모양 보존).

## 명세 (Specifications)

### 결정 1 — `GetConfig()` 는 건드리지 않는다

노드가 자기 의존성을 읽는 자리이므로 거기서 걸러 내면 주입이 통째로 무의미해진다. 필터는
API 로 나가는 단 한 지점(`buildNodeInstanceInfo`)에 둔다 — 목록과 상세가 그 함수를 공유한다.

### 결정 2 — 접두사 규칙을 그대로 쓴다

`_` 접두사는 이미 저장소 전반의 관례다(`_agent_resolver`, `_influxdb_agent` 등). 새 목록을
만들어 관리하면 다음 주입 지점이 빠뜨린다.

## 불변식 (Invariants)

- 노드 목록·상세 응답은 언제나 직렬화 가능하다.
- 내부 주입 키는 API 응답에 나타나지 않는다.

## 범위 밖 (Non-Goals)

1. **`Configure` 의 map 통째 교체** — 엔진이 `Configure(nd.Config)` 로 설정 map 을 교체하면
   주입된 `_` 키가 지워진다. enrich 노드는 생성 시점에 룩업을 구조체 필드로 붙잡아 두므로
   영향이 없음을 확인했고, 이번 범위에서는 손대지 않았다.
2. **함수·채널 값의 일반 방어** — 접두사 규칙으로 충분하다고 보았다.

## 검증

### 시험 (internal/engine/node_config_exposure_test.go)

| 시험 | 고정하는 것 |
|------|-------------|
| 주입된 노드의 정보가 직렬화된다 | REQ-01 |
| 내부 주입 키가 응답에 없다 | REQ-01 |
| 사용자 키는 남고 내부 키만 빠진다 | REQ-02 |
| 설정 없는 노드는 nil 을 유지한다 | REQ-03 |

### 재현

고치기 전 임시 시험에서 `config 키: [_enrich_agent_lookup]` / `json.Marshal 결과: json:
unsupported type: node.AgentLookupFunc` 를 관측했다 — 신고 로그와 같은 문구다.

### 가드가 무는지

필터를 걷어 내면 직렬화·키 제거 2종이, 접두사 판정을 무력화하면 3종이 실패한다.

### 전량

Go 47 패키지 통과, vet·gofmt 0.

## 잔여 위험

`_` 로 시작하는 **사용자** 설정 키가 있다면 함께 가려진다. 코드 관례상 그 접두사는 내부 주입
전용이지만, 편집기가 그런 키를 저장한 적이 없는지는 전수 확인하지 않았다.
