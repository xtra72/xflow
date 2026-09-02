# SPEC-DASHBOARD-003 — 구현 계획 (plan.md)

## 관련 문서
- 명세: `spec.md`
- 인수 기준: `acceptance.md`

## 기술 접근 (Technical Approach)

백엔드 + 프론트 양면 확장. SPEC-DASHBOARD-002 가 이미 구축한 `agent-status` 패널 위에 두 능력을 얹는다.

- 백엔드: 기존 옵셔널 인터페이스 패턴(`ConnectionStatsProvider`)을 그대로 복제해 `SummaryStatsProvider`
  를 신설하고, 어댑터의 `Connections`/`NodeRefs` 조건부 채움과 동일한 타입 단언 방식으로 DTO 신규
  옵셔널 필드를 채운다. 데이터는 기존 `/agents/{id}/stats` 응답 및 SSE 에 그대로 실린다(신규 엔드포인트
  없음). 첫 구현체는 xsfm 하나로 한정한다.
- 프론트: DTO 를 TS 타입에 미러링하고, `AgentStatusPanel` 에 인라인 SVG 다이어그램 뷰를 추가한 뒤
  `config.viewMode` 로 diagram|tile 을 전환한다. 기존 타일 렌더는 그대로 보존하고 요약 카운트 영역만
  두 뷰에 공통 추가한다. 선택기는 `AgentStatusSettingsSection` 의 agent-picker `<select>` 를 미러링한다.

개발 방법론: hybrid — 신규 Go 인터페이스·xsfm 구현·SVG 다이어그램 컴포넌트·viewMode 분기는 신규 코드
(TDD, 신규 코드 85% 목표); 어댑터/DTO/패널 기존 파일 수정은 DDD(회귀 방지 특성화 관점).

## 재사용 맵 (Reuse Map — file:line)

| 목적 | 재사용/참조 대상 | 파일:라인 |
|------|------------------|-----------|
| 옵셔널 인터페이스 선례(복제 대상) | `ConnectionStatsProvider` | `internal/agent/agent.go:94-99` |
| 옵셔널 인터페이스 선례(추가 참조) | `BufferInfoProvider` / `InternalStatsRecorder` | `internal/agent/agent.go:71-73` / `:104-108` |
| 어댑터 조건부 채움 선례(타입 단언) | `Connections`/`NodeRefs` 채움 | `internal/api/service/agent_adapter.go:466-526` |
| 어댑터 진입점 | `AgentServiceAdapter.AgentStats` | `internal/api/service/agent_adapter.go:390-529` |
| DTO(신규 필드 추가 대상) | `AgentStatsInfo` | `internal/api/handler/agent.go:126-149` |
| DTO 중첩 응답 선례 | `ConnectionStatsResponse` / `NodeRefStatsResponse` | `internal/api/handler/agent.go` |
| xsfm 장비 로스터(등록 수) | `ListDevices()` (RLock, `len(a.devices)`) | `internal/agent/xsfm/agent.go:949-957` |
| xsfm 온라인 필드(동작 중 수) | `Device.Online` | `internal/agent/xsfm/device.go:28` |
| TS DTO 미러 대상 | `AgentStatsInfo` | `web/src/types/agent.ts:86-109` |
| 확장 패널(뷰 추가 대상) | `AgentStatusPanel` (StatCard, EnhancedMessagesStats 조건부) | `web/src/pages/dashboard/panels/AgentStatusPanel.tsx:31-38,164-205` |
| 인라인 시각화 선례(자족·data-testid) | `RegisterMapGrid` | `web/src/pages/dashboard/panels/modbus/RegisterMapGrid.tsx` |
| 설정 섹션(선택기 미러 대상) | `AgentStatusSettingsSection` agent-picker `<select>` | `web/src/pages/dashboard/PanelSettingsDialog.tsx:1276-1312` |
| 상태 배지/connected 팔레트 | `AgentStatusPanel` 헤더 배지 | `AgentStatusPanel.tsx:117-131` |
| i18n 키 파일(기존 agentStatus 블록) | `ko.json` / `en.json` | `web/src/lib/i18n/{ko,en}.json:230-236` |

## 산출/수정 파일

신규:
- `web/src/pages/dashboard/panels/AgentStatusDiagram.tsx` — 인라인 SVG 메시지 흐름 다이어그램 컴포넌트(External→Agent→Internal, 화살표 라벨, 보조 지표).
- `web/src/pages/dashboard/panels/AgentStatusDiagram.test.tsx` — 다이어그램 렌더/폴백 테스트.
- (백엔드) provider 인터페이스 및 xsfm 구현 테스트(`agent_test.go` / `xsfm/*_test.go` 확장).

수정:
- `internal/agent/agent.go` — `SummaryStat` struct + `SummaryStatsProvider` 인터페이스 추가.
- `internal/api/handler/agent.go` — `SummaryStatResponse` + `AgentStatsInfo.SummaryStats` 필드.
- `internal/api/service/agent_adapter.go` — `SummaryStatsProvider` 타입 단언 채움(조건부).
- `internal/agent/xsfm/agent.go`(또는 인접 파일) — `SummaryStats()` 구현(등록/동작 중 장비 수).
- `web/src/types/agent.ts` — `AgentSummaryStat` + `AgentStatsInfo.summary_stats` 미러.
- `web/src/pages/dashboard/panels/AgentStatusPanel.tsx` — viewMode 분기 + 요약 카운트 영역(두 뷰 공통) + diagram 뷰 배선.
- `web/src/pages/dashboard/panels/AgentStatusPanel.test.tsx` — viewMode/요약/graceful 테스트 확장.
- `web/src/pages/dashboard/PanelSettingsDialog.tsx` — `AgentStatusSettingsSection` 에 viewMode `<select>` 추가.
- `web/src/lib/i18n/ko.json`, `web/src/lib/i18n/en.json` — 신규 라벨 키.

## 아키텍처 방향

- 백엔드 확장은 순수 가산형: 기존 flat/중첩 DTO 필드를 변경하지 않고 `summary_stats` 옵셔널 필드만
  추가한다. 어댑터는 인터페이스 미구현 시 필드를 채우지 않아 omitempty 로 응답에서 생략된다.
- provider 인터페이스는 타입 유의미 카운트를 안정적 key + 정수 value 목록으로 반환하는 범용 형상으로,
  이후 다른 에이전트 타입이 자체 key 집합으로 구현 가능하게 한다(프론트는 key→i18n, 미매핑 시 key fallback).
- 프론트 다이어그램은 자족 인라인 SVG 컴포넌트로 분리(`AgentStatusDiagram.tsx`)하고, 패널은
  `viewMode` 에 따라 diagram|tile 을 렌더한다. 데이터 획득은 기존 훅 그대로(추가 요청 없음).
- 요약 카운트 렌더는 두 뷰가 공유하는 하위 컴포넌트로 두어 중복을 피한다.

## 마일스톤 (우선순위 기반, 시간 예측 없음)

- Primary Goal (Priority High): 백엔드 provider 인터페이스(REQ-01) + 어댑터/DTO 전파(REQ-02) + xsfm 구현(REQ-03) + TS 미러(REQ-04). `/agents/{id}/stats` 에 `summary_stats` 노출.
- Secondary Goal (Priority High): SVG 다이어그램 뷰(REQ-05) + viewMode 선택기(REQ-06) + 두 뷰 공통 요약 표출(REQ-07).
- Final Goal (Priority Medium): i18n ko/en 정합(REQ-08) + 하위호환/비범위 회귀 확인 + go/tsc/vitest 그린(REQ-09).
- Optional Goal (Priority Low): xsfm line/station/group 카운트, 다이어그램 애니메이션/툴팁(향후 여지, 본 SPEC 범위 밖).

## 위험 및 대응

| 위험 | 영향 | 대응 |
|------|------|------|
| DTO 신규 필드가 기존 응답 파서를 깨뜨림 | 프론트/기존 소비자 회귀 | omitempty + 옵셔널 필드만 추가, flat/중첩 필드 불변(A1); 기존 테스트 그린 확인 |
| 미구현 에이전트에서 다이어그램/요약 부재로 blank | 뷰 공백 | 조건부 렌더·flat 폴백(REQ-07, §S3 graceful); `EnhancedMessagesStats` 부재 처리 선례 |
| xsfm 집계에서 로스터 lock 오용(재귀 RLock deadlock) | 백엔드 hang | `ListDevices()`/동일 RLock 규약 준수, lock 보유 중 재진입 금지(HVAC lock 트랩 교훈) |
| `messages`(external/internal) 부재 시 다이어그램 방향 불명 | 오표기 | flat `messages_in/out`+`error_count` 폴백으로 단일 흐름만 표기(§S3) |
| viewMode 기본값 미설정으로 기존 패널 뷰 변경 | DASHBOARD-002 회귀 | 미설정/미인식 → 기본 `tile` 폴백(REQ-06, 바이트 동일 동작 지향) |
| i18n 키 누락(ko/en 불일치) | 라벨 미표기 | 두 파일 동시 추가 + 키 정합 검사(REQ-08) |

## 품질 게이트

- 백엔드: `go build ./...`, `go test ./internal/agent/... ./internal/api/...` 그린; 신규 코드 커버리지 85% 목표.
- 프론트: `tsc --noEmit` 타입 에러 0, `vitest` 신규/기존 테스트 그린; 신규 코드 커버리지 85% 목표.
- 경계: 신규 엔드포인트 0, 신규 npm 의존성/차트 라이브러리 0, `exec(list_devices)` 경로 미변경.
- 코드 주석 한국어(프로젝트 `code_comments: ko`), TRUST 5 준수.
