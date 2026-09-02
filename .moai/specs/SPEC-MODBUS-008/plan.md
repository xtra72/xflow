# SPEC-MODBUS-008 구현 계획 (Plan)

> 연관 SPEC: [spec.md](./spec.md) · 수용 기준: [acceptance.md](./acceptance.md)
> 대상: `internal/agent/modbus/` (type id `modbus-client`) 확장 — 신규 패키지 복제 금지.
> 서버 패키지 `internal/agent/modbusserver`(type id `modbus-gateway`)는 F4 참조 패턴만 사용(변경 없음).

---

## 1. 개발 방법론

Hybrid.

- **신규 코드는 TDD**: 클라이언트 프레임 로그(`clientObs`/`logFrame` 포팅), 공유 트랜스포트 참조 카운팅, per-device 트랜스포트 선택 로직 — 테스트 우선(RED-GREEN-REFACTOR).
- **기존 패키지 편집은 DDD**: `config.go`(DeviceConfig/parseDeviceConfig 확장), `agent.go`(buildDevices, Process 스위치, devices/폴링 편입), `transport.go`/`transport_rtu.go`(프레임 hook, 공유) 편집 — 특성화(characterization) 테스트로 `modbus-client` 동작 보존 후 개선(ANALYZE-PRESERVE-IMPROVE).

기존 modbus 클라이언트 테스트는 행위 보존 대상으로 취급하여, 편집 전후 동일 결과를 보장한다(하위 호환 HARD 게이트).

---

## 2. 기술 제약 (Technology Constraints)

| 제약                     | 내용                                                                        |
|--------------------------|-----------------------------------------------------------------------------|
| 언어/버전                | Go 1.23+                                                                     |
| 신규 의존성 금지         | `go.mod`에 신규 외부 modbus 모듈 미추가                                       |
| type id 보존             | `modbus-client` 불변 (register.go, cmd/xflowd/main.go 배선 유지)             |
| 트랜스포트 인터페이스    | `ModbusTransport.SendAndReceive(ctx, unitID, pdu)→respPDU`(transport.go:25) 유지 |
| 설정 소스                | `AgentConfig.Transport.Options`(map[string]any), `parseModbusConfig`(config.go:66-221) |
| 명령 표면                | 기존 `Process` 문자열 스위치(agent.go:801-824) 스타일 준수, 신규 병렬 메커니즘 금지 |
| 스레드 안전성            | 런타임 변경은 `a.mu`(RWMutex) 보호, 공유 접근은 turnaround mutex(transport_rtu.go:23) 준거 |
| 프레임 로그 참조         | `internal/agent/modbusserver/observability.go` `serverObs`/`logFrame` 규약 포팅(명명 "modbus-client: frame") |
| 프론트엔드               | 스키마 주도(`agentSchemas.ts`), 신규 React 컴포넌트 없이 필드 세트 확장(SPEC-006 선례) |
| 하위 호환 (HARD)         | per-device 필드 없음 + share_session 없음 + 프레임 로그 없음 = 기존과 바이트 동일 동작 |
| 코드 주석 언어           | `code_comments: ko`                                                          |

---

## 3. 마일스톤 (Milestones)

시간 예측 없이 우선순위/의존 순서로 기술한다. 구현 순서는 독립성/의존 관계를 반영한다:
**M0 특성화 → M1 F4(가장 독립적) → M2 F2 → M3 F3(F2 위) → M4 F1(F2/F3 활용) → M5 등록/프론트/마감**.

### M0 — 하위 호환 특성화 베이스라인 (Priority High, 선행)
- 기존 `modbus-client` 설정(에이전트 레벨 transport, per-device 필드 없음, share 없음, 프레임 로그 없음)에 대한 특성화 테스트 확보.
- TCP 디바이스별 독립 연결 + RTU 단일 버스 공유 토폴로지, 폴링/읽기/쓰기 동작 스냅샷(회귀 게이트, AC-09).

### M1 — F4 프레임 로그 / Raw 프레임 (Priority High, 독립)
- 게이트웨이 `observability.go` `serverObs`/`logFrame` 규약을 클라이언트로 포팅(`clientObs`: logFrames/logRawFrames atomic.Bool, framesOn/rawOn).
- `transport.go`(TCP SendAndReceive) + `transport_rtu.go`(RTU SendAndReceive) 프레임 경계에 TX/RX hook(dir/unit_id/fc/adu-hex, "modbus-client: frame").
- `log_frames`/`log_raw_frames` 파싱(기본 false, no-op). raw는 frames 활성 시에만 의미(AC-01, AC-02).
- (선택) `set_config`/Configure atomic 토글.

### M2 — F2 디바이스별 트랜스포트 override (Priority High, M0 의존)
- `DeviceConfig`(config.go:46-53)에 선택적 `Transport` + per-device `Serial` 추가. `parseDeviceConfig`(config.go:202) 파싱·검증(RTU override→시리얼 필수, TCP override→host 필수).
- `buildDevices`(agent.go:160-173): 디바이스별 유효 트랜스포트 = per-device override ?? 에이전트 기본. override 없으면 기존 경로 그대로(AC-03, AC-04).

### M3 — F3 세션 공유 opt-in (Priority Medium, M2 의존)
- `share_session`(bool, 기본 false) 파싱. 활성 시 `buildDevices`가 엔드포인트 키((host,port)/serial_port) + 트랜스포트 종류로 그룹화하여 그룹당 공유 트랜스포트 생성.
- 공유 트랜스포트 참조 카운팅(connect/close 소유, 마지막 참조 close), reconnect 소유, `SendAndReceive` 직렬화(turnaround mutex 준거).
- F2 상호작용: 종류/엔드포인트 일치 디바이스만 공유. RTU 단일 버스 공유는 특수 케이스로 보존(AC-05, AC-06).

### M4 — F1 런타임 디바이스 등록 add/remove (Priority Medium, M2·M3 활용)
- 명령 표면(§5.5 확정): 신규 `add_device`/`remove_device` 명령을 `Process` 스위치(agent.go:801-824)에 추가(기존 디스패치 확장).
- add: `parseDeviceConfig` 재사용 검증 → F2 트랜스포트 선택 → 생성·connect → devices/폴링 편입(`a.mu.Lock()`).
- remove: 폴링 제외 → close(F3 공유 시 마지막 참조만). 미존재/중복 ID 원자적 거부.
- 통계 맵(agent.go:177-186) 처리: 런타임 추가 디바이스/그룹까지 확장(확정). init-후 불변이던 맵의 런타임 변경 → a.mu 보호 또는 원자적 교체로 스레드 안전성 확보(AC-08, AC-10).

### M5 — 등록 · 프론트엔드 · 마감 (Priority Medium, 종속 마감)
- `register.go`/`cmd/xflowd/main.go` type id `modbus-client` 보존 확인.
- `agentSchemas.ts` 필드 세트 확장: per-device transport, share_session, 프레임 로그 토글(스키마 주도, 신규 React 컴포넌트 없이).
- 하위 호환 특성화 테스트 최종 검증(M0 스냅샷과 동일)(AC-09, AC-10).

---

## 4. 재사용 맵 (Reuse Map)

| 자산                                              | 처리      | 비고                                                        |
|---------------------------------------------------|-----------|-------------------------------------------------------------|
| `internal/agent/modbus/config.go`                 | 편집(DDD) | `DeviceConfig` per-device transport/serial, `parseDeviceConfig` 검증 |
| `internal/agent/modbus/agent.go` (buildDevices)   | 편집(DDD) | per-device 트랜스포트 선택 + F3 공유 그룹화                  |
| `internal/agent/modbus/agent.go` (Process 스위치) | 편집(DDD) | 신규 `add_device`/`remove_device`(권장) 또는 set_config 확장 |
| `internal/agent/modbus/agent.go` (initRequestStats) | 편집(DDD) | 런타임 추가 디바이스/그룹 통계 처리(확장 or 한계 문서화)     |
| `internal/agent/modbus/transport.go`              | 편집(DDD) | TCP SendAndReceive 프레임 hook, 공유 참조 카운팅            |
| `internal/agent/modbus/transport_rtu.go`          | 편집(DDD) | RTU SendAndReceive 프레임 hook, turnaround mutex 공유 선례   |
| `internal/agent/modbus/set_config.go`             | 참고/편집 | (후보 B 채택 시) set_config에 디바이스 add/remove 확장       |
| `internal/agent/modbusserver/observability.go`    | 참고      | `serverObs`/`logFrame` 규약 포팅 원본 (변경 없음)           |
| `internal/agent/modbusserver/client_registry.go`  | 참고      | 프레임/클라이언트 표면화 참조 (변경 없음)                    |
| `internal/agent/modbus/register.go`               | 확인      | `modbus-client` type id 보존                                |
| `cmd/xflowd/main.go`                              | 확인      | 등록 배선 유지                                              |
| `web/src/config/agentSchemas.ts`                  | 편집      | 필드 세트(per-device transport, share_session, 프레임 토글) |

신규 파일: 클라이언트 관측성 구현 파일(예: `observability.go` 클라이언트판) 및 각 테스트, 공유 트랜스포트 참조 카운팅 헬퍼(필요 시).

---

## 5. 리스크 분석 (Risks)

| 리스크                                        | 영향 | 대응                                                                       |
|-----------------------------------------------|------|----------------------------------------------------------------------------|
| `modbus-client` 행위 회귀 (하위 호환 HARD)    | 높음 | M0 특성화 베이스라인 확보, 전후 동일성 검증(DDD), AC-09 회귀 게이트         |
| F3 공유 연결 수명/경합 (가장 민감)            | 높음 | 참조 카운팅 명세, turnaround mutex 준거 직렬화, 제거 시 마지막-참조 close 테스트 |
| F2×F3 상호작용 복잡성 (종류/엔드포인트 매칭)   | 중간 | 종류+엔드포인트 키 매칭 규칙 명확화, 혼재 케이스 단위 테스트                 |
| F1 런타임 add/remove 시 폴링 goroutine 경합    | 중간 | `a.mu.Lock()` 하 devices/스케줄 변경, 0-device no-op 안전성 전제, 경쟁 테스트 |
| 런타임 추가 디바이스/그룹 통계 미생성          | 중간 | SPEC-006 IN-5 선례 — 확장 or 문서화된 한계로 §7에서 결정                     |
| 프레임 로그 폭주 / 성능 영향                   | 낮음 | opt-in 게이팅(기본 no-op), raw는 frames 활성 시에만, atomic 토글            |
| 실제 하드웨어 RTU 타이밍 정확성                | 잔여 | mock 시리얼로 로직 검증, 실 하드웨어 타이밍은 잔여 위험으로 명시            |

---

## 6. 품질 게이트 (Quality Gates)

- `go build ./...` 클린.
- `go vet ./...` / golangci-lint 클린.
- 코드 커버리지 ≥ 85% (신규 프레임 로그·공유 트랜스포트·per-device 선택 로직 우선).
- `go.mod`에 신규 외부 modbus 모듈 미추가.
- 기존 modbus 클라이언트 테스트 전부 통과(행위 보존, AC-09).
- `go test -race` 클린(F3 공유 연결·F1 런타임 변경 경합 검증).

---

## 7. 설계 결정 (해소됨 — plan 리뷰 사용자 확정)

spec.md §7과 동일. 초안의 미결 5건은 전량 확정되었다:

1. **F1 명령 표면**: 신규 `add_device`/`remove_device` 명령 — 확정(`set_config` 확장 미채택).
2. **F3 플래그 스코프**: 에이전트 레벨 기본 + 선택적 per-device 오버라이드 — 확정.
3. **F3 공유 연결 제거 시맨틱**: 참조 카운팅, 마지막-참조 close — 확정.
4. **F4 raw 프레임 표면화**: 로그 전용 — 확정(이벤트/조회 노출 이연).
5. **F1 런타임 그룹 통계**: init-불변 통계 맵을 런타임까지 확장(신규 스레드 안전성 요구) — 확정.
