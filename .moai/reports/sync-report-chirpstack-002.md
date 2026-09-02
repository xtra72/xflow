# Sync Report — SPEC-CHIRPSTACK-002

- **SPEC**: SPEC-CHIRPSTACK-002 — ChirpStack status/control 노드 (Tier M, `lifecycle: spec-anchored`)
- **구현 커밋**: `d5d0089d` (branch `feature/SPEC-CHIRPSTACK-002`)
- **sync 수행일**: 2026-08-12
- **문서 언어**: 한국어 (`documentation: ko`)

---

## 1. 범위 (Scope)

SPEC-CHIRPSTACK-001(수신 전용 에이전트) 위에 LoRaWAN **다운링크(제어)** 와 **캐시 통신 상태 조회** 를 Flow 노드로 노출했다. 제어 노출면은 Flow 노드 전용이며 control-panel `ControllableDevice` 어댑터 / REST `/execute` 경로는 범위 밖이다.

| 마일스톤 | 내용 | 상태 |
|----------|------|------|
| M1 | `ChirpStackAgent.PublishMessage` 발행 프리미티브 (`mqtt_agent.go:609` 가드 미러) | 완료 |
| M2 | per-deviceProfile 다운링크 코덱 레지스트리 + Milesight WS301 seed 코덱 + `chirpstack-control` 노드 | 완료 |
| M3 | `chirpstack-status` 노드 (캐시 방출, 0 publish / 0 poll) | 완료 |
| M4 | `registry.go` + `pkg/flow/validate.go` 배선 (노드 타입 71 → 73) | 완료 |
| M5 | 테이블 주도 테스트 + 품질 게이트 | 완료 |

**파일 인벤토리**: 24 파일 (신규 13 / 수정 11), +2491 / -20. 상세는 `plan.md` §6.1.

**REQ-M1-02 (의도적 역전)**: SPEC-001 이 의도적으로 배제했던 발행 경로(`MessagePublisher`)를 본 SPEC 이 명시적으로 역전한다. `agent.go` 의 배제 선언 3곳(패키지 doc / 타입 doc / `Process` doc)을 전부 갱신하고 인터페이스 어서션을 추가했다 — **SPEC-001 회귀가 아니다**.

---

## 2. 분기 요약 (Divergence Summary)

플래닝 기록과 실제 구현이 갈라진 지점. 전체 목록은 `spec.md` § Implementation Notes (IN-1~IN-9), `plan.md` §6.2~§6.3.

| # | 분기 | 결정 |
|---|------|------|
| D-1 | **control 노드 입력 계약** (사용자 결정) | 대상 디바이스를 설정이 아니라 **입력 메시지**로 지정. payload `unit_id`(폴백 `device_id`, 이후 metadata 동일 2키) + `command` + `params` + 선택 `confirmed`. 노드 1개가 N 개 디바이스 담당. `lgap.go` / `xsfm.go` 관용구 미러. |
| D-2 | **status 노드 키잉** (사용자 결정) | 동일하게 payload `unit_id`, 트리거 1건당 메시지 1건. |
| D-3 | **R6 신규 식별** (원 SPEC 미기재) | comm 맵은 `emit_comm_state`(기본 **false**)가 켜져야만 채워진다 → 노브가 꺼진 에이전트의 status 노드는 항상 offline/unknown 방출. 해소: **REQ-M3-01 엄격 준수 + 전제조건 공개** — registry 설명 문자열 + 노드 Go doc 에 명시하고 `Init` 에서 경고 1회. `devices` 로스터 폴백 없음, 하드 실패 없음. |
| D-4 | **REQ-M1-03(Optional) 충족 형태** | `SendDownlink` 에이전트 메서드를 추가하지 **않고**, 패키지 레벨 순수 함수 `BuildDownlinkTopic` / `BuildDownlinkPayload` 를 노드가 조립 후 `PublishMessage` 직접 호출. `internal/node/mqtt.go` thin-adapter 원칙. |
| D-5 | **`applicationId` 캐시 위치** | comm 맵이 아니라 **`devices` 맵**(`upsertDevice` 경유). comm 맵 갱신이 `EmitCommState` 게이트에 종속되므로 거기 캐시하면 제어 경로가 그 노브에 숨은 의존성을 갖는다(D-3 과 동일 함정). |
| D-6 | **패키지 경계 export 3 + 접근자 2** | `internal/node` 가 경계를 넘어 소비 → `BuildDownlinkTopic` / `BuildDownlinkPayload` / `DownlinkTarget` export. `CommStateRecordJSON`(레코드 조립을 에이전트 패키지에 유지해 빌더를 정확히 1개로) / `CommStateEnabled`(D-3 경고용) 접근자 추가. |
| D-7 | **`a.client` 동기화 수정** | 발행 경로가 노드 goroutine 에서 호출 가능해지면서 선재하던 비동기화 쓰기가 실제 레이스가 됨 → `connect()` 의 `a.client` 대입을 `a.mu.Lock()` 하로 이동(동작 보존 최소 수정). |
| D-8 | **base64 = `StdEncoding`** | ChirpStack v4 소스 대조(`integration.proto` `DownlinkCommand`, `src/integration/mqtt.rs`, `influxdata/pbjson` `lib.rs`) — pbjson 은 표준 알파벳 1순위, `-`/`_` 만날 때만 URL-safe fallback. 파서가 `ignore_unknown_fields` 라 **필드명 오타가 무음 실패**하므로 필드명은 골든 벡터 테스트로 고정. |
| D-9 | **하드코딩 단언 1곳 예상 → 3곳** | 노드 타입 수 71 → 73(59 canonical + 14 alias) 반영에 `internal/api/service/node_adapter_test.go`, `internal/node/registry_test.go`, `internal/node/mqtt_test.go` 3곳 갱신 필요. |
| D-10 | **R2/A6 해소** | 플래닝 중 HTTP 401 이 재현되지 않아 WS301 User Guide V1.4 대조 검증 완료. 잔여 예외 `ff 28 ff` 1건. §4 참조. |

---

## 3. 품질 근거 (Quality Evidence)

모두 sync 시점에 직접 실행해 관측한 결과다.

### 3.1 스코프 테스트 / vet / build

```
go test ./internal/agent/chirpstack/... ./internal/node/... ./pkg/flow/... ./internal/api/service/...
  exit=0
  ok  internal/agent/chirpstack  2.218s
  ok  internal/node             10.992s
  ok  internal/node/adapter     (cached)
  ok  pkg/flow                  (cached)
  ok  internal/api/service       3.350s

go vet ./internal/agent/chirpstack/... ./internal/node/... ./pkg/flow/...   exit=0 (출력 없음)
go build ./...                                                              exit=0
```

### 3.2 Race Detector

```
go test -race ./internal/agent/chirpstack/... ./internal/node/    exit=0
  ok  internal/agent/chirpstack  3.136s
  ok  internal/node             11.629s
```

(macOS 링커의 `malformed LC_DYSYMTAB` 경고가 출력되나 이는 툴체인 경고이며 테스트 결과와 무관하다.)

### 3.3 golangci-lint — SPEC-002 파일 지적 0건

```
golangci-lint run ./internal/agent/chirpstack/... ./internal/node/... ./pkg/flow/...   exit=1
  internal/node/deduplicate.go:303:6    func extractValues is unused (unused)
  internal/node/flow_node.go:17:7       const flowNodeConfigMode is unused (unused)
  internal/node/modbus_common.go:82:5   var readOnlyAreas is unused (unused)
  internal/node/modbus_common.go:430:6  func applyMessageOverrides is unused (unused)
  4 issues: * unused: 4
```

4건 모두 **선재 findings** 이며 `lsp-baseline.json` 의 `lint_findings_detail` 과 정확히 일치한다. SPEC-002 신규/수정 파일에서의 지적은 **0건**이다.

### 3.4 커버리지 93.43%

신규 소스 7개 파일 statement 가중 실측 **256/274 = 93.43%** (목표 85% 초과). 커밋 메시지의 93.43% 수치가 sync 시점 재측정으로 정확히 재현되었다.

| 파일 | 커버리지 |
|------|---------:|
| `internal/agent/chirpstack/codec.go` | 95.24% (20/21) |
| `internal/agent/chirpstack/codec_ws301.go` | 100.00% (27/27) |
| `internal/agent/chirpstack/commstate_query.go` | 93.75% (15/16) |
| `internal/agent/chirpstack/downlink.go` | 100.00% (6/6) |
| `internal/agent/chirpstack/publish.go` | 100.00% (18/18) |
| `internal/node/chirpstack_control.go` | 92.52% (99/107) |
| `internal/node/chirpstack_status.go` | 89.87% (71/79) |

패키지 전체 기준 `internal/agent/chirpstack` 은 91.0%.

### 3.5 SPEC-001 characterization 무회귀

프로즌 특성 테스트 파일 2개가 본 커밋에서 **무변경**임을 git 이력으로 확인:

```
git log --oneline -1 -- internal/node/chirpstack_test.go
  88807142 test(chirpstack): M6 comm-state 테이블 테스트 ... (SPEC-CHIRPSTACK-001)
git log --oneline -1 -- internal/agent/chirpstack/comm_state_test.go
  88807142 test(chirpstack): M6 comm-state 테이블 테스트 ... (SPEC-CHIRPSTACK-001)
```

두 파일 모두 최종 수정이 SPEC-001 M6 커밋(`88807142`)이며 `d5d0089d` 의 변경 목록에 없다. 현재 트리에서 통과한다(§3.1).

### 3.6 AC → 테스트 매핑

`acceptance.md` § 실측 결과 표 참조. 8개 AC 항목의 검증 테스트 함수명을 전부 소스 대조로 확인했다(`grep -rn 'func Test' internal/agent/chirpstack internal/node`).

---

## 4. 미해소 항목 (Unresolved)

정직 기록. 아래 3건은 본 sync 시점에 **해소되지 않았다**.

### U-1. 저장소 전체 게이트가 선재 결함으로 RED (본 SPEC 무관)

`go vet ./...` 및 `go test ./...` 는 저장소 전체 기준 RED 다.

```
go vet ./...   exit=1
# github.com/xtra/xflow/internal/schedulelog
vet: internal/schedulelog/observer_test.go:30:39: cannot use (*fakeScheduleLogRepo)(nil)
  ... does not implement storage.ScheduleLogRepository (wrong type for method List)
    have List(context.Context, storage.ScheduleLogFilter, int, int) ([]storage.ScheduleLogRecord, error)
    want List(context.Context, storage.ScheduleLogFilter, int, int, string) ([]storage.ScheduleLogRecord, error)
```

- **원인**: 커밋 `8881d9c7` (SPEC-SCHEDULE-VIEW-001, 로그 정렬/필터 서버사이드)가 `storage.ScheduleLogRepository.List` 시그니처에 `string` 파라미터를 추가했으나 테스트 페이크 `fakeScheduleLogRepo` 를 갱신하지 않았다. `observer_test.go` 의 최종 수정은 그 이전 커밋(`50f97255`)이다.
- **본 SPEC 과의 무관성 증명**: `lsp-baseline.json` 에 clean HEAD 트리(`git archive HEAD`) 추출 후 동일 재현이 기록되어 있다. 결함 위치 `internal/schedulelog/` 는 SPEC-002 가 건드리지 않은 패키지다.
- **영향**: 본 SPEC 의 무회귀 판정은 **절대값이 아니라 delta 기준**이다(`lsp-baseline.json` `regression_gate`): build delta 0, type_errors delta 0, chirpstack 범위 신규 lint 0. "저장소 전체 zero errors" 는 주장하지 않는다.
- **조치**: 본 SPEC 범위 밖. SPEC-SCHEDULE-VIEW-001 측에서 별도 수정 필요.

### U-2. `ff 28 ff` (query device status) 바이트 미확정

`ff 28 ff` 는 공식 `Milesight-IoT/SensorDecoders` 의 `ws301-encoder.js` 에만 존재하고, WS301 User Guide V1.4 및 milesight.com WS301 downlink HTML 문서 어디에도 등재되어 있지 않다. 문서 대조로 확정된 reboot / set_report_interval 과 **확신 수준이 다르다**.

- 구현은 하되 `internal/agent/chirpstack/codec_ws301.go:41-45` 에 `@MX:DEBT` + `@MX:CEILING` + `@MX:UPGRADE` 를 부착해 표기했다.
- **해소 조건**(`@MX:UPGRADE`): 실기 확인 또는 Milesight 기술지원 확인 완료 시 마커 제거.
- 나머지 바이트(fPort 85 / `ff 10 ff` / `ff 03 <lo> <hi>` LE 60~64800s)는 User Guide V1.4 대조로 검증 완료 — R2 전제조건 자체는 충족되었다.

### U-3. 실브로커 / 실기기 E2E 미수행

**모든 발행 단언은 테스트 더블 기준이다.** 실제 ChirpStack 브로커나 실제 WS301 디바이스를 대상으로 한 종단 검증은 수행하지 않았다.

미검증으로 남는 항목:

- 실 ChirpStack 브로커가 본 페이로드를 실제로 파싱해 다운링크 큐에 넣는지.
- 파서의 `ignore_unknown_fields` 특성상 필드명이 틀려도 **에러 없이 무시**되므로, 골든 벡터 테스트는 필드명을 고정할 뿐 브로커 수용을 증명하지 못한다.
- 실제 WS301 디바이스가 인코딩된 TLV 를 수신·해석하는지(U-2 와 별개로 reboot / report-interval 도 실기 미확인).
- `confirmed` 플래그의 ChirpStack 다운링크 큐(FIFO) / confirmed-ack 동작 — v1 범위 밖(A4, fire-and-publish).

---

## 5. 문서 동기화 결과

| 파일 | 변경 |
|------|------|
| `.moai/specs/SPEC-CHIRPSTACK-002/spec.md` | frontmatter `status: completed` / `version: 1.0.0`; A6·R2 원문 보존 + `[run-phase 해소]` 블록 추가; `## Implementation Notes (run-phase 실측)` IN-1~IN-9 신설 |
| `.moai/specs/SPEC-CHIRPSTACK-002/plan.md` | frontmatter 동일 갱신; `## 6. 실측 결과 (as-built)` 신설(파일 인벤토리 / F-1~F-4 / 분기 / 품질), 기존 Traceability 는 §7 로 번호 이동 |
| `.moai/specs/SPEC-CHIRPSTACK-002/acceptance.md` | frontmatter 동일 갱신; DoD 6개 항목 전부 체크 + 근거 주석; TRUST 5 아래 커버리지 실측표 + AC→테스트 매핑표 추가 |
| `.moai/reports/sync-report-chirpstack-002.md` | 신규(본 문서) |
| `CHANGELOG.md` | `[Unreleased]` 에 신규 노드 2종 항목 추가 |
| `README.md` | ChirpStack 섹션에 SPEC-002 하위 항목 추가 |

`internal/` · `pkg/` · `web/` · `cmd/` 는 무변경(문서 전용 sync). 다른 SPEC 디렉터리는 건드리지 않았다.

---

## 6. Traceability

- 선행: SPEC-CHIRPSTACK-001 (completed) — 수신 에이전트 기반.
- 관련: SPEC-DEVICE-001(통합 레지스트리), SPEC-DEVICE-IDENTITY-001(Phase D, ID==UID==UUID v4).
- baseline: `.moai/specs/SPEC-CHIRPSTACK-002/lsp-baseline.json`.
