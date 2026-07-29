# SPEC-FACILITY-DASHBOARD-001 — 구현 계획 (plan.md)

> 대상 SPEC: `SPEC-FACILITY-DASHBOARD-001` — 지하철 시설물 관리 대시보드 패널 (라인·역사·기기)
> Tier: **M** (3 files: spec.md + plan.md + acceptance.md)
> 의존: **SPEC-XSFM-001**(디바이스 계층·역사 레지스트리·fan-out) + **SPEC-DASHBOARD-001**(패널 영속화)

## Tier 판정 근거

- 영향 파일: 신규 패널 컴포넌트 3 + 집계 유틸 1 + 와이어링 4-지점(uiStore/AddPanelDialog/renderDashboardPanel/PanelSettingsDialog) + i18n 2 + 테스트 → **약 10~14 파일**, 대부분 프론트엔드.
- 신규 백엔드 없음(client-side 집계, XSFM exec 재사용) → LOC은 Tier M 범위(300~1000).
- **판정: Tier M**. 단, 라인도(line diagram) 렌더링 복잡도가 예상보다 커지거나 백엔드 집계 엔드포인트(OI-1)를 본 SPEC에 편입해야 하면 Tier L 승격을 재검토한다(현재는 M로 진행). 라인도는 order 기반 논리 배치로 스코프를 제한하여 M를 유지한다.

## 마일스톤 (우선순위 기반, 시간 예측 없음)

### M1 — 데이터/집계 소스 (Priority High, 최우선 · 기반)

- 목표: 에이전트 exec(`list_devices`/`list_stations`) 응답 → 라인/역사/기기 집계 파생.
- 산출물:
  - `web/src/lib/facilityAggregation.ts` — 로스터 + 레지스트리를 입력받아 (a) station→line 해석, (b) line/station 통계 카운트(online/offline, power on/off, fan_speed 분포, 총계), (c) 역사별 요약, (d) 라인도 배치(order 정렬), (e) 미등록 station 분리("미분류")를 순수 함수로 산출.
  - `agentService.execAgent` 재사용 래퍼(시설물 조회 헬퍼): `list_devices`/`list_stations` 취득 + 타입.
- 의존: SPEC-XSFM-001의 `list_devices`/`list_stations` 응답 스키마. 미확정 필드는 방어적 파싱.
- 완료 기준: 순수 집계 함수 단위 테스트 통과(REQ-FACDASH-001-05-01/02/03).

### M2 — 기기 패널 (Priority High)

- 목표: 단일 기기 상태 + 제어 + 응답 대기 피드백.
- 산출물: `panels/FacilityDevicePanel.tsx` (기존 `SingleDevicePanel.tsx` 재사용 검토), `deviceService.executeCommand` 연동, 성공/타임아웃 표시, 전원-풍량 의존 UX.
- 완료 기준: REQ-FACDASH-001-03-01~04. 제어 응답의 `ok`/`timeout` 렌더 검증.

### M3 — 역사 패널 (Priority High)

- 목표: 역사 통계 + 기기별 상태 목록 + 역사 일괄 제어.
- 산출물: `panels/FacilityStationPanel.tsx`, M1 집계 소비, 셀렉터 `station` fan-out(`execAgent`) + 멤버별 집계 렌더.
- 완료 기준: REQ-FACDASH-001-02-*, 06-*.

### M4 — 라인 패널 + 라인도 (Priority Medium)

- 목표: 라인도 + 역사별 요약 + 라인 통계 + 라인 일괄 제어.
- 산출물: `panels/FacilityLinePanel.tsx` + 라인도 서브컴포넌트(역사 order 배치 + 상태 배지), 셀렉터 `line` fan-out + 집계 렌더.
- 완료 기준: REQ-FACDASH-001-01-*, 06-*.

### M5 — 패널 등록/와이어링 + i18n (Priority Medium)

- 목표: 3종 패널을 기존 대시보드 패널 시스템에 편입.
- 산출물(4-지점 + i18n):
  - `stores/uiStore.ts`: `PanelType` 유니온에 3종 추가, `panelDefaultSize`/`createDefaultPanel` 케이스.
  - `AddPanelDialog.tsx`: 3종 항목(icon/label/description) + 라인/역사 대상 선택 스텝, 기기는 `needsDevice`.
  - `renderDashboardPanel.tsx`: 3종 case 디스패치.
  - `PanelSettingsDialog.tsx`: agentId/line/station/deviceId/표시옵션 편집.
  - `lib/i18n/{ko,en}.json`: 라벨/설명/컨트롤 텍스트 키.
- 완료 기준: REQ-FACDASH-001-04-*, 영속 스키마 정합(REQ-FACDASH-001-04-05).

### M6 — 통합·영속 검증 (Priority Low, 마무리)

- 목표: 3종 패널 추가→배치→저장→새로고침 복원 라운드트립, 빈/미등록 degrade, 응답 대기 UX 종합 검증.
- 산출물: 통합 테스트/시나리오, acceptance.md 시나리오 충족.
- 완료 기준: acceptance.md 전 시나리오 그린.

## 기술 접근 (Technical Approach)

### 패널 컴포넌트

- 기존 `panels/*Panel.tsx` 구조를 미러링: `PanelConfig.config`에서 대상 식별(agentId/line/station/deviceId)을 읽어 데이터 훅으로 조회 후 렌더. `AcControlPanel`/`HvacControlPanel`의 제어 UX(버튼→요청→결과)를 참조.

### 집계 소스 (결정: client-side)

- **채택**: 신규 백엔드 없이 `list_devices`+`list_stations` exec 응답을 client-side 집계(REQ-FACDASH-001-05-01, A-2). 근거: XSFM-001이 이미 로스터+레지스트리를 exec로 노출하므로 중복 백엔드 표면을 피하고, MVP 단일 에이전트 스코프에서 데이터량이 단일 row/JSON 수준.
- **대안(기각/보류)**: 백엔드 집계 엔드포인트 → 대규모 fleet 성능 이슈 관찰 시 OI-1로 승격(REQ-FACDASH-001-05-05).

### 일괄 제어 호출 + 결과 렌더

- 라인=`execAgent(agentId, {command, line, params})`, 역사=`execAgent(agentId, {command, station, params})`. 응답 body의 `results[]`(멤버별 `ok`/`error`/`timeout`)를 요약 배지 + 상세 목록으로 렌더. 개별 제어는 `executeCommand(deviceId, ...)`의 `ok`/`timeout` 렌더.

### 라인도

- 역사 레지스트리 `order`로 정렬한 역사 노드를 수평 라인 위에 배치(논리 배치, GIS 아님). 각 노드에 M1 집계의 역사 요약 배지. 실측 노선도/환승은 OI-4.

## 위험 및 대응 (Risks)

| 위험                                          | 영향                         | 대응                                                                 |
| ------------------------------------------- | -------------------------- | ------------------------------------------------------------------ |
| 라인도 레이아웃 데이터 소스 불명확(order/좌표)             | 라인도 배치 부정확                | order 기반 논리 배치로 스코프 제한(A-4/OI-4). 실좌표는 후속.                        |
| 대규모 fleet에서 client-side 집계 성능/전송량          | 라인 패널 렌더 지연               | MVP는 단일 에이전트·카운트 집계로 한정. 성능 관찰 시 백엔드 집계(OI-1)로 승격.                |
| 응답 대기 UX 피드백(비동기 에코, 타임아웃) 표현             | 사용자가 성공/실패 오인             | exec 응답의 `ok`/`timeout`/`error`를 그대로 매핑. 자체 타임아웃 로직 금지(A-6).       |
| XSFM-001 셀렉터/응답 스키마 결합(coupling)   | 상류 스키마 변경 시 패널 파손        | Traceability 표로 소비 지점 명시 + 방어적 파싱. 상류 REQ ID 고정 참조.               |
| XSFM-001 미구현/부분 구현 상태에서 개발 착수      | 데이터/제어 표면 부재             | 안전 degrade(A-7) 우선 구현. 실표면 결합은 상류 완료 후 통합(depends_on 게이트).        |
| 영속 스키마 오염(신규 최상위 필드)                       | SPEC-DASHBOARD-001 백워드 파손 | 확장은 `config` 내부 키로 한정(UB-003, REQ-FACDASH-001-04-05).             |

## 소비 SPEC 게이트

- 본 SPEC의 run-phase는 `depends_on: SPEC-XSFM-001` 충족(상류 `status: completed`)을 전제로 한다. 상류 미완 시 실표면 통합(M2~M6의 exec 결합)은 안전 degrade 스텁으로 진행하고, 실결합은 상류 완료 후 수행한다.
