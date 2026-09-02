# Sync Report — SPEC-TRIGGER-SCHED-001

- **SPEC**: SPEC-TRIGGER-SCHED-001 — 설비 제어 예약 패널 (스케줄 규칙 테이블 + 모달 편집)
- **동기화 일자**: 2026-07-31
- **구현 커밋**: `03e8827b`(백엔드 M1~M2 — 스케줄 규칙 메타 + 발화 게이팅) + `fb40c4b2`(프런트 M3~M6 — 설비 제어 예약 패널)
- **브랜치**: `develop`
- **버전**: v0.2.0 → v0.3.0
- **SPEC lifecycle**: spec-anchored (Tier L), Level 2 (as-implemented 정련 기록)
- **Phase**: /moai sync (문서 동기화 + as-implemented 정련 + 3-phase close)

---

## SPEC 상태 전이

| 파일 | 항목 | 이전 | 이후 |
|------|------|------|------|
| spec.md | `status` | `draft` | `completed` |
| spec.md | `version` | `0.2.0` | `0.3.0` |
| spec.md | `updated` | `2026-07-31` | `2026-07-31` (유지) |
| plan.md / acceptance.md / design.md / research.md | 버전 노트 | v0.2.0 | v0.3.0 추가 |

- spec.md HISTORY 에 `0.3.0` 행 추가(전 마일스톤 M1~M6 구현 완료 + 3-phase close).
- spec-anchored Level 2 규율에 따라 spec.md 에 **§8 구현 노트(as-implemented) IN-1~IN-7** 신설.
- spec.md 12-field frontmatter 스키마 유효(전 필드 canonical 명칭 유지, snake_case 별칭 없음).

> **마일스톤 명명 정정**: sync 지시 요약은 "M1~M7" 로 표기했으나, plan.md 정의 마일스톤과 두 구현 커밋 라벨은 **M1~M6** 이다(백엔드 커밋 M1 = 모델+게이팅[plan M1~M2], 프런트 커밋 M2~M6[plan M3~M6]). 검증 가능한 사실에 따라 문서에는 **M1~M6** 로 기록했다(존재하지 않는 M7 미기재 — verification-claim-integrity 준수).

---

## 동기화된 문서

| 파일 | 변경 요약 |
|------|-----------|
| `.moai/specs/SPEC-TRIGGER-SCHED-001/spec.md` | frontmatter `status: draft→completed`, `version: 0.2.0→0.3.0`; HISTORY 0.3.0 행 추가; **§8 구현 노트(as-implemented) IN-1~IN-7 신설** |
| `.moai/specs/SPEC-TRIGGER-SCHED-001/plan.md` | v0.3.0 버전 노트 추가(전 마일스톤 구현 완료 + close, as-implemented → spec.md §8 포인터) |
| `.moai/specs/SPEC-TRIGGER-SCHED-001/acceptance.md` | v0.3.0 버전 노트 추가(AC-1~13 전량 통과 + AC↔IN 매핑 + 검증 지표) |
| `.moai/specs/SPEC-TRIGGER-SCHED-001/design.md` | v0.3.0 버전 노트 추가(설계→구현 실현 매핑, §2~§5 ↔ IN-1/2/4/5/7 포인터) |
| `.moai/specs/SPEC-TRIGGER-SCHED-001/research.md` | v0.3.0 버전 노트 추가(정찰 기준선 구현 검증 확인) |
| `CHANGELOG.md` | `[Unreleased]` 최상단에 "추가 — 설비 제어 예약 패널 (규칙 테이블 + 모달) + Trigger 스케줄 규칙 메타·발화 게이팅" 항목 추가(기능 + 분기 7건 + 품질 요약) |
| `.moai/project/structure.md` | 대시보드 패널 섹션에 "설비 제어 예약 패널 + Trigger 스케줄 규칙 메타·발화 게이팅" 문단 추가(파일·와이어링) |
| `.moai/project/product.md` | Dashboard 기능 목록에 "설비 제어 예약 패널" 불릿 추가 |

### 동기화하지 않은(스킵) 문서 — 사유 명시

| 파일 | 사유 |
|------|------|
| `README.md` | 저가치 스킵. 대시보드 패널 전용 섹션이 README 에 없고, 본 기능은 example YAML 추가를 수반하지 않음 → 스킵해도 README 실질 완전성 불변(scope discipline 준수). |
| `.moai/project/tech.md` | 중복. 신규 기술 스택·외부 의존성 없음(기존 Go/React 스택 + 기존 flow/panel/node config API 재사용). |
| `.moai/project/architecture.md` | 해당 없음. 백엔드는 기존 노드 발화 경로 내 최소 침습 게이트 삽입, 프런트는 기존 패널 시스템 가산으로 상위 아키텍처 변경 없음. |

---

## as-implemented 분기 (Divergence — spec-anchored Level 2)

계획(§4/§5) 대비 7건의 정련이 있으며, 모두 **정확성·하위호환을 강화**하는 방향이다(스코프 확장 아님). spec.md §8 IN-1~IN-7 에 정식 기록됨.

| # | 항목 | 계획 | 실제 구현 | 판정 |
|---|------|------|-----------|------|
| IN-1 | `Enabled` 타입 | `bool`(§4.1) | **`*bool` 트라이스테이트** — config 부재→`nil`→`true`(무회귀), 명시 `false` 구별 | 무회귀 강화 정련(REQ-01-01/02) |
| IN-2 | 날짜 포맷/파싱 실패 | `YYYY-MM-DD`, 파싱 실패→미발화(§4.2) | `YYYY-MM-DD` **+ RFC3339 수용**, 비교는 서버 로컬·날짜 단위, 잘못된 날짜→**경계 무제한** | 오발화 방지 정련(REQ-01-04/RD-8) |
| IN-3 | 메타 pass-through | priority/name 통과 전달(REQ-01-06) | `rule_name`/`priority` **조건부** 메타(미지정 규칙 방출 byte-identical) | 무회귀 강화 정련 |
| IN-4 | TARGET 열거 | 셀렉터 종류 UI(§4.5) | 패널 config **`agentId` 기반 열거** — 설정 시 id 셀렉터(useStations/useGroups/useXsfmDevices), 미설정 시 free-form + 이름 셀렉터 폴백 | 피커 구현 구체화(REQ-04-02~05) |
| IN-5 | ACTION 인코딩 | command+params 조립(§4.3) | 내부 **`{power:bool\|null, fanSpeed:number\|null}`**, 무-축/범위밖 저장 차단, `mode` 키 무방출 | RD-4 구체 구현(REQ-05-01~06) |
| IN-6 | PLAN 편집기 | 스케줄 타입+세부 폼(§4.5) | `TriggerScheduleEditor` **단일 원소 배열 재사용**(타이밍 키 전용, payload 는 ACTION 소유) | 단순성 사다리 재사용 |
| IN-7 | dual-write 경로 | 선행 dual-write 재사용(REQ-06) | STATE 토글 포함 **단일 `persist()`** 경로가 `triggerPanelUtils` 재사용(LIVE→PERSIST→404→last-write-wins 계승) | RD-9 구체 구현 |

---

## 확정 설계 결정(RD-1~9) 반영 확인

| 결정 | 구현 반영 |
|------|-----------|
| RD-1 모델 확장 + 백엔드 존중 | `TriggerSchedule` 5필드 + `makeHandler` 게이팅(비활성/기간외 미발화) |
| RD-2 payload = xsfm 제어 명령 | `TargetPicker`(셀렉터) + `ActionEditor`(set_power/set_fan_speed/set_multiple) 조립 |
| RD-3 테이블 + 모달 UX | 6컬럼 읽기 전용 테이블 + 행별 EDIT + STATE 토글 + "+" 모달 |
| RD-4 ACTION 2축(모드 없음) | `ActionEditor` 전원/풍량만, payload `mode` 키 무방출(IN-5) |
| RD-5 신규 패널 공존 | 신규 `facility-schedule` PanelType, 범용 `trigger-config` 무변경 병존(AC-13) |
| RD-6 "전체" = line | `TargetPicker` 전체→`line` 셀렉터(호선 전체), 전역 "all" 미조립 |
| RD-7 priority 표시/정렬 전용 | PRIO 컬럼 + 안정 정렬, 런타임 충돌 해소 없음 |
| RD-8 유효기간 게이팅 | `withinValidity` 서버 로컬·날짜 단위 양끝 inclusive·빈=무제한, 발화=enabled AND 범위(IN-2) |
| RD-9 지속성 | 확장 필드 노드 config dual-write persist(별도 저장소 없음, IN-7) |

---

## 품질 게이트 (관측 증거)

### sync-phase 직접 재검증 (docs-only, 코드 무변경)

| 항목 | 명령 | 결과 |
|------|------|------|
| 백엔드 발화 게이팅 테스트(-race) | `go test -race -run 'Sched\|Validity\|Enabled\|FireGat\|Priority\|RuleName' ./internal/node/` | exit=0, `ok github.com/xtra/xflow/internal/node 4.286s` (`.moai/state/verify/trigger-sched/m1-sched.log`) |
| 구현 커밋 존재 | `git show --stat 03e8827b fb40c4b2` | `03e8827b`: trigger.go(+145) + trigger_sched_test.go(+337); `fb40c4b2`: 13파일 +2409(FacilitySchedulePanel/FacilityRuleModal/TargetPicker·ActionEditor 포함 facilityScheduleUtils + AddPanelDialog/uiStore/renderDashboardPanel 와이어링 + i18n + 테스트) |

> 참고: `-race` 재검증은 동일 `internal/node` 패키지에 무관한 미커밋 변경(mqtt_test.go/registry.go/xsfm.go 등)이 존재하는 상태에서 수행되어 exit 0 로 통과했다. macOS `ld` LC_DYSYMTAB warning 은 benign 링커 경고로 테스트 실패가 아니다.

### run-phase 보고 지표 (커밋 시점 — 본 sync 에서 재실행하지 않음)

- 백엔드 M1~M2(`03e8827b`): 발화 게이팅/유효기간 경계/트라이스테이트/메타 pass-through 신규 함수 커버리지 100%, `-race` 클린, node test green.
- 프런트 M3~M6(`fb40c4b2`): vitest 2370 pass(+50), `tsc`/eslint 클린.
- 전체: `go test ./...` exit 0(42 pkgs), 무회귀.

> **미검증(Gaps)**: 전체 `go test ./...`(42 pkgs), 프런트 vitest 2370/`tsc`/eslint, 백엔드 커버리지 100% 는 본 sync(docs-only)에서 재실행하지 않고 run-phase 커밋 보고를 인용했다. 본 sync 에서 **직접 관측**한 것은 발화 게이팅 테스트(-race, `internal/node`, exit 0)와 두 구현 커밋의 stat 뿐이다.
>
> **잔여 위험(Residual risk)**: (a) `-race` 재검증이 미커밋 동일-패키지 변경과 함께 실행되어 커밋-격리 결과와 미세 차이 가능(관측 결과는 통과). (b) 프런트 지표는 재실행 없이 인용 → 커밋 이후 미커밋 web 변경(nodeSchemas 등)이 있으면 현 트리 vitest 수치는 보고값과 다를 수 있음. (c) 전체 스위트(42 pkgs)는 본 sync 에서 미실행 — 무회귀 주장은 run-phase 커밋 보고에 근거한다.

---

## 비고

- 소스 코드/테스트 파일(`internal/**`, `web/**`)은 **수정하지 않았다**(문서/SPEC 전용).
- 무관한 pre-existing 미커밋 변경(`internal/node/mqtt_test.go`·`registry.go`·`registry_test.go`·`xsfm.go`·`xsfm_test.go`, `web/src/config/nodeSchemas*`, `web/src/lib/flow/nodeType.ts`, 루트 `data.json`/`output.json`/`packet.json`, `web/data/` 등)은 **손대지 않았다**.
- 커밋/푸시/PR 생성은 하지 않았다(오케스트레이터가 sync 커밋 + PR 을 별도 처리).
