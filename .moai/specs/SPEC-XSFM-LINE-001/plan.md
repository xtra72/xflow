# SPEC-XSFM-LINE-001 구현 계획 (plan.md)

> 연관: `spec.md` (요구사항·사양), `acceptance.md` (인수 기준). Tier M. 개발 방법론: hybrid (신규 코드 TDD / 기존 코드 DDD, `quality.yaml`).
> 버전: 0.2.0 (spec.md 정합). HISTORY: 0.2.0 — OQ-1~4 확정(RD-4~7) 반영 — M1 remove_line `ErrLineInUse`(RD-5), M2 slugify 마이그레이션(RD-6), M4 빈-라인 3-세그먼트 네이밍(RD-4), Line.Order(RD-7). 리스크 표·DoD 갱신.

## 1. 기술 접근 (Technical Approach)

### 1.1 핵심 전략 — 가산 레지스트리 레이어 (GROUP-001 재현)

SPEC-XSFM-GROUP-001 이 확립한 "**별도 레지스트리를 비침습 가산 레이어로 신설**" 패턴을 라인에 그대로 적용한다. `LineRegistry` 는 `GroupRegistry`/`StationRegistry` 를 미러한다:

- 자체 `sync.RWMutex` + 인메모리 캐시(`map[string]Line`) + `lineRegistryStore`(tmp+rename atomic write).
- `newLineRegistry(dir)` — `dir==""` 이면 인메모리 전용(단위 테스트), 아니면 `{dir}/line_registry.json` 로드/복원.
- 공개 메서드는 자체 락만 취득. 타 레지스트리 락과 중첩 금지(프로젝트 RWMutex 재귀 deadlock 트랩 — HVAC 락 패턴 교훈 참조).

### 1.2 station→line SSOT 보존

라인 레지스트리는 라인 엔티티(코드/이름/정렬)만 소유한다. station→line 매핑의 SSOT 는 `StationRegistry.ResolveLine` 로 유지하고, `line:<code>` 멤버는 항상 `DevicesByLine(code)` 파생으로 도출한다(멤버 device_id 미저장). 이로써 기존 파생 로직 무회귀.

### 1.3 코드 기반 주소 통일

그룹 id 접두사 인코딩(`group_registry.go` `groupTypeFromID`)은 이미 `custom:`/`station:`/`line:` 를 지원한다. 변경점은 커스텀 id 생성 규칙만: `customIDFor(name)` → 코드 기반. 셀렉터/fan-out(`group.go`)은 접두사 판별을 재사용하므로 로직 추가 최소화.

### 1.4 composeName 순수성 유지

`composeName` 은 순수 함수로 유지하되 시그니처에 `line` 선두 인자 추가(`composeName(line, station, place, index)`). 호출부(디바이스 등록/이름 재생성)에서 `ResolveLine(station)` 결과를 주입. 빈-라인 거동(RD-4)은 라인 코드 미해석 시 라인 세그먼트를 생략(3-세그먼트)하도록 호출부/함수에서 분기.

## 2. 우선순위 마일스톤 (의존성 순서)

> 시간 추정 없음. 의존성 순서 = 실행 순서. 각 마일스톤은 green(테스트 통과 + `-race` 클린) 상태로 커밋.

### M1 — 라인 레지스트리 신설 (Priority High · 선행)

- `line_registry.go`: `Line{Code, Name, Order}` 엔티티(Order 포함, RD-7) + `LineRegistry`(RWMutex/캐시) + `lineRegistryStore`(atomic 영속) — `group_registry.go` 미러.
- `errors.go`: `ErrLineNotFound` + `ErrLineInUse`(RD-5) 추가.
- `remove_line` 핸들러: 대상 라인 코드를 참조하는 역사가 존재하면 `ErrLineInUse` 반환(거부), 참조 없을 때만 제거 + 영속(RD-5).
- 단위 테스트: upsert/조회/영속 왕복/락(-race), remove_line 참조-존재 거부(ErrLineInUse)/참조-없음 제거.
- 의존: 없음. **선행 마일스톤**.

### M2 — 로드 마이그레이션 (Priority High · M1 의존)

- 기동 로드 훅에서 `StationRegistry` 라인 문자열 → Line 엔티티 ensure-create(멱등).
- 커스텀 그룹 `custom:<name>` → `custom:<code>` 승격(RD-6): 포맷 적합 name 은 code=name 그대로, 포맷 비적합 name 은 `slugify`(§4.6: NFC·소문자화, 한글 Revised Romanization, 비허용문자→`-`, 중복 하이픈 축약·트림, 빈 결과 시 `g<fnv1a32-hex8>` 폴백, 충돌 시 접미 번호)로 code 생성 + name 원문 표시 보존.
- `slugify` 유닛 테스트: `"2층 창고"`→`2cheung-changgo`, 충돌 시 접미 번호, 순수 비-ASCII 해시 폴백.
- 비파괴·멱등 테스트(재기동 2회 동일 상태).
- 의존: M1(라인 레지스트리 존재).

### M3 — 커스텀 그룹 코드 (Priority High · M1 병행 가능, M2 이전 정합 권장)

- `Group` 에 `Code` 반영 + id 생성 `custom:<code>`.
- `add_group{code, name, members}` 핸들러: 포맷·유일성 검증(`ErrGroupAlreadyExists`).
- `name` 표시 전용화.
- 의존: 그룹 레지스트리(GROUP-001, 존재). M2 마이그레이션과 코드 규칙 정합.

### M4 — 디바이스 네이밍 (Priority High · M1 의존)

- `composeName` 시그니처 확장 + 호출부 `ResolveLine` 주입.
- 빈-라인 분기(RD-4): 라인 코드 해석 시 4-세그먼트(`{line}:{station}:{place}:{index:03d}`), 미해석 시 라인 세그먼트 생략 3-세그먼트(`{station}:{place}:{index:03d}`, 하위호환).
- 라인 후지정 시 4-세그먼트 재계산(단, sticky 이름 보존) 검증.
- sticky-name(`nameOverridden`) 보존 검증.
- 보조 인덱스 키 불변 검증.
- 의존: M1(ResolveLine 은 기존, 라인 엔티티는 표시용).

### M5 — 노드/셀렉터 검증 (Priority Medium · M3 의존)

- `station:`/`line:`/`custom:` 코드 셀렉터 fan-out 경로 검증(로직 추가 최소, 회귀 방지 중심).
- 노드 레이어 pass-through 무변경 확인(SPEC-XSFM-GROUP-001 Module 7).
- 의존: M3(커스텀 코드), M1(라인).

### M6 — 프런트엔드 (Priority Medium · M1~M5 백엔드 의존)

- 라인 관리 UI(add_line/list_lines, Order 정렬, 빈 라인 표시).
- 그룹 탭 코드 입력 필드 + `add_group` 코드 전송.
- 디바이스 이름 표시 새 포맷(RD-4: 라인 있음 4-세그먼트 / 라인 없음 3-세그먼트).
- 기존 역사/디바이스/그룹 탭 무회귀.
- vitest 커버리지 유지.
- 의존: 백엔드 명령 API 확정(M1~M5).

## 3. 아키텍처 설계 방향

- **레이어 경계**: `LineRegistry`(엔티티) ⟂ `StationRegistry`(station→line SSOT) ⟂ `GroupRegistry`(커스텀 멤버십). 셋은 독립 락·독립 영속.
- **파생 vs 명시**: line/station 멤버십 파생, custom 멤버십 명시 저장(GROUP-001 원칙 계승).
- **주소 스킴**: `<type>:<code>` 단일 규약을 station/line/custom 에 관통 적용.
- **이름 합성**: 표시(composeName, 사람이 읽는 4-세그먼트) vs 매칭(compositeKey, 정규화 int) 목적 분리 유지.

## 4. 리스크 및 대응

| 리스크                                              | 영향  | 대응                                                                 |
| ------------------------------------------------ | --- | ------------------------------------------------------------------ |
| 레지스트리 간 락 중첩 → RWMutex 재귀 deadlock (프로젝트 알려진 트랩)   | 높음  | 호출부에서 한 번에 하나의 레지스트리 락만 보유하도록 직렬화; `-race` + 락 순서 테스트. lock-holding 함수에서 타 레지스트리 접근 금지. |
| composeName 시그니처 변경으로 호출부 누락 → 이름 회귀              | 중간  | 컴파일 타임 시그니처 강제(모든 호출부 갱신) + sticky-name/빈-라인 characterization 테스트.   |
| 마이그레이션 비멱등 → 재기동 시 중복/덮어쓰기                        | 중간  | ensure-create(존재 시 무시) + 재기동 2회 동일성 테스트(REQ-05-03).                  |
| 커스텀 id name→code 전환으로 기존 저장 그룹 유실                 | 높음  | 비파괴 마이그레이션(원본 미삭제) + RD-6 slugify 규칙(§4.6) 기반 레거시 name 처리 테스트.          |
| slugify 결과 코드 충돌 → 서로 다른 레거시 그룹이 동일 code 로 병합       | 중간  | RD-6 충돌 접미 번호(`<slug>-2`…) 유일성 보장 + 충돌 테스트.                            |
| 노드 레이어 회귀                                        | 낮음  | GROUP-001 Module 7 pass-through 무변경 가정 검증(REQ-03-04) — 코드 변경 없음 확인.  |

## 5. TRUST 5 준수

- **Tested**: xsfm 패키지 커버리지 85%+ 유지, 신규 코드 TDD, 기존 코드 characterization, `-race` 클린(REQ-07-04). 프런트 vitest.
- **Readable**: 기존 패키지 명명/주석 규약(한글 code_comments) 계승, `LineRegistry` 는 `GroupRegistry` 대칭 구조.
- **Unified**: gofmt/goimports, 기존 파일 스타일 일치.
- **Secured**: 코드 포맷 검증(입력 검증), MQTT 규약 불변, 영속 atomic write.
- **Trackable**: Conventional Commit, 커밋 subject 에 `SPEC-XSFM-LINE-001` 참조, 마일스톤 단위 커밋.

## 6. 완료 정의 (Definition of Done)

- REQ-01~07 전 요구사항 구현 및 acceptance.md 시나리오 통과.
- RD-1~7(구 OQ-1~4 확정 포함) 반영.
- xsfm `-race` 클린 + 커버리지 85%+, 프런트 vitest + `tsc` 클린.
- SPEC-XSFM-GROUP-001 그룹 동작 + 기존 station/line 파생 무회귀.
- MQTT 규약 불변 확인.
