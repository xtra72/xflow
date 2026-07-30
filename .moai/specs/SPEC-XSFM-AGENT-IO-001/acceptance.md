# SPEC-XSFM-AGENT-IO-001 — Acceptance Criteria

> Given-When-Then 인수 시나리오. 상위: `spec.md` (RD-1 수신 forward, RD-2 방출 모드, RD-3~6 확정). Tier M.

> **버전 노트**: v0.2.0 (2026-07-30) — OQ-1~4 확정(RD-3~6)에 따라 OQ 의존 시나리오를 구체화: 주기 스냅샷 기본 60s + `{timestamp, devices:[...]}` 단일 배열 shape, forward mode-agnostic(direct·port 양 모드), `device_state_received` 단일 디바이스 shape, 프런트 설정 토글 UI. 신규 AC-2.5(port 모드 forward)·AC-3.8(interval 기본 60s) 추가, AC-2.3/AC-3.5/AC-7.x 갱신.

## §1. 설정 파싱·검증 (Module 3, M1)

### AC-1.1 기본값
- **Given** `Transport.Options` 에 forward/emit 관련 키가 없을 때
- **When** `parseXSFMConfig` 를 호출하면
- **Then** `ForwardReceivedToNode=false`, `StateEmitMode="event"`, `StateEmitInterval=60s`(RD-3) 로 파싱된다.

### AC-1.2 state_emit_mode 유효 enum 수락
- **Given** `state_emit_mode` 가 `event`/`interval`/`both` 중 하나일 때
- **When** 파싱하면
- **Then** 에러 없이 해당 모드로 설정된다.

### AC-1.3 state_emit_mode 위반 거부
- **Given** `state_emit_mode="periodic"`(비허용 값)일 때
- **When** 파싱하면
- **Then** `ErrInvalidStateEmitMode` 를 반환한다(조용한 폴백 없음).

### AC-1.4 state_emit_interval 파싱·음수 거부
- **Given** `state_emit_interval="30s"`(또는 `"-5s"`)일 때
- **When** 파싱하면
- **Then** 30s 로 설정된다(음수는 에러 반환).

### AC-1.5 forward_received_to_node 파싱
- **Given** `forward_received_to_node=true` 일 때
- **When** 파싱하면
- **Then** `ForwardReceivedToNode=true` 로 설정된다.

## §2. 수신 forward 옵션 (Module 1, M2)

### AC-2.1 forward on — 매 수신 방출(무변경 포함)
- **Given** `forward_received_to_node=true` 이고 디바이스 상태가 한 번 관측된 뒤
- **When** **동일한 값**(무변경)의 상태가 다시 유입되면
- **Then** `changed_fields` 가 비어도(변경 없음) `device_state_received` 가 방출된다(매 수신 탭).

### AC-2.2 forward off — 미방출
- **Given** `forward_received_to_node=false`(기본)일 때
- **When** 상태가 유입되면
- **Then** `device_state_received` 는 방출되지 않는다.

### AC-2.3 forward shape — 파싱/정규화 형태 (단일 디바이스)
- **Given** forward on 상태에서 상태가 유입될 때
- **When** `device_state_received` 를 검사하면
- **Then** `type/device_id/online/관측 축(StateForJSON)/timestamp` + meta(station_code 등)를 담은 **단일 디바이스** 메시지이며 원시 브로커 바이트가 아니고 `changed_fields` 를 싣지 않는다(RD-5).

### AC-2.4 forward 는 emit_mode 와 독립
- **Given** `forward_received_to_node=true`, `state_emit_mode="interval"`(on-change 억제)일 때
- **When** 상태가 유입되면
- **Then** on-change `device_state_changed` 는 억제되어도 `device_state_received` 는 매 수신 방출된다.

### AC-2.5 forward 는 mode-agnostic — direct·port 양 모드
- **Given** `forward_received_to_node=true` 이고 (a) direct 모드(`subscribeState`→`handleStateMessage`→`ingestState`) 또는 (b) port 모드(노드가 `FeedState`→`ingestState` 호출)일 때
- **When** 각 모드에서 상태가 유입되면
- **Then** **양 모드 모두** `ingestState` 단일 시임에서 `device_state_received`(파싱/정규화 상태)를 방출한다 — port 모드에서도 건너뛰지 않으며(no-op 특별 케이스 없음, RD-4) 원시 바이트 에코가 아니다.

## §3. 상태 방출 모드 (Module 2, M3)

### AC-3.1 event 모드 — on-change 방출
- **Given** `state_emit_mode="event"`(기본)일 때
- **When** 상태가 **변경**되어 유입되면
- **Then** `device_state_changed` 가 방출되고 주기 방출기 goroutine 은 기동되지 않는다.

### AC-3.2 interval 모드 — 주기 스냅샷 + on-change 억제
- **Given** `state_emit_mode="interval"`, `state_emit_interval` 이 짧게 설정된 상태에서 디바이스 다수가 등록되어 있을 때
- **When** 상태가 변경 유입되고 한 tick 이 경과하면
- **Then** on-change `device_state_changed` 는 **방출되지 않고**, tick 마다 `device_state_snapshot`(전체 디바이스 풀 스냅샷)이 방출된다.

### AC-3.3 both 모드 — 병행 방출
- **Given** `state_emit_mode="both"` 일 때
- **When** 상태가 변경 유입되고 tick 이 경과하면
- **Then** on-change `device_state_changed`(변경 시) **와** 주기 `device_state_snapshot`(heartbeat) 이 **둘 다** 방출된다.

### AC-3.4 주기 스냅샷 — offline 포함(include-all)
- **Given** interval/both 모드에서 online 디바이스와 offline 디바이스가 혼재할 때
- **When** 주기 `device_state_snapshot` 을 검사하면
- **Then** offline 디바이스도 `online:false` 로 포함된(전체 등록 디바이스) 배열을 담는다.

### AC-3.5 주기 스냅샷 shape — 단일 배열 메시지
- **Given** interval/both 모드에서 tick 이 경과할 때
- **When** `device_state_snapshot` 을 검사하면
- **Then** `{type:"device_state_snapshot", timestamp, devices:[deviceStateJSON...]}` **단일 배열 메시지**이며(request_state 응답 shape 재사용, RD-5), N개 per-device 개별 메시지로 방출되지 **않는다**.

### AC-3.6 방출기 미기동 조건
- **Given** `state_emit_mode="event"` 또는 `state_emit_interval<=0` 일 때
- **When** 에이전트를 Start 하면
- **Then** 주기 방출기 goroutine 이 기동되지 않는다.

### AC-3.7 전이 이벤트 불변
- **Given** `state_emit_mode="interval"`(on-change 억제) 상태에서
- **When** 디바이스가 online↔offline 전이하면
- **Then** `device_online`/`device_offline` 전이 이벤트는 방출 모드와 무관하게 그대로 방출된다.

### AC-3.8 interval 기본값 60s + 하한 클램프
- **Given** `state_emit_mode="interval"` 이고 `state_emit_interval` 이 미설정일 때
- **When** `parseXSFMConfig` 를 호출하면
- **Then** `StateEmitInterval=60s`(RD-3, offline_timeout 기본 90s 와 정합)로 파싱되며, 유효 tick 이 `minStateEmitInterval` 미만이면 하한으로 클램프되어 `time.NewTicker` panic 이 발생하지 않는다.

## §4. 생명주기·동시성 (Module 4, M4)

### AC-4.1 goroutine 누수 없음
- **Given** interval/both 모드로 방출기가 기동된 상태에서
- **When** `Stop` 을 호출하면
- **Then** `stopCh` 관측 후 `monitorWG.Wait()` 가 반환되어 방출기 goroutine·타이머가 정리된다(누수 없음).

### AC-4.2 락 경합 없음 (-race)
- **Given** 짧은 interval 로 주기 방출기가 tick 하는 동안 상태 유입·조회가 동시 발생할 때
- **When** `-race` 로 실행하면
- **Then** 데이터 경합이 검출되지 않는다(스냅샷-후-락해제 규율).

## §5. 무회귀 (Module 4, M4)

### AC-5.1 기본 설정 바이트 동일
- **Given** `forward_received_to_node=false`, `state_emit_mode="event"`(기본)일 때
- **When** 상태 변경/무변경이 유입되면
- **Then** 방출은 현행과 동일(변경 시에만 `device_state_changed`)하며 `device_state_received`·`device_state_snapshot` 는 방출되지 않는다.

### AC-5.2 ReceiveMessage 계약 불변
- **Given** 기본 또는 확장 설정에서
- **When** 노드가 `ReceiveMessage` 로 `msgCh` 를 drain 하면
- **Then** 기존 non-blocking·가득 참 시 drop 규약이 그대로 유지된다.

## §6. 품질 게이트 (Module 4)

### AC-6.1 커버리지 & race
- **Given** 전체 xsfm 테스트를
- **When** `go test -race -cover ./internal/agent/xsfm/...` 로 실행하면
- **Then** 커버리지 85% 이상이고 `-race` 클린이다.

### AC-6.2 MQTT 규약 불변
- **Given** 두 기능이 활성/비활성인 상태에서
- **When** MQTT 토픽/페이로드를 검사하면
- **Then** 규약이 불변이다(방출은 `msgCh` 노드 계층에만 작용).

## §7. 프런트엔드 (Module 5, M5 — RD-6 확정, 본 SPEC 범위 내 · 저우선)

### AC-7.1 옵션 UI
- **Given** 에이전트 설정 UI 에서
- **When** `forward_received_to_node` 토글·`state_emit_mode`(event/interval/both) 선택·`state_emit_interval` 입력을 조작하면
- **Then** 해당 설정이 `Transport.Options`(`forward_received_to_node`/`state_emit_mode`/`state_emit_interval`) 로 저장·전달된다.

### AC-7.2 기존 UI 무회귀
- **Given** 신규 옵션 UI 추가 후
- **When** 기존 에이전트 설정 화면을 사용하면
- **Then** 기존 동작이 회귀 없이 유지된다.

## Definition of Done (요약)
- AC-1.x ~ AC-6.x 전부 green(백엔드). AC-7.x(프런트 M5)는 RD-6 확정으로 본 SPEC 범위 내(저우선, 백엔드 완료 후).
- 기본 설정 무회귀(바이트 동일) 검증.
- 커버리지 85%+, `-race` 클린, `go test ./...` exit 0.
