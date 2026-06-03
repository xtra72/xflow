# SPEC-LINK-001 구현 계획 (Plan)

> 가상(네임드) 링크 — 와이어 가상화 표시로 캔버스 연결 간소화
> PLAN 단계 산출물. 코드 미구현. 개발 방법론: Hybrid (신규=TDD, 기존 변경=동작 보존 DDD).

## 1. 기술 접근

### 핵심 원칙

- **엔진 라우팅 불변 / 백엔드 최소 변경.** `virtual` 은 표시 전용 플래그이며 와이어 라우팅 코드에는 손대지 않는다. 백엔드 변경은 (1) `Wire` 구조체 필드 추가, (2) 어댑터 통과 두 곳으로 한정한다.
- **`name` 은 기존 `Wire.Name` 재사용.** 새 필드를 만들지 않는다.
- **별도 link 노드/엔진 합성 금지.** 오직 "와이어 가상화 표시" 모델만 구현한다.

### 변경 지점 요약

1. 백엔드: `flow.Wire.Virtual bool` 추가(+ 선택 `WithVirtual` 옵션).
2. 백엔드: `flow_adapter.go convertReactFlowEdgesToWires` 에 `virtual` 통과(이미 `name` 통과 중).
3. 프론트: `editorStore.onConnect` edge 메타에 `virtual: false` 기본값 + 편집 액션.
4. 프론트: `EdgePropertyPanel` 에 virtual 토글 + name 편집 입력.
5. 프론트: `CustomEdge` 에서 `data.virtual` 시 선 숨김 + 엔드포인트 배지 렌더.
6. 프론트: 포트 옆 배지 배치(`CustomNode`/`NodeHandle`) + 이름 그룹 하이라이트(선택 상태).

## 2. 마일스톤 (우선순위 기반, 시간 추정 없음)

### Primary Goal (우선순위: 높음) — 백엔드 데이터 보존 + 동작 불변

- M1. `flow.Wire` 에 `Virtual bool`(`json:"virtual"`) 추가. 기존 `Name` 보존 확인.
- M2. `convertReactFlowEdgesToWires` 에 `virtual` 통과 추가. `name`/`mode`/`buffer_size` 정책 불변.
- M3. 저장→재로드, export→import 라운드트립에서 `virtual`/`name` 보존 검증.
- M4. 엔진 회귀: `virtual=true` 와이어가 일반 와이어와 동일하게 라우팅됨을 기존 테스트로 확인(엔진 코드 미변경).

### Secondary Goal (우선순위: 중간) — 프론트 편집 + 가상화 렌더

- M5. `editorStore.onConnect` edge 메타에 `virtual: false` 기본값. `updateEdgeData` 로 `virtual`/`name` 편집 지원.
- M6. `EdgePropertyPanel` 에 "가상 링크(virtual)" 토글 + "링크 이름(name)" 편집 입력 추가(현재 name 읽기 전용 → 편집 가능).
- M7. `CustomEdge` 렌더 분기: `data.virtual` 시 베지어 선 숨김.
- M8. 소스 포트 옆 "출력 링크 [name]", 타겟 포트 옆 "입력 링크 [name]" 배지 표시.

### Final Goal (우선순위: 중간) — 그룹 표시 + 연결성 하이라이트

- M9. 같은 플로우 내 같은 `name` 가상 와이어를 같은 링크로 표시 취급(이름 라벨만, 색상 구분 없음).
- M10. 배지 클릭 시 같은 이름 그룹/상대 엔드포인트 하이라이트.
- M11. dirty 정책 검증: 선택만으로 dirty 금지, `virtual`/`name` 편집만 dirty.

### Optional Goal (우선순위: 낮음)

- M12. `WithVirtual` WireOption 추가(백엔드 생성 편의).
- M13. 배지 합치기 규칙/하이라이트 방식 OPEN QUESTION 확정 및 문서화.

## 3. 아키텍처 설계 방향

```
[React Flow edge]  --(virtual,name,mode,buffer_size 메타)-->  [flow_adapter]  -->  [flow.Wire{Virtual,Name,...}]
        |                                                                                  |
   CustomEdge 렌더 분기                                                         엔진 라우팅(virtual 무시, 불변)
        |
  virtual? --예--> 선 숨김 + 소스/타겟 배지("출력/입력 링크 [name]") + 이름그룹 하이라이트
           --아니오--> 기존 베지어 곡선(하위 호환)
```

- 데이터 계약은 단방향 1:1(`edge.virtual ↔ wire.Virtual`, `edge.name ↔ wire.Name`).
- 표시 로직은 전적으로 프론트(CustomEdge/CustomNode)에 위치. 백엔드/엔진은 표시를 모른다.

## 4. 위험과 대응

| 위험 | 영향 | 대응 |
|------|------|------|
| 배지 레이아웃/포트 겹침 | 포트 라벨과 배지가 겹쳐 가독성 저하 | `CustomNode` 포트 row 레이아웃 재사용, 배지를 핸들 옆 라벨 또는 오버레이로 배치(OPEN Q). 최근 커밋의 포트 정렬 수정 패턴 참고. |
| 같은 포트+같은 이름 다중 와이어의 배지 중복 | 동일 배지 N개 중복 표기 | 배지 합치기 규칙 확정(와이어별 vs 그룹 1개). 기본안: 같은 (포트,이름) 조합은 배지 1개로 합침. |
| export redaction 과의 상호작용 | export 시 `virtual`/`name` 누락 또는 민감정보 노출 | export/import 라운드트립 테스트로 보존 검증. redaction 대상 필드와 분리 확인. |
| React Flow 커스텀 edge 분기 도입 영향 | 기존 비가상 엣지 렌더 회귀 | `data.virtual` 없을 때 기존 경로 100% 유지. CustomEdge 스냅샷/렌더 테스트. |
| dirty 오작동 | 선택만으로 미저장 표시(빨간점) 발생 | `onEdgesChange` 의 `select` 제외 정책 유지. 편집 액션만 `isDirty=true`. |
| 엔진 동작 변화 의심 | 회귀 우려 | 엔진 코드 미변경 + 기존 큐/카운터/브로드캐스트 테스트 통과로 불변 입증. |

## 5. 개발 방법론 (Hybrid)

- **신규(TDD)**: 가상화 렌더 분기, 배지 컴포넌트, 어댑터 `virtual` 통과 → 테스트 우선.
- **기존 변경(DDD, 동작 보존)**: `Wire` 구조체 확장, `editorStore.onConnect`, `EdgePropertyPanel`, `CustomEdge` → 변경 전 동작 캡처 후 보존 검증.
- 커버리지 목표: 신규 코드 85%+. 엔진 회귀 0건.

## 6. 검증 게이트 (TRUST 5)

- Tested: 어댑터 통과/렌더 분기/하이라이트 단위 테스트, export-import 라운드트립.
- Readable: 한국어 주석(코드 주석은 ko), 명확한 키 이름(`virtual`).
- Unified: 기존 edge 메타/패널 패턴과 일관.
- Secured: export redaction 회귀 없음.
- Trackable: SPEC-LINK-001 참조 커밋, REQ-ID 추적.
