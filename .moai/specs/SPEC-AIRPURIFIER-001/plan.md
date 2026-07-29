---
id: SPEC-AIRPURIFIER-001
type: plan
version: "0.3.0"
created: "2026-07-28"
updated: "2026-07-28"
author: xtra
---

# SPEC-AIRPURIFIER-001 구현 계획: 지하철 역사 공기청정기 관리 에이전트 (MQTT)

## 1. 작업 분해 (우선도 기반 마일스톤)

시간 예측 대신 우선도(High/Medium/Low)와 마일스톤 순서로 표현한다.

### Primary Goal (M1): 에이전트 코어 + MQTT 트랜스포트 + 설정 시임

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 1 | 센티널 에러 정의 (12종, `ErrInvalidTransportMode` 포함) | `internal/agent/airpurifier/errors.go` | High |
| 2 | AirPurifierConfig 파싱·검증 (**transport_mode**/broker/tls/lwt/토픽템플릿/페이로드매핑, 모드별 검증 분기 REQ-01-13) | `internal/agent/airpurifier/config.go` | High |
| 3 | I/O 경계 추상화: `CommandSink`(명령 출력) + state ingress(상태 입력) 인터페이스 + direct(Paho 래핑, thingplus 패턴) / port(포트) 두 구현 | `internal/agent/airpurifier/transport.go` | High |
| 4 | Device / 로스터 타입 + 관측 플래그 | `internal/agent/airpurifier/device.go` | High |
| 5 | AirPurifierAgent 코어 (Agent + MessageReceiver, Init/Start/Stop/Pause/Resume, transport_mode 배선 분기) | `internal/agent/airpurifier/agent.go` | High |
| 6 | 토픽 렌더링(direct) + 페이로드 매핑 헬퍼(양 모드 공통 인코딩/디코딩) | `internal/agent/airpurifier/mapping.go` | High |

### Secondary Goal (M2): 개별 제어

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 7 | Process 명령 표면 (set_power / set_fan_speed / set_multiple) | `internal/agent/airpurifier/agent.go` | High |
| 8 | 풍량 1/2/3 검증 + power-ON 게이트 정책 | `internal/agent/airpurifier/control.go` | High |
| 9 | 제어 명령 → 명령 토픽 발행 | `internal/agent/airpurifier/control.go` | High |

### M3: 그룹 제어 (NET-NEW)

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 10 | GroupMembers 도출 (로스터 group_id 속성) | `internal/agent/airpurifier/device.go` | High |
| 11 | 그룹 fan-out (멤버별 개별 발행) + 집계 응답 | `internal/agent/airpurifier/group.go` | High |
| 12 | 부분 실패 best-effort + 빈 그룹 처리 | `internal/agent/airpurifier/group.go` | High |

### M4: 상태 모니터링 + 오프라인 감지

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 13 | state 토픽 구독 → 페이로드 디코딩 → 로스터 갱신 | `internal/agent/airpurifier/agent.go` | High |
| 14 | 관측 기반 emit (observed 플래그, power=off 시 fan 생략) | `internal/agent/airpurifier/device.go` | High |
| 15 | LWT + offline_timeout 오프라인 감지 루프 | `internal/agent/airpurifier/monitor.go` | High |
| 16 | request_state 온디맨드 캐시 응답 | `internal/agent/airpurifier/agent.go` | Medium |

### M5: 로깅 (상태 시계열 + 제어 감사)

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 17 | 상태 변경 메시지 JSON (epoch ms timestamp) → msgCh | `internal/agent/airpurifier/agent.go` | High |
| 18 | 제어 감사 기록 (개별/그룹, remote_audit_repository) | `internal/agent/airpurifier/control.go` | High |

### M6: 플로우 노드 + 디바이스 어댑터

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 19 | airpurifier_status 노드 (상태 emit → influxdb_write 연동 + **port 모드 상태 입력 포트**) | `internal/node/airpurifier_status.go` | High |
| 20 | airpurifier_control 노드 (제어 + **port 모드 제어 출력 포트**, 상태 입력 포트와 분리) | `internal/node/airpurifier_control.go` | High |
| 21 | CommandSpec (set_power bool, set_fan_speed enum) + Executor | `internal/device/adapter/airpurifier.go` | High |

### M7: 로스터 영속화 + add/remove/set

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 22 | add_device / remove_device / set_device / list_devices | `internal/agent/airpurifier/agent.go` | High |
| 23 | 로스터 영속화 (GetPersistableDevices + device_id repo 라운드트립) | `internal/agent/airpurifier/persist.go` | High |

### M8: 등록/배선

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 24 | RegisterAirPurifierTypes 타입 등록 | `internal/agent/airpurifier/register.go` | High |
| 25 | main.go 배선 (RegisterAirPurifierTypes 호출) | `cmd/xflowd/main.go` | High |
| 26 | API 어댑터 (agent_adapter.go) | `internal/api/service/agent_adapter.go` | High |

### M9 (Final): 프론트엔드 스키마

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 27 | AIRPURIFIER_FIELDS + 타입 엔트리 | `web/src/config/agentSchemas.ts` | High |
| 28 | i18n 문자열 (en/ko) | `web/src/lib/i18n/{en,ko}.json` | Medium |

### Optional Goal: 예제 + 테스트

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 29 | 단위 테스트 (설정/제어/그룹 fan-out/상태 디코딩/오프라인/영속화) | `internal/agent/airpurifier/*_test.go` | High |
| 30 | 예제 에이전트 YAML (broker + 토픽 템플릿 + 페이로드 매핑) | `examples/agents/airpurifier-mqtt.yaml` | Medium |
| 31 | 예제 플로우 YAML (상태 → influxdb_write) | `examples/flows/airpurifier-monitoring.yaml` | Low |

---

## 1B. v0.3.0 확장 마일스톤 (디바이스 모델 · 제어 의미론)

> v0.3.0에서 추가된 작업. 기존 M1~M9 코어 위에 얹히며, 디바이스 모델(위치 계층 + 역사 레지스트리)과 제어 의미론(응답 대기 + line/station 셀렉터)에 집중한다.

### M10: 디바이스 위치 계층 (station/place/index)

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 32 | Device 구조체에 `Station`/`Place`/`Index` 선택 필드 추가 (하위호환) | `internal/agent/airpurifier/device.go` | High |
| 33 | add_device/set_device가 station/place/index 수용·갱신 | `internal/agent/airpurifier/agent.go` | High |
| 34 | 위치 속성 영속화 라운드트립 (기존 필드 없는 파일 하위호환 로드) | `internal/agent/airpurifier/persist.go` | High |

### M11: 역사 레지스트리 (station → line SSOT)

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 35 | StationEntry 타입 + StationRegistry(인메모리 캐시 + station→line/StationsByLine) | `internal/agent/airpurifier/station_registry.go` | High |
| 36 | 역사 레지스트리 파일 저장소 (device_metadata 패턴, atomic write) | `internal/storage/station_registry_repository.go` | High |
| 37 | 설정 시드(`station_registry`) 로드 + `station_registry_path` 배선 | `internal/agent/airpurifier/config.go` | High |
| 38 | add_station/remove_station/list_stations 런타임 명령 | `internal/agent/airpurifier/agent.go` | Medium |

### M12: 제어 응답 대기 (pending-command 레지스트리)

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 39 | `control_response_timeout` 설정 파싱 (0=비활성) | `internal/agent/airpurifier/config.go` | High |
| 40 | pending-command 레지스트리 (device_id+명령 키잉, 명령별 타임아웃, 동시성) | `internal/agent/airpurifier/pending.go` | High |
| 41 | 제어 발행 경로에 pending 등록 + 에코 대기/타임아웃(`ErrControlTimeout`) 통합 | `internal/agent/airpurifier/control.go` | High |
| 42 | 상태 유입 경로에서 매칭 에코로 pending resolve (양 모드 동일 상관) | `internal/agent/airpurifier/agent.go` | High |

### M13: line/station 셀렉터 일괄 제어 (M4 fan-out 확장)

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 43 | `DevicesByStation` / `DevicesByLine`(레지스트리 경유) 대상 도출 | `internal/agent/airpurifier/group.go` | High |
| 44 | 셀렉터 우선순위(device_id>station>line>group_id) + 멤버별 응답 대기 집계(ok/error/timeout) | `internal/agent/airpurifier/group.go` | High |

### v0.3.0 테스트 (추가)

| 순서 | 작업 | 파일 | 우선도 |
|------|------|------|--------|
| 45 | 위치 속성 라운드트립 / 역사 레지스트리 station→line / 응답 대기 성공·타임아웃 / port 에코 상관 / 동시 명령 / line·station fan-out 집계 단위 테스트 | `internal/agent/airpurifier/*_test.go`, `internal/storage/station_registry_repository_test.go` | High |

---

## 2. 기술 접근 방식

### 2.1 아키텍처 (3계층, MQTT 트랜스포트)

```
                    ┌──────────────────────────────────┐
                    │        AirPurifierAgent           │
                    │   (Agent + MessageReceiver)       │
                    ├──────────────┬────────────────────┤
                    │  MQTTClient (Paho 래핑)            │
                    │   Connect/Subscribe/Publish/LWT   │
                    │              │                     │
                    │  Roster (map[device_id]*Device)   │
                    │   + GroupMembers(group_id)         │
                    │              │                     │
                    │  TopicTemplate + PayloadMapping    │
                    │   (설정 주도 시임)                  │
                    │              │                     │
                    │  msgCh ──────> status 노드 ──> influxdb_write
                    └──────────────┴────────────────────┘
      제어: control 노드 / 디바이스 어댑터 ──> Process() ──> COMMAND 토픽 발행
      감사: 제어 명령 ──> remote_audit_repository
```

### 2.2 듀얼 트랜스포트 모드 + I/O 경계 추상화 (M1 — 핵심 설계)

`transport_mode`(`direct` | `port`, 기본 `direct`)는 **I/O 경계만** 분기하고, 상위 프로토콜/로직 레이어는 두 모드에서 동일하게 재사용한다.

- **I/O 경계 추상화** (REQ-01-11): 두 인터페이스를 정의한다.
  - **명령 출력 경계** `CommandSink`: `direct`=브로커 publish 구현, `port`=제어 출력 포트 emit 구현.
  - **상태 입력 경계** state ingress: `direct`=브로커 구독 콜백, `port`=상태 노드 입력 포트. 두 경로 모두 동일 디코딩(페이로드 매핑)으로 로스터 갱신.
- **direct 구현**: `internal/agent/system/thingplus_agent.go`의 Paho 패턴 참조 — 브로커 URL/TLS/CA/client_id/username/password/QoS 파싱(동일 옵션 키 컨벤션), `Connect()` 자동 재연결 + `OnConnect` 구독 복원, LWT 연결 옵션(오프라인 감지 보조). MQTTClient를 인터페이스로 추상화하여 목 브로커 주입으로 테스트.
- **port 구현**: 브로커 클라이언트를 생성하지 않는다(`client=nil`). 상태는 상태 노드 입력 포트로 유입, 제어는 제어 출력 포트로 방출. 토픽 템플릿은 에이전트가 사용하지 않고(외부 mqtt 노드가 토픽 소유), 페이로드 매핑만 사용. 오프라인 감지는 `offline_timeout` 타임아웃 경로만(LWT 비활성).
- **설정 검증 분기** (REQ-01-13): `direct`는 broker/토픽템플릿 필수, `port`는 이들 미사용. `payload_mapping`은 양 모드 공통 필수.
- **핵심 설계 목표**: 인코딩/디코딩 결과가 두 모드에서 바이트 단위로 동일해야 한다 — 공유 로직 레이어는 I/O 경계를 인터페이스로만 접하고, 브로커 vs 포트 여부를 알지 못한다.

### 2.3 설정 주도 시임 (M1 — 핵심 설계)

토픽/페이로드를 하드코딩하지 않는다:
- `state_topic_template` / `command_topic_template`: `{device_id}` placeholder 포함. `renderTopic(tmpl, id)`가 실제 문자열 생성.
- `payload_mapping`: `power_field` / `fan_speed_field` / `online_field`(선택) + 각 필드의 값 표현(bool/문자열/정수). 상태 디코딩·제어 인코딩이 이 매핑을 경유.
- 매뉴얼 확보 후 설정 값으로 확정 — `/moai:2-run` 단계에서 실제 스키마를 채운다.

### 2.4 2-축 상태 + 개별 제어 (M2)

- 상태: `{power, fan_speed(1/2/3), online}`. 풍량은 power ON에서만 유효.
- `set_fan_speed`는 값 1/2/3 검증. power OFF 시 기본 정책은 거부(`ErrPowerOff`); 설정으로 "전원 선행 발행" 정책 선택 가능.
- `set_multiple`은 power ON 선행 후 fan 발행.

### 2.5 그룹 제어 fan-out (M3 — NET-NEW)

- group_id 지정 명령 → `GroupMembers(group_id)` 순회 → 멤버별 개별 MQTT 발행.
- best-effort: 일부 실패해도 나머지 계속, 멤버별 성공/실패 집계.
- 빈 그룹은 발행 없이 `ErrEmptyGroup` 또는 no-op 집계.
- device_id + group_id 동시 지정 시 device_id(단일) 우선.

### 2.6 상태 모니터링 (M4)

- 등록된 각 디바이스의 state 토픽 구독. 수신 시 페이로드 매핑으로 디코딩 → 로스터 갱신 → 변경 시 msgCh emit.
- 관측 기반 emit: observed 플래그로 미관측 축 생략 (Samsung StateForJSON 규칙).
- 오프라인: LWT + offline_timeout 이중 경로.

### 2.7 로깅 (M5)

- 상태 시계열: 에이전트 → status 노드 → influxdb_write (직접 기록 금지). timestamp는 epoch ms(int64).
- 제어 감사: 개별/그룹 명령을 remote_audit_repository로 기록. 그룹은 group_id + 멤버 목록 포함.

### 2.8 노드/어댑터/배선 (M6~M9)

- status/control 노드 분리 (Samsung/LG 패턴)를 **입력/출력 포트로 확장**: 상태 노드는 port 모드 상태 입력 포트(상류 mqtt-in 연결), 제어 노드는 port 모드 제어 출력 포트(하류 mqtt-out 연결)를 가지며, 두 포트는 별개(REQ-01-12).
- CommandSpec: set_power(bool) + set_fan_speed(enum "1"/"2"/"3"), Executor는 Process() 브리지.
- RegisterAirPurifierTypes를 main.go(:376-400 부근)에 배선 (프로젝트 컨벤션: 정의만으로는 부족).
- API 어댑터 + 프론트엔드 AIRPURIFIER_FIELDS + i18n.

### 2.9 디바이스 위치 계층 + 역사 레지스트리 (M10~M11 — v0.3.0 핵심 설계)

- **위치 계층**: 기존 자유 텍스트 `Location`(device.go:117) 대신 구조화 속성 `Station`/`Place`/`Index`를 로스터 Device에 추가. 세 필드 모두 선택/하위호환(zero-value 허용). `index`는 station(+place) 범위 시퀀스.
- **line은 디바이스 속성 아님**: line(호선)은 station 상위 집계 레벨. 디바이스는 station만 보유하고 line은 `ResolveLine(station)`로 해석 → 계층 SSOT를 역사 레지스트리 단일 지점에 둠.
- **역사 레지스트리 저장소**: `internal/storage/device_metadata.go`의 `DeviceMetadataFileRepository`(단일 JSON + atomic rename + 인메모리 캐시)를 **그대로 미러**하여 `station_registry_repository.go` 신설(`{dir}/station_registry.json`). device_metadata와 **별개 저장소**로 관심사 분리(REQ-02-12).
- **표면**: `UpsertStation`/`RemoveStation`/`GetStation`/`ResolveLine`/`ListStations`/`StationsByLine`. 설정 시드(`station_registry`)로 초기 로드 후 런타임 CRUD.

### 2.10 제어 응답 대기 (M12 — v0.3.0 핵심 설계)

- **응답 = 상태 에코**(별도 ack 아님). 성공 = `control_response_timeout` 내 유입 상태가 commanded 변경 반영(상관 by device_id+명령).
- **pending-command 레지스트리**: `map[key]*pending`(key = device_id + 명령 종류), 각 pending은 자체 타이머/타임아웃. 제어 발행 전 등록 → 상태 유입 경로가 매칭 에코 시 resolve → 타임아웃 시 `ErrControlTimeout`.
- **비동기 에코**: 특히 port 모드에서 에코는 상태 입력 포트로 비동기 도착. direct/port 동일한 디코딩·상관 경로로 resolve(공유 로직 레이어 재사용).
- **엣지**: 늦은 에코(만료 pending 무시), 동시 명령(명령 종류별 독립 pending), timeout=0(fire-and-forget).
- **구현 주의**: 채널/타이머 기반 대기 시 goroutine 누수·경합 방지(context 취소 + pending 제거). RWMutex로 pending 맵 보호.

### 2.11 line/station 셀렉터 일괄 제어 (M13 — M4 확장)

- 셀렉터 우선순위 결정론: `device_id`>`station`>`line`>`group_id`.
- `station`→`DevicesByStation`, `line`→`StationsByLine`→station별 `DevicesByStation` 합집합(line→stations→devices 역방향 조회).
- 각 멤버는 개별 제어(M3)의 검증·인코딩·**응답 대기**를 재사용(중복 구현 금지, REQ-04-08). 집계에 멤버별 `ok`/`error`/`timeout`.
- 미등록 station 참조 디바이스는 line 대상에서 제외 + 집계 표기(REQ-02-13).

---

## 3. 리스크 및 대응

| 리스크 | 영향 | 대응 |
|--------|------|------|
| 토픽/페이로드 시임 확정 지연 (매뉴얼 미확보) | 상태 디코딩·제어 인코딩 미완 | 시임을 설정 주도로 완전 분리 → 매뉴얼 확보 시 설정만 채우면 동작. 코드 경로는 매핑 기반으로 선구현·테스트(목 매핑). |
| 두 트랜스포트 모드의 공유 로직 레이어 동작 불일치 | direct/port 간 인코딩·디코딩 결과 미세 차이로 회귀/이중 검증 부담 | I/O 경계를 인터페이스(`CommandSink` + state ingress)로만 접하는 순수 로직 레이어로 격리 → 동일 로직이 두 모드에 주입되도록 설계. 목 sink/ingress로 두 모드가 동일 매핑 결과를 산출함을 assert(바이트 단위 동일성 테스트). |
| port 모드 메시지 봉투(envelope)/포맷 미정 | 입력 포트 유입 메시지·제어 출력 포트 방출 메시지의 구조 불확정 | 입력/출력 포트 메시지 포맷을 페이로드 매핑 시임과 동일 계층으로 표현(토픽은 외부 노드 소유). `/moai:2-run` 단계에서 mqtt-in/out 노드 계약과 함께 봉투 포맷 확정. 상태 입력 포트 ≠ 제어 출력 포트 분리 불변식 테스트로 고정. |
| 그룹 fan-out 부분 실패 의미론 | 일부 멤버만 제어되어 상태 불일치 | best-effort + 멤버별 집계 응답 + 그룹 감사 로그로 가시성 확보. 재시도/롤백은 본 SPEC 범위 외(후속). |
| MQTT 재연결/LWT 오프라인 판정 신뢰성 | 오탐/미탐 오프라인 | LWT + offline_timeout 이중 경로. 재연결 시 구독 복원. SPEC-HVACR-CONNSTATE-001 규약 정합. |
| device_id 키 충돌 (에이전트 이름 중복) | 로스터/영속화 혼선 | device_id 기준 키잉 준수(프로젝트 메모리 컨벤션), 설정 우선 복원. |
| 관측 전 기본값 방출로 대시보드 회귀 | power/fan "꺼졌다 켜짐" 오표시 | 관측 기반 emit (observed 게이트) — Samsung v1.21.0 회귀 교훈 반영. |
| port 모드 에코 상관 실패 | 응답 대기가 항상 타임아웃 → 오탐 실패 | 에코 상관을 device_id+명령 키로 명시. direct/port 동일 디코딩·상관 경로를 공유 로직 레이어로 격리하고, port 입력 포트 유입 에코가 pending을 resolve함을 목 입력 포트로 assert. |
| 응답 타임아웃 튜닝 (기기별 응답 지연 편차) | 너무 짧으면 오탐 timeout, 너무 길면 UI 지연 | `control_response_timeout` 설정화(기본 5s, 0=fire-and-forget). 매뉴얼/현장 확보 후 조정. 늦은 에코는 상태 갱신은 하되 pending 소급 resolve 금지. |
| 동시 명령 pending 경합/goroutine 누수 | 동일 디바이스 다중 명령 시 잘못된 resolve, 리소스 누수 | 명령 종류별 독립 pending 키잉 + RWMutex 보호 + context 취소로 타이머/goroutine 정리. 동시 명령 독립 판정 테스트로 고정. |
| 역사 레지스트리 SSOT vs device_metadata 저장소 중첩 | station 계층이 두 저장소에 분산되어 혼선 | 역사 레지스트리를 device_metadata와 **별개 파일 저장소**로 분리(REQ-02-12). line은 레지스트리에서만 SSOT로 관리, 디바이스는 station만 보유(중복 저장 금지). |
| 미등록 station 참조 | line fan-out 대상 누락을 조용히 삼킴 | `ResolveLine`/`GetStation` 미등록 시 `ErrStationNotFound`. line 대상 선정 시 미등록 station 디바이스 제외 + 집계/로그 표기(REQ-02-13). |
| 대시보드 범위 침범 | 패널 UI가 본 SPEC에 유입되어 범위 팽창 | 본 SPEC은 디바이스 계층 + 역사 레지스트리 + fan-out **표면만** 소유. 시각화/패널은 SPEC-FACILITY-DASHBOARD-001로 명확히 경계(Non-goals 재확인). |

> **Tier 판단**: 본 SPEC은 9개 모듈·다수 파일(에이전트 신규 패키지 + 노드 2종 + 어댑터 + 배선 + 프론트엔드)에 걸쳐 15개 이상 파일을 수정/생성하므로 Tier L 논의 여지가 있다. 다만 각 계층이 기존 선례(Samsung/thingplus)를 강하게 재사용하고 신규 설계 표면은 (a) MQTT 시임과 (b) 그룹 fan-out 두 곳에 집중되므로, **Tier M(3파일)로 진행**하여 모멘텀을 유지한다. 그룹 fan-out/시임의 설계 복잡도가 실제 구현 중 확대되면 후속에서 design.md/research.md 추가(Tier L 승격)를 검토한다.
>
> **v0.3.0 Tier 재검토**: 위치 계층(M10)·역사 레지스트리(M11)·응답 대기(M12)·셀렉터 fan-out(M13)이 추가되어 신규 설계 표면이 (c) 역사 레지스트리 SSOT와 (d) 응답 대기 pending 상관 두 곳으로 늘었다. 각각 device_metadata 저장소 패턴과 pending-timeout 표준 패턴을 재사용하므로 **Tier M 유지**하되, 응답 대기 동시성/에코 상관의 실제 구현 복잡도가 확대되면 M12 한정으로 design.md 분리(Tier L 승격)를 재검토한다.
