# SPEC-MODBUS-010 수용 기준 (acceptance.md)

> Given-When-Then 형식. 각 REQ에 최소 1개 시나리오. 모든 시나리오는 `go test -race`로 검증 가능해야 한다. 대상 패키지: `internal/agent/modbusserver`.

## REQ-01 — 백킹 설정 스키마

### AC-01 (하위 호환: 백킹 없음 = 순수 slave)
- **Given** `backing` 키가 없는 `modbus-gateway` device 설정이 주어졌을 때
- **When** 에이전트를 설정·기동하고 마스터가 FC03 읽기를 보내면
- **Then** upstream 연결을 전혀 수립하지 않고 RegisterMap 저장값으로 응답하며, 동작이 백킹 도입 전과 바이트 단위로 동일하다(특성화 테스트).

### AC-02 (백킹 설정 검증)
- **Given** `backing`이 있으나 `mode==indirect`인데 `poll_interval` 또는 `timeout`이 누락되었거나, tcp인데 host/port가 누락된 설정이 주어졌을 때
- **When** `parseDevicesConfig`가 설정을 파싱하면
- **Then** 설정 오류를 반환하여 부분 적용 없이 거부한다.

## REQ-02 — upstream master 연결

### AC-03 (트랜스포트 재사용 + 연결 수립)
- **Given** tcp 백킹(host/port/unit_id/mode=direct)을 가진 device 설정이 주어졌을 때
- **When** 에이전트가 `Start`되면
- **Then** `internal/agent/modbus`의 `ModbusTransport`를 통해 upstream에 `Connect`가 수행되고(신규 외부 라이브러리 없이), 디바이스가 서빙 준비 상태가 된다.

### AC-11 (연결 종료 수명)
- **Given** 백킹 디바이스가 기동 중일 때
- **When** 해당 디바이스가 `remove_device`로 제거되거나 에이전트가 `Stop`되면
- **Then** upstream 트랜스포트가 `Close`되고 관련 폴러(indirect인 경우)가 종료되며 goroutine 누수가 없다(race/누수 테스트).

## REQ-03 — Direct 모드

### AC-04 (Direct 읽기 = 실시간 조회 + 저장 + 전달)
- **Given** direct 백킹 디바이스와 응답 가능한 fake upstream이 주어졌을 때
- **When** 마스터가 FC03 읽기 요청을 보내면
- **Then** upstream에 즉시 `SendAndReceive`가 발생하고, 응답값이 RegisterMap에 저장되며 동일 값이 마스터에 응답된다.

### AC-05 (Direct 쓰기 = 전달 + 미러)
- **Given** direct 백킹 디바이스와 응답 가능한 fake upstream이 주어졌을 때
- **When** 마스터가 FC06 쓰기 요청을 보내면
- **Then** 쓰기가 upstream으로 전달되고, 성공 시 RegisterMap에 동일 값이 미러 저장된 뒤 정상 응답이 반환된다.

### AC-06 (Direct 끊김 = 즉각 0x0B)
- **Given** direct 백킹 디바이스와 도달 불가한 upstream이 주어졌을 때
- **When** 마스터가 읽기 또는 쓰기 요청을 보내면
- **Then** 대기 없이 MODBUS 예외 `0x0B`(Gateway Target Device Failed to Respond)가 반환되고, 실패한 쓰기는 RegisterMap을 변형하지 않는다.

## REQ-04 — Indirect 모드

### AC-07 (Indirect 폴링 = RegisterMap 갱신)
- **Given** indirect 백킹 디바이스(poll_interval 설정)와 응답 가능한 fake upstream이 주어졌을 때
- **When** poll_interval이 최소 1회 경과하면
- **Then** 폴러가 upstream을 폴링하여 RegisterMap을 갱신하고 마지막 성공 갱신 시각(`lastSuccess`)을 기록한다.

### AC-08 (Indirect 읽기 = 저장값 서빙)
- **Given** 폴러가 이미 RegisterMap을 갱신한 indirect 백킹 디바이스가 주어졌을 때
- **When** 마스터가 FC03 읽기 요청을 보내면
- **Then** upstream을 직접 호출하지 않고 RegisterMap의 저장값으로 응답한다.

### AC-09 (Indirect stale 허용 / 타임아웃 초과)
- **Given** 마지막 성공 갱신 이후 upstream이 도달 불가가 되어 폴이 실패하는 indirect 디바이스가 주어졌을 때
- **When** 경과 시간이 `timeout` 이내인 상태에서 읽기가 오면 **Then** stale 저장값을 정상 응답한다.
- **When** 경과 시간이 `timeout`을 초과한 상태에서 읽기가 오면 **Then** MODBUS 예외 `0x0B`를 반환한다.

### AC-10 (Indirect 쓰기 = 즉시 전달, timeout 무관 예외)
- **Given** indirect 백킹 디바이스가 주어졌을 때
- **When** 마스터가 쓰기 요청을 보내면
- **Then** 폴 캐시를 우회하여 upstream에 즉시 전달하고, 성공 시 RegisterMap 미러; upstream 미도달이면 `timeout` 이내라도 즉각 `0x0B`를 반환한다.

## REQ-05 — 하위 호환 · 예외 · 동시성 · 품질

### AC-12 (예외 코드 0x0B + 순수 slave 회귀 없음)
- **Given** 백킹 디바이스와 순수 slave 디바이스가 공존하는 에이전트가 주어졌을 때
- **When** 백킹 실패로 예외가 발생하고 순수 slave에 정상 요청이 오면
- **Then** 백킹 실패는 `0x0B`(modbusserver 로컬 정의, internal/agent/modbus 미변경)로 매핑되고, 순수 slave 요청은 기존과 동일하게 정상 응답된다.

### AC-13 (동시성 race 클린 + 등록/프론트엔드 보존)
- **Given** 폴러 갱신과 마스터 읽기/쓰기가 동시에 발생하는 indirect 백킹 디바이스가 주어졌을 때
- **When** `go test -race ./internal/agent/modbusserver/...`를 실행하면
- **Then** 데이터 경합이 보고되지 않고, type id `modbus-gateway`가 보존되며, `agentSchemas.ts`/`ModbusServerDevicesEditor`에 백킹 필드가 스키마 주도로 노출된다(신규 React 컴포넌트 없이).

## 품질 게이트 (Definition of Done)

- [ ] AC-01~13 전부 통과 (Given-When-Then 검증)
- [ ] 백킹 없는 디바이스 순수 slave 특성화 테스트 통과(하위 호환 HARD)
- [ ] type id `modbus-gateway` 보존, `go.mod` 신규 modbus 모듈 없음
- [ ] `go test -race ./internal/agent/modbusserver/...` 클린
- [ ] 커버리지 ≥ 85% (신규 백킹 코드 포함)
- [ ] 코드 주석 한국어(`code_comments: ko`), TRUST 5 게이트 통과
- [ ] 폴러 goroutine 누수 없음(remove/Stop 경로 검증)
