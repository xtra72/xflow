# SPEC-XSFM-LINE-001 인수 기준 (acceptance.md)

> Given-When-Then 형식. 기계 검증 가능(Go `testing`+`testify` / 프런트 `vitest`). 각 시나리오는 요구사항(REQ) 및 마일스톤(M)에 매핑.
> 버전: 0.4.0 (spec.md 정합). 상태: **completed** (2026-07-31, amendment 후 re-completed). HISTORY: 0.4.0 — `{line_code}` 토픽 placeholder 정식 amendment(in-place, `amendment_of: SPEC-XSFM-LINE-001`, 직전 완료 0.3.1 `e23a93b5`). §9(AC-9.1~9.6) 추가 — outbound 파생 렌더/빈-라인 빈 세그먼트/inbound 무시(식별 불변)/기존 placeholder 무회귀. 기존 §1~§8 무회귀. 상세는 spec.md §4.7·Module 8 참조. / 0.3.1 — 역번호(station_number) 후속 노트(`a35c468b`, 원 EARS 범위 밖 직접 후속, 기존 AC 무회귀). 상세는 spec.md §7.6 참조. / 0.3.0 — M1~M6 구현 완료로 §1~§8 전 시나리오 통과(백엔드 `ef4f28a7`, 프런트 `e8678033`). xsfm 커버리지 89.1% · `-race` 클린 · `go test ./...` exit 0, 프런트 vitest 2275 pass · `tsc` 클린. as-implemented 분기 5건은 spec.md §7 참조(분기 1 add_group code optional / 분기 2 station 코드 포맷 미강제 / 분기 5 디바이스 이름 프런트 무변경). / 0.2.0 — OQ-1~4 확정(RD-4~7) 반영으로 구 OQ-의존 시나리오를 확정 시나리오로 전환/추가 — AC-1.8(ErrLineInUse), AC-1.9/1.10, AC-3.3(포맷 거부), AC-5.4/5.6/5.7(빈-라인 3-세그먼트·후지정 재계산·sticky 보존), AC-6.2/6.2a/6.2b(slugify 마이그레이션·충돌).

## §1. 라인 레지스트리 — add_line / list_lines (Module 1, M1)

### AC-1.1 add_line 생성 + list_lines 반영
- **Given** 빈 라인 레지스트리
- **When** `add_line{code: "line_2", name: "2호선"}` 을 처리한다
- **Then** `list_lines` 는 `line_2`(name "2호선") 1건을 반환한다 (REQ-01-03, REQ-01-06)
- **검증**: Go 단위 테스트 — 반환 목록 길이 1, code/name 일치

### AC-1.2 add_line upsert (표시명 갱신)
- **Given** `line_2`(name "2호선") 이 등록됨
- **When** `add_line{code: "line_2", name: "2호선(순환)"}` 을 재처리한다
- **Then** 라인 수는 1개로 유지되고 name 이 "2호선(순환)" 으로 갱신된다 (REQ-01-04, upsert 의미)

### AC-1.3 list_lines Order 오름차순
- **Given** `add_line{code:"line_2", name:"2호선", order:2}`, `add_line{code:"line_1", name:"1호선", order:1}` 등록
- **When** `list_lines` 를 처리한다
- **Then** 결과는 `[line_1, line_2]` (Order 오름차순) 순으로 반환된다 (REQ-01-06, REQ-01-02)

### AC-1.4 영속 왕복 (write-through)
- **Given** 영속 dir 로 구성된 라인 레지스트리에 `line_1` 등록
- **When** 레지스트리를 새 인스턴스로 재로드한다
- **Then** `line_1` 이 캐시에 복원된다 (REQ-01-02, REQ-07-02)
- **검증**: `{dir}/line_registry.json` 존재 + 재로드 후 조회 성공

### AC-1.5 빈 라인 유효성
- **Given** `line_9`(참조 역사 0개) 등록
- **When** `line:line_9` 멤버를 조회한다
- **Then** 빈 device_id 집합을 반환한다(에러 아님), 라인 엔티티는 유효 (REQ-01-07, A-5)

### AC-1.6 락 동시성 (-race)
- **Given** 라인 레지스트리
- **When** 다수 goroutine 이 동시에 add_line/list_lines 를 호출한다
- **Then** `go test -race` 가 클린이며 데이터 경쟁이 없다 (REQ-07-01, REQ-07-04)

### AC-1.7 remove_line (참조 없음)
- **Given** `line_9`(참조 역사 0개) 등록
- **When** `remove_line{code:"line_9"}` 을 처리한다
- **Then** `list_lines` 에서 `line_9` 가 사라지고 영속에 반영된다 (REQ-01-05)

### AC-1.8 remove_line (참조 존재) → ErrLineInUse (RD-5)
- **Given** `line_2` 를 참조하는 역사가 1개 이상 존재
- **When** `remove_line{code:"line_2"}` 을 처리한다
- **Then** `ErrLineInUse` 를 반환하고 라인 `line_2` 는 유지되며 영속에서 삭제되지 않는다 (REQ-01-05a, RD-5)
- **검증**: Go 단위 테스트 — 반환 에러가 `ErrLineInUse` 와 일치, `list_lines` 에 `line_2` 잔존

### AC-1.9 remove_line (참조 제거 후 삭제 가능) (RD-5)
- **Given** `line_2` 를 참조하던 역사를 모두 다른 라인으로 재배치하여 참조 0개
- **When** `remove_line{code:"line_2"}` 을 처리한다
- **Then** `line_2` 가 제거되고 영속에 반영된다(에러 없음) (REQ-01-05, RD-5)

### AC-1.10 add_line 코드 포맷 위반 거부 (RD-6)
- **Given** 빈 라인 레지스트리
- **When** 포맷 비적합 코드(예: `"2 호선"` 공백 포함, 또는 `"Line2"` 대문자)로 `add_line` 을 처리한다
- **Then** 코드 포맷(`^[a-z0-9][a-z0-9_-]*$`) 검증 에러를 반환하고 라인이 생성되지 않는다 (REQ-01-03, RD-6)

## §2. line:<code> 파생 멤버십 (Module 1, M4)

### AC-2.1 line:<code> 멤버 = 참조 역사 디바이스 파생
- **Given** `station:st01`(line=`line_2`) 에 디바이스 D1, D2 존재
- **When** `line:line_2` 멤버를 조회한다(`DevicesByLine`)
- **Then** D1, D2 가 포함된다 (REQ-01-08, RD-1)

### AC-2.2 라인 레지스트리는 멤버 미저장
- **Given** `line_2` 등록 + 참조 역사 디바이스 존재
- **When** `line_registry.json` 을 검사한다
- **Then** device_id 목록이 저장되어 있지 않다(엔티티 코드/이름/정렬만) (REQ-01-08)

## §3. 커스텀 그룹 코드 (Module 2, M3)

### AC-3.1 add_group 코드 기반 id
- **Given** 빈 그룹 레지스트리
- **When** `add_group{code:"gpump", name:"펌프군", members:[D1,D2]}` 을 처리한다
- **Then** 그룹 id 는 `custom:gpump` 이고 name 은 "펌프군"(표시 전용) 이다 (REQ-02-01, REQ-02-02)

### AC-3.2 코드 중복 거부
- **Given** `custom:gpump` 존재
- **When** `add_group{code:"gpump", ...}` 을 재처리한다
- **Then** `ErrGroupAlreadyExists` 를 반환한다 (REQ-02-04)

### AC-3.3 코드 포맷 위반 거부 (RD-6)
- **Given** 빈 그룹 레지스트리
- **When** 포맷 비적합 코드 `"펌프 군"`(공백·한글 포함, `^[a-z0-9][a-z0-9_-]*$` 위반)로 `add_group` 을 처리한다
- **Then** 코드 포맷 검증 에러를 반환하고 그룹이 생성되지 않는다 (REQ-02-05, RD-6)
- **검증**: Go 단위 테스트 — 포맷 정규식 위반 코드 집합(`"펌프 군"`, `"Pumps"`, `"_pump"`, `""`)이 모두 거부됨

## §4. 코드 기반 통일 주소 셀렉터 (Module 3, M5)

### AC-4.1 line:<code> 셀렉터 fan-out
- **Given** `line_2` 참조 역사에 디바이스 D1, D2
- **When** group_id=`line:line_2` 셀렉터로 제어를 요청한다
- **Then** D1, D2 로 fan-out 되고 기존 `fanOutControl` 경로를 재사용한다 (REQ-03-02)

### AC-4.2 custom:<code> 셀렉터 fan-out
- **Given** `custom:gpump`(members D1) 존재
- **When** group_id=`custom:gpump` 셀렉터로 제어를 요청한다
- **Then** D1 로 fan-out 된다 (REQ-03-03)

### AC-4.3 station:<code> 병존 무회귀
- **Given** `station:st01` 에 디바이스 D1
- **When** group_id=`station:st01` 셀렉터로 제어를 요청한다
- **Then** D1 로 fan-out 되어 기존 결과와 동일하다 (REQ-03-01, REQ-07-03)

### AC-4.4 노드 pass-through 무변경
- **Given** 노드 레이어(xsfm-control / 통합 xsfm 노드, GROUP-001 Module 7)
- **When** `line:line_2` / `custom:gpump` group_id 를 flow 메시지로 전달한다
- **Then** 노드 코드 변경 없이 접두사+코드 문자열이 에이전트로 그대로 전달되고 에이전트가 접두사 판별한다 (REQ-03-04, A-3)
- **검증**: 노드 스키마/매핑 코드 무변경 + 에이전트 해석 통합 테스트

## §5. 디바이스 네이밍 (Module 4, M4)

### AC-5.1 새 4-세그먼트 자동 이름
- **Given** `station:st01`(line=`line_2`), place=`pump`, index=3 인 신규 디바이스
- **When** 자동 이름을 합성한다(`composeName`)
- **Then** 이름은 `line_2:st01:pump:003` 이다 (REQ-04-01, REQ-04-04)

### AC-5.2 index 3자리 0-채움
- **Given** index=7
- **When** 이름을 합성한다
- **Then** 세그먼트가 `007` 로 0-채움된다 (REQ-04-04)

### AC-5.3 sticky-name 보존
- **Given** `nameOverridden=true` 인 디바이스(사용자 지정 이름 "메인펌프")
- **When** 이름 재생성이 트리거된다
- **Then** "메인펌프" 가 보존되고 새 포맷이 적용되지 않는다 (REQ-04-02, A-4)

### AC-5.4 빈-라인 자동 이름 3-세그먼트 (RD-4)
- **Given** 라인 코드가 없는 역사 `st99`(`ResolveLine(st99)=""`)의 디바이스(place=`pump`, index=3)
- **When** 자동 이름을 합성한다(`composeName`)
- **Then** 라인 세그먼트를 생략해 `st99:pump:003`(3-세그먼트, 하위 호환·무회귀) 이다 (REQ-04-03, RD-4)
- **검증**: Go 단위 테스트 — 4-세그먼트(AC-5.1 `line_2:st01:pump:003`) vs 3-세그먼트(빈-라인) 대비

### AC-5.6 라인 후지정 시 4-세그먼트 재계산 (RD-4)
- **Given** 라인 코드 없이 생성되어 자동 이름 `st99:pump:003` 을 가진 디바이스(sticky 아님)
- **When** 역사 `st99` 에 라인 코드 `line_9` 를 지정한 뒤 자동 이름 재계산이 트리거된다
- **Then** 자동 이름이 `line_9:st99:pump:003`(4-세그먼트)으로 재계산된다 (REQ-04-03a, RD-4)

### AC-5.7 라인 후지정에도 sticky 이름 보존 (RD-4)
- **Given** `nameOverridden=true`("메인펌프") 디바이스가 라인 없는 역사에 존재
- **When** 역사에 라인 코드가 지정되어 재계산이 트리거된다
- **Then** "메인펌프" 가 보존되고 4-세그먼트 재계산이 적용되지 않는다 (REQ-04-02, REQ-04-03a, RD-4)

### AC-5.5 보조 인덱스 키 불변
- **Given** 디바이스 이름 규칙 변경 적용 후
- **When** `compositeKey`(정규화 int 매칭 키)를 계산한다
- **Then** 매칭 키는 변경되지 않는다(표시 vs 매칭 분리) (REQ-04-05)

## §6. 마이그레이션 (Module 5, M2)

### AC-6.1 station.Line → Line 엔티티 ensure-create
- **Given** 기존 영속 데이터에 `station.Line = "line_2"` 인 역사 존재, 라인 레지스트리에 `line_2` 없음
- **When** 에이전트가 로드 마이그레이션을 수행한다
- **Then** `line_2` Line 엔티티가 생성되고(code="line_2", name="line_2") `list_lines` 에 나타난다 (REQ-05-01)

### AC-6.2 custom:<name> → custom:<code> 승격 (포맷 적합, RD-6)
- **Given** 기존 영속 그룹에 id=`custom:pumps`(포맷 적합) 존재
- **When** 로드 마이그레이션을 수행한다
- **Then** id 가 `custom:pumps`(code="pumps") 로 유지/승격되고 멤버가 보존된다 (REQ-05-02, RD-6)

### AC-6.2a 레거시 포맷 비적합 name slugify 마이그레이션 (RD-6)
- **Given** 기존 영속 그룹에 id=`custom:2층 창고`(포맷 비적합: 숫자+한글+공백) 존재, 멤버 [D1, D2]
- **When** 로드 마이그레이션을 수행한다(§4.6 slugify)
- **Then** id 가 `custom:2cheung-changgo`(code="2cheung-changgo") 로 승격되고, name 표시값은 원문 `"2층 창고"` 로 보존되며, 멤버 [D1, D2] 가 유지된다 (REQ-05-02, RD-6)
- **검증**: Go 단위 테스트 — slugify(`"2층 창고"`)==`"2cheung-changgo"`, 마이그레이션 후 그룹 조회로 id/name/members 확인

### AC-6.2b slugify 코드 충돌 접미 번호 (RD-6)
- **Given** slugify 결과가 이미 존재하는 코드(예: `2cheung-changgo`)와 충돌하는 레거시 그룹이 추가로 존재
- **When** 로드 마이그레이션을 수행한다
- **Then** 두 번째 그룹의 code 에 접미 번호가 부여되어 `2cheung-changgo-2` 로 유일성이 보장된다 (REQ-05-02a, RD-6)

### AC-6.3 멱등성 (재기동 2회 동일)
- **Given** 마이그레이션이 1회 수행된 상태
- **When** 에이전트를 재기동하여 재로드한다
- **Then** 라인/그룹 상태가 동일하고 중복 생성이 없다 (REQ-05-03)

### AC-6.4 비파괴 (원본 미삭제, 무회귀)
- **Given** 마이그레이션 수행
- **When** 기존 station/line 파생 및 그룹 조회를 수행한다
- **Then** 기존 동작이 회귀 없이 유지된다 (REQ-05-04, REQ-07-03)

## §7. 프런트엔드 (Module 6, M6)

### AC-7.1 라인 관리 UI
- **Given** 라인 관리 탭
- **When** 사용자가 라인을 추가/조회한다
- **Then** `add_line`/`list_lines` 를 통해 라인이 관리되고 Order 정렬·빈 라인 표시가 반영된다 (REQ-06-01, REQ-06-04)
- **검증**: vitest — 컴포넌트가 add_line/list_lines 페이로드를 생성

### AC-7.2 그룹 코드 입력
- **Given** 그룹 생성 UI
- **When** 사용자가 코드 + 이름 + 멤버로 그룹을 생성한다
- **Then** `add_group{code, name, members}` 페이로드가 전송된다 (REQ-06-02)

### AC-7.3 디바이스 이름 표시 새 포맷
- **Given** 디바이스 목록
- **When** 이름을 렌더한다
- **Then** 라인 있음 디바이스는 `{line}:{station}:{place}:{index:03d}`(4-세그먼트), 라인 없음 디바이스는 `{station}:{place}:{index:03d}`(3-세그먼트, RD-4)로 표시된다 (REQ-06-03)

### AC-7.4 기존 탭 무회귀
- **Given** 역사/디바이스/그룹(GROUP-001) 탭
- **When** 기존 기능을 사용한다
- **Then** vitest 전량 통과 + `tsc` 클린, 회귀 없음 (REQ-06-05, REQ-07-03)

## §8. 품질 게이트 (Module 7)

### AC-8.1 커버리지 & race
- **Then** xsfm 패키지 커버리지 85%+ 및 `go test -race ./internal/agent/xsfm/...` 클린 (REQ-07-04)

### AC-8.2 MQTT 규약 불변
- **Then** MQTT 토픽/페이로드 형식 관련 테스트가 무변경으로 통과한다 (REQ-07-05)

## §9. `{line_code}` 토픽 placeholder (Module 8, M9 · amendment 0.4.0)

> amendment(0.4.0)로 추가된 시나리오. direct-mode 토픽 템플릿 한정. 기계 검증 가능(Go `testing`+`testify`).

### AC-9.1 placeholder 등록
- **Given** direct-mode 토픽 placeholder 집합
- **When** placeholder 상수 집합을 검사한다
- **Then** `line_code` 가 기존 placeholder(`station_code`/`place_code`/`device_index`/`device_id`/`attribute`)와 함께 등록되어 있다 (REQ-08-01)
- **검증**: Go 단위 테스트 — placeholder 집합에 `line_code` 존재, 기존 placeholder 전량 잔존

### AC-9.2 outbound 파생 렌더 (라인 있음)
- **Given** device.Station=`st01`, `ResolveLine(st01)="line_2"`, 토픽 템플릿 `cmd/{line_code}/{station_code}/{place_code}/bse9000/{device_index}`, device_index=3
- **When** outbound(command/pub) 토픽을 렌더한다
- **Then** 렌더 결과는 `cmd/line_2/st01/pump/bse9000/003` 이다(`{line_code}` ← 파생 라인) (REQ-08-02)
- **검증**: Go 단위 테스트 — 렌더 문자열이 파생 라인 코드로 치환됨

### AC-9.3 빈 라인 → 빈 세그먼트 (라인 없음)
- **Given** device.Station=`st99`, `ResolveLine(st99)=""`(라인 미배정), 동일 토픽 템플릿, device_index=3
- **When** outbound 토픽을 렌더한다
- **Then** `{line_code}` 가 빈 문자열로 렌더되어 `cmd//st99/pump/bse9000/003`(빈 세그먼트 유지) 이다(에러 아님) (REQ-08-04)
- **검증**: Go 단위 테스트 — 라인 세그먼트가 빈 문자열, 나머지 세그먼트 정상

### AC-9.4 lineHint 락 규율 (roster 락 밖 해석, -race)
- **Given** roster 와 station-registry 를 동시 접근하는 다수 goroutine
- **When** outbound 렌더가 `{line_code}` 파생 해석을 수행한다
- **Then** `go test -race` 가 클린이며, 라인 해석은 roster 락 밖(lineHint)에서 이루어져 레지스트리 락 중첩/재진입 deadlock 이 없다 (REQ-08-03, REQ-07-01)
- **검증**: Go 단위 테스트 — `-race` 클린 + lineHint 선해석 경로 확인(roster 락 내 station-registry 락 미취득)

### AC-9.5 inbound `{line_code}` 추출·식별 무시
- **Given** state(sub) 템플릿에 `{line_code}` 포함, inbound 메시지가 `state/line_2/st01/pump/bse9000/003` 로 도착(단, station `st01` 의 파생 라인은 `line_9` 로 불일치)
- **When** inbound 토픽을 파싱한다
- **Then** `{line_code}` 세그먼트는 추출되지만 디바이스 식별에는 무시되고, 디바이스는 `station_code`(st01)+`place_code`(pump)+`device_index`(003)로 식별되며 라인은 station 파생(`line_9`)으로 결정된다(`{line_code}` 는 Device 필드에 매핑되지 않음) (REQ-08-05)
- **검증**: Go 단위 테스트 — 토픽의 `{line_code}` 값과 무관하게 동일 디바이스로 식별, `applyAddressFields` 가 `line_code` 를 어떤 필드에도 세팅하지 않음

### AC-9.6 기존 placeholder 렌더/파싱 무회귀
- **Given** `line_code` placeholder 도입 후
- **When** 기존 placeholder(`station_code`/`place_code`/`device_index`/`device_id`/`attribute`)만 사용하는 토픽 템플릿을 렌더/파싱한다
- **Then** 렌더/파싱 결과가 amendment 이전과 동일하며, port mode·노드 레이어가 변경되지 않는다 (REQ-08-06, REQ-07-05)
- **검증**: Go 단위 테스트 — 기존 토픽 렌더/파싱 characterization 무변경, 노드 스키마/매핑 무변경

## Definition of Done (요약)

- §1~§8 전 시나리오 통과(RD-1~7, 구 OQ-1~4 확정 포함 반영).
- §9 전 시나리오 통과(RD-8, amendment 0.4.0 — `{line_code}` 토픽 placeholder).
- xsfm `-race` 클린 + 커버리지 85%+, 프런트 vitest + `tsc` 클린.
- SPEC-XSFM-GROUP-001 + 기존 station/line 파생 무회귀.
