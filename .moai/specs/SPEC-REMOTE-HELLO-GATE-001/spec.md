---
id: SPEC-REMOTE-HELLO-GATE-001
version: "0.1.0"
status: implemented
created: 2026-09-17
updated: 2026-09-17
author: xtra
priority: P1
lifecycle_level: spec-first
title: "hello 는 문을 지켜야 한다 — 그리고 거부는 사유를 돌려준다"
phase: run
module: internal/remote
tier: S
tags: "remote, hello, registration, gate, self-heal, defect"
---

# SPEC-REMOTE-HELLO-GATE-001: hello 는 문을 지켜야 한다

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|----------|
| 0.1.0 | 2026-09-17 | xtra | 최초 작성 + 구현 — 사용자 신고("로그에는 연결되었다고 출력되지만, UI에는 보이지 않음") |

## 개요 — M2 에 채우겠다고 적어 둔 자리가 비어 있었다

```go
func (a *bootstrapAuthenticator) Authenticate(_ HelloPayload) error {
	// M1: accept-with-bootstrap-secret-check-stub.
	// M2 seam: 여기서 노드 토큰 검증 / 등록 상태(pending/approved) 게이팅을 수행한다.
	return nil
}
```

주석은 **"M2 에서 게이팅한다"** 고 적었고, `cmd/xflowd/main.go` 는 `NewServer(cfg, nil)` 로
불러 이 스텁을 운영의 문지기로 세웠다. 그래서 hello 는 **무엇이든 수락**했다.

수락된 hello 는 `markOnline` 으로 메모리에 항목을 만들고 `관리 노드 online` 을 찍는다.
그러나 `ListNodes` 는 `repo.List` 만 읽는다. 등록 항목이 없는 instance 라면 그 둘이
갈라진다 — **로그에는 붙어 있고 화면에는 없는 유령**이다. `persistOnline` 은 행이 없을 때의
실패를 Debug 로 삼키므로 INFO 로그만 보면 아무 이상이 없다.

미러는 등록 여부를 묻지 않으므로(`handleInventorySnapshot` 은 `mirror == nil` 만 본다)
유령의 flows/agents/devices 가 미러 테이블에 쌓인다. 목록에 없는 노드의 자원이 서버에
남는다.

## 왜 그 상태에 빠지는가 — 사슬 셋

1. 노드 토큰은 **24시간 access 토큰**이고(`server.basic_auth.token_expiry` 기본 24h),
   서명 키 `jwt_secret` 의 기본값은 빈 문자열이다 — 빈 값이면 부팅마다 32바이트 랜덤을
   새로 뽑는다. **서버 재시작 한 번이 모든 노드 토큰을 무효로 만든다.**
2. 토큰이 무효하면 서버는 `관리 WS 토큰 무효 — 등록 경로로 진행` 을 **Debug 로만** 남기고
   `authedInstanceID` 를 비운다 — INFO 운영 로그에서는 보이지 않는다.
3. 그런데 클라이언트의 `sendHandshake` 는 **토큰을 가지고 있다는 사실만으로** hello 를
   고른다. 토큰은 명시적 `rejected` ack 에서만 지워지므로, 만료·키 교체에는 지우는 자리가
   없다. 노드는 무효한 토큰을 들고 hello 만 되풀이한다.

문지기가 그 hello 를 수락한다. **스스로 풀리지 않는 상태**다 — SPEC-REMOTE-ONLINE-001 이
고친 결함과 같은 모양(권위 없는 판정이 재시작 경계에서 진실을 지움)이 인증 쪽에 한 벌 더
있었다.

## 요구사항 (Requirements — EARS)

### REQ-01 — 미등록 hello 를 거부한다 (Event-Driven)

**When** repo 가 구성된 서버가 hello 를 받고 그 instance_id 의 등록 항목이 없으면,
서버는 그 hello 를 수락하지 **않고** `hello_nack{reason:"unregistered"}` 를 회신한 뒤
연결을 닫는다. 노드 상태를 만들지 않으며 인벤토리도 받지 않는다.

### REQ-02 — 비승인 hello 를 거부한다 (Event-Driven)

**When** 항목은 있으나 상태가 `approved` 가 아니면(pending/rejected/revoked),
서버는 `hello_nack{reason:"not_approved"}` 를 회신한 뒤 연결을 닫는다.

### REQ-03 — 거부는 사유를 돌려준다 (Ubiquitous)

서버는 hello 거부를 **조용히 끊지 않는다**. `hello_nack` 은 노드가 스스로 풀려나기 위한
신호이며, 사유 없는 종료는 같은 토큰으로의 재접속 루프만 만든다. 재접속 복원 거부
(`restoreSession` — 토큰은 유효하나 항목이 없거나 비승인)도 같은 신호를 보낸다.

### REQ-04 — 노드는 거부를 받으면 토큰을 버린다 (Event-Driven)

**When** 노드가 `hello_nack` 을 받으면, 메모리와 디스크의 노드 토큰을 모두 지우고 세션을
끊는다. `rejected` 정지 플래그는 세우지 **않는다** — 그것은 관리자의 명시적 거부에 대한
영구 정지 신호이고, 이쪽은 되돌아가 다시 등록해야 하는 경우다. 재연결 루프가 토큰 없이
dial 하므로 다음 핸드셰이크는 `register` 가 된다.

### REQ-05 — M1 하위 호환 (State-Driven)

**While** repo 가 구성되지 않은 동안(M1 모드), 등록이라는 판정 근거 자체가 없으므로 hello
는 종전처럼 수락된다. 게이트는 server 모드에서만 작동한다.

## 명세 (Specifications)

### 결정 1 — 게이트는 authenticator 가 아니라 hello 분기에 둔다

`Authenticator` 인터페이스는 `HelloPayload` 만 받고 repo 를 모른다. 등록 상태를 물으려면
repo 접근이 필요하므로, 스텁을 고치는 대신 hello 분기에서 `admitHello` 를 지나게 했다.
`bootstrapAuthenticator` 는 전송 계층 신뢰(부트스트랩 시크릿) 자리로 남는다.

### 결정 2 — 자가 복구는 서버의 `register` 경로를 **재사용**한다

새 갱신 프로토콜을 만들지 않았다. `handleRegister` 에는 이미 *"토큰 없이 재접속한 approved
노드 → 새 토큰을 발급해 전달"* 경로가 있다. 노드가 토큰을 버리고 register 로 되돌아가면
그 경로가 그대로 갱신 경로가 된다. 항목이 아예 없으면 pending 으로 큐잉되어 **관리 UI 에
보이고**, 관리자가 승인하면 새 토큰이 내려간다.

### 결정 3 — 승인된 항목의 무인증 hello 는 이번 범위에서 **수락한다**

hello 는 토큰을 운반하지 않으므로, hello 분기에 도달했다는 것은 그 연결이 토큰으로 인증되지
않았다는 뜻이다(인증된 노드는 `?token=` 으로 `restoreSession` 을 지난다). 그러므로 엄격히
보면 repo 가 있는 서버는 **모든** 무인증 hello 를 거부해야 하고, 그러면 24시간 만료도
register 왕복으로 자동 치유된다. 다만 그것은 승인 노드의 동작(현재 `hello` 로 online 이
되는 계약)을 바꾸는 더 큰 변경이므로 이번 범위에서 제외했다 — 범위 밖 ①로 남긴다.

## 불변식 (Invariants)

- **목록과 로그가 갈라지지 않는다**: repo 가 있는 서버에서 `관리 노드 online` 이 찍힌
  instance 는 `ListNodes` 에도 있다.
- **거부는 언제나 사유를 동반한다**: hello/복원 거부 경로는 연결을 닫기 전에 `hello_nack`
  을 쓴다.
- **복구는 노드 혼자 할 수 있다**: 관리자의 수동 개입(노드 데이터 디렉터리의 토큰 파일
  삭제) 없이, 거부 → 토큰 폐기 → register 로 상태가 풀린다.

## 범위 밖 (Non-Goals)

1. **승인 노드의 무인증 hello 거부** — 결정 3 참조. 임의의 instance_id 를 아는 상대가
   승인 노드로 위장해 online 이 될 수 있는 구멍은 남는다(instance_id 는 UUID).
2. **노드 토큰 수명 정책** — 24시간 access 토큰 재사용과 `jwt_secret` 랜덤 기본값은
   그대로다. 운영에서는 `server.basic_auth.jwt_secret` 을 고정해야 한다.
3. **죽은 손잡이 `remote_management.auto_register`** — config/API/UI 에 있으나
   `remote.Client` 가 읽지 않는다. 별건.
4. 미러의 등록 게이트(유령이 남긴 미러 행 정리) — 이번 게이트로 새로 쌓이지는 않는다.

## 검증

### 시험 (internal/remote/hello_gate_test.go)

| 시험 | 고정하는 것 |
|------|-------------|
| `TestHelloGate_UnregisteredRefused` | 미등록 hello → `hello_nack{unregistered}`, 노드 상태 없음, `ListNodes` 0건 |
| `TestHelloGate_NotApprovedRefused` | pending 항목 hello → `hello_nack{not_approved}`, `IsManaged` false |
| `TestHelloGate_M1NoRepoStillAccepts` | repo 미구성 → 종전처럼 수락 (REQ-05) |
| `TestHelloGate_RestoreRefusalSendsNack` | 토큰 유효 + 항목 없음 → 조용히 끊지 않고 사유 회신 |
| `TestClient_HelloNackClearsTokenAndRegisters` | 거부 → 토큰 폐기(메모리·디스크) → 다음 dial 에 토큰 없음 → `register` |

### 가드가 무는지 — 게이트를 뒤집어 확인

`admitHello` 의 미등록 분기를 `return "", true` 로 뒤집으면
`TestHelloGate_UnregisteredRefused` 가 실패한다(확인함). 되돌린 뒤 재통과.

### 전량

`go test ./...` 통과(47 패키지), `go vet ./internal/remote/` 통과,
`internal/remote` 커버리지 85.8%(변경 전 85.7%).

### 시험 자체의 결함 하나

처음 쓴 클라이언트 시험은 dial URL 채널(버퍼 4)에 blocking 송신을 했다. 3회차 이후 dial 이
닫힌 conn 을 되돌려 주어 세션이 즉시 끝나고 재연결이 폭주하면 채널이 차고, `runLoop`
고루틴이 막혀 `Stop()` 의 `wg.Wait()` 가 풀리지 않는다. 같은 판의 시간 의존 시험 두 개가
그 부하에 흔들렸다(깨끗한 나무에서는 2회 연속 통과 — 귀속 확인). non-blocking 송신 +
3회차 이후 dial 실패로 봉합했고, 이후 커버리지 실행 3회 연속 통과.

## 참고

- 같은 모양의 앞선 결함: `.moai/specs/SPEC-REMOTE-ONLINE-001/spec.md`
- 사용자 신고 로그: 서버 `관리 노드 online` + `인벤토리 스냅샷 수신` 이 찍히는데 관리
  노드 화면은 비어 있음(2026-09-17 21:23~21:26, xagent02/xagent03)
