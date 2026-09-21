---
id: SPEC-REMOTE-ONLINE-001
version: "0.2.0"
status: implemented
created: 2026-09-17
updated: 2026-09-17
author: xtra
priority: P1
lifecycle_level: spec-first
title: "online 은 keep-alive 가 답한다 — 항목의 유무가 아니다"
phase: run
module: internal/remote
tier: S
tags: "remote, online, heartbeat, keep-alive, defect"
---

# SPEC-REMOTE-ONLINE-001: online 은 keep-alive 가 답한다

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|----------|
| 0.1.0 | 2026-09-17 | xtra | 최초 작성 — 사용자 신고("xagent04 는 연결도 안되어 있는데 online 으로 표시됨") |
| 0.2.0 | 2026-09-17 | xtra | 구현 완료. 조사 중 **조건부 판정의 둘째 사본**(`NodeDetail`)을 발견 — K2 를 고쳐 적음 |

## 개요 — 주석이 세운 불변식이 그 자리에서 깨진다

```go
// in-memory online 상태를 반영한다(repo 의 online 은 영속 시점 기준이므로 라이브
// 연결 상태를 우선 적용 — 라이브 추적이 권위).
if st, ok := s.nodes[nodes[i].InstanceID]; ok {
    nodes[i].Online = st.Online
}
```

주석은 **"라이브 추적이 권위"** 라고 적었다. 그런데 오버레이가 **조건부**다 — 메모리에 항목이
없으면 영속값이 그대로 나간다. 즉 **가장 중요한 경우**(이번 부팅 이후 한 번도 붙지 않은
노드)에서 권위가 조용히 DB 로 넘어간다.

`s.nodes` 항목은 `setNodeState` 가 register/hello 에서만 만들고, 지우는 곳은 노드 삭제
둘뿐이다. 그러므로 **재시작 이후 한 번도 붙지 않은 노드는 항목이 없다** — 그리고 부팅
시점에 영속값을 맞추는 자리가 **없다**(`SetOnline` 을 부르는 곳은 노드 단위
`persistOnline` 하나다).

## 결함 둘

### 결함 ① `sweepOnce` 가 영속하지 않는다

끊김을 감지하는 `markOffline` 은 영속한다(`persistOnline(id, false, now)`). 하트비트
타임아웃을 감지하는 `sweepOnce` 에는 **그 한 줄이 없다.** 메모리만 뒤집고 DB 에 `online=1`
을 남긴다. 프로세스가 사는 동안은 오버레이가 그 거짓을 가려 주지만 **다음 재시작에 드러난다.**

### 결함 ② 항목이 없으면 영속값이 그대로 새어 나간다

위 §개요. xagent04 가 겪은 것이 이 둘의 겹침이다:

1. 붙어 있었다 → DB `online=1`
2. 끊겼다 — 타임아웃이었으면 DB 는 `1` 로 남는다(①). 서버가 먼저 죽었어도 `1` 이다
3. 재시작 → `s.nodes` 가 빔
4. 오버레이가 건너뛰어짐 → `1` 이 그대로 나감(②)
5. 청소기는 그 노드를 **보지도 않는다**(메모리만 돈다)

**스스로 풀리지 않는다.** 풀리는 길은 그 노드가 다시 붙는 것 하나뿐이었다.

## 판정의 권위 — keep-alive 최근성

항목의 유무로 가르지 않는다. **keep-alive 가 최근에 왔는가**로 가른다.

| 상태 | 무엇이 답하는가 |
|------|-----------------|
| 항목이 있다 | `st.Online` — 하트비트가 `touch` 로 갱신하는 라이브 진실 |
| 항목이 없다 | **영속된 `last_seen` 의 최근성** — `now - last_seen <= HeartbeatTimeout` |

항목의 유무는 "연결 여부" 가 아니라 "이번 부팅 이후 붙었는가" 다. 그 둘을 같은 것으로 쓰면
재시작이 진실을 지운다. keep-alive 는 그 경계를 지나 살아남는 유일한 신호다.

## 요구사항 (Requirements — EARS)

### REQ-01 — 타임아웃이 영속된다 (Event-Driven)

**When** `sweepOnce` 가 하트비트 타임아웃으로 노드를 offline 으로 뒤집으면, **the** 서버
**shall** 그 사실을 영속해야 한다.

`markOffline` 이 이미 쓰는 그 함수(`persistOnline`)를 지난다 — 두 번째 쓰기 경로를 만들지
않는다.

### REQ-02 — 항목이 없으면 keep-alive 최근성이 답한다 (State-Driven)

**Where** 메모리 항목이 없는 노드를 보고할 때, **the** 서버 **shall** 영속된 `online` 과
`last_seen` 을 함께 읽어, `last_seen` 이 `HeartbeatTimeout` 보다 오래되었으면 offline 으로
보고해야 한다.

영속값이 이미 offline 이면 그대로 offline 이다 — 최근성을 묻지 않는다.

### REQ-03 — 판정이 한 자리다 (State-Driven)

**Where** `online` 을 보고할 때, **the** 서버 **shall** 그 판정을 **한 함수**에서 내야 한다.

`ListNodes` 와 `ListPending` 이 같은 답을 내야 하며, 둘이 각자 셈하면 갈린다.

### REQ-04 — 견고성 (Unwanted)

**If** `last_seen` 이 0 이거나 미래이면, **the** 서버 **shall** 던지지 않고 판정을 내야 한다.

`0` 은 "본 적 없음" 이므로 오래된 것으로 읽는다(offline). 미래 값은 시계 왜곡이며 최근으로
읽는다 — 어느 쪽도 패닉이 아니다.

## 명세 (Specifications)

### 결정 1 — 부팅 시 DB 를 일괄 수정하지 **않는다**

`UPDATE managed_nodes SET online = 0` 을 부팅에 넣는 안을 기각한다.

두 인스턴스가 한 DB 를 공유하는 구성이 실재하며(같은 `storage.sqlite.path`), 그때 한 쪽의
부팅이 **다른 쪽에 붙어 있는 노드**의 online 을 지운다. 읽는 쪽을 고치면 그 위험 없이 같은
결과를 얻는다.

### 결정 2 — 청소기가 **repo 를 돌지 않는다**

`sweepOnce` 가 매 tick 마다 전체 행을 읽는 안을 기각한다. REQ-02 가 읽는 쪽을 고치므로
항목 없는 행도 올바르게 보고되고, 청소기의 일은 **라이브 추적 중인** 노드를 뒤집는 것으로
남는다. tick 마다의 전량 조회는 그 일에 필요하지 않은 부하다.

### 결정 3 — `touch` 는 영속하지 않는다 (그대로 둔다)

하트비트마다 DB 에 `last_seen` 을 쓰는 안을 기각한다. 노드 수 × 하트비트 주기만큼의 쓰기가
생기는데, 붙어 있는 노드는 **항목이 있으므로** REQ-02 의 최근성 판정을 지나지 않는다.

대가를 숨기지 않는다: 서버가 오래 돌다 재시작하면, 붙어 있던 노드의 영속 `last_seen` 은
접속 시점의 값이라 오래되어 있고 그래서 **offline 으로 읽힌다.** 그 방향이 안전한 쪽이며
(없는 연결을 있다고 말하지 않는다) 노드가 다시 붙는 순간 스스로 낫는다.

## 불변식 (Invariants)

| # | 불변식 |
|---|--------|
| K1 | `online` 판정은 한 함수를 지난다 |
| K2 | 영속된 `online` 을 판정에 쓰는 제품 코드가 그 함수 하나뿐이다 — 목록과 상세가 같은 답을 낸다 |
| K3 | 항목이 있으면 라이브 추적이 답한다 — 주석이 세운 그 불변식이 참이 된다 |
| K4 | 타임아웃을 영속하는 경로가 `persistOnline` 하나다 |

## 범위 밖 (Non-Goals)

- **부팅 시 일괄 offline** — §결정 1
- **청소기의 전량 조회** — §결정 2
- **하트비트마다 `last_seen` 영속** — §결정 3
- **`requireAdmin` 과 권한표의 이중 게이트** — 조사 중 본 별개 사안

## 검증

### 시험

| 파일 | 수 | 무엇을 지는가 |
|------|---:|----------------|
| `internal/remote/online_authority_test.go` | 11 | 신고 장면(REQ-02) · 견고성(REQ-04) · 청소기 영속(REQ-01) · 재시작 흉내 · 목록과 상세의 일치(REQ-03) |

`go build ./...` 0, `go vet ./...` 0, `go test ./internal/remote/... -count=1` 통과.
전체 `go test ./...` 는 46 패키지 통과. 증거: `.moai/state/verify/spec-remote-online-001/`.

### 가드가 무는지 — 다섯 곳을 뒤집어 확인

| 뒤집은 것 | 우는 시험 |
|-----------|----------:|
| 최근성 판정을 지우고 영속값을 그대로 낸다(결함 되돌리기) | 2 |
| 청소기의 영속을 뗀다 | 2 |
| 상세가 제 손으로 셈한다 | 4 |
| 영속 offline 단축을 뗀다 | 2 |
| 항목 우선을 뒤집는다 | 1 |

### 조사 중 스스로 틀린 자리

진단하며 "영속 `online` 을 읽는 자리는 `ListNodes` 하나" 라고 말했다. **틀렸다.**
`NodeDetail`(`GET /remote/nodes/{instance_id}`)에 **같은 조건부 오버레이의 사본**이 한 벌
더 있었다 — 항목이 있으면 라이브값, 없으면 영속값. 목록만 고쳤다면 상세는 같은 노드를
online 으로 계속 보고했을 것이다.

그래서 REQ-03 을 "판정이 한 자리다" 로 세우고, 락을 잡는 겉면(`OnlineOf`)을 만들어 두
자리가 **같은 함수**를 지나게 했다. 목록은 한 번의 RLock 안에서 돌므로 `onlineLocked` 를
직접 쓴다.

### 기존 플레이크 (귀속 확인)

전체 `go test ./...` 에서 agent 패키지의 시험 몇이 회차마다 다르게 운다
(`TestSerialAgent_WriteNotStarved` · `TestHvacr01Agent_TCPClient_AC_G8_NoWriteInvariant` ·
`TestAgent_ProcessDrain_SkipsACKFrames`).

**이 SPEC 의 것이 아니다.** 근거 둘:

- `internal/agent/century` 는 `internal/remote` 에 **의존하지 않는다**(`go list -deps` 0건)
- **변경을 치운 깨끗한 나무**에서 전체를 3회 돌려 같은 시험이 2회 울었다

`internal/remote` 의 `TestServer_DispatchGroupUpdate_PerArchTargetVersion` 도 같은 부류다 —
깨끗한 나무에서 20회 중 5회 실패했다.

## 참고

- 사용자 신고 2026-09-17 — "xagent04 는 연결도 안되어 있는데 online 으로 표시됨"
- `markOffline` — REQ-01 이 빌려 쓰는 그 한 줄
