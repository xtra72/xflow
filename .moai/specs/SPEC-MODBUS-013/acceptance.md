# SPEC-MODBUS-013 수용 기준 (acceptance.md)

Given/When/Then 형식. 백엔드 AC 는 `go test`, 프론트 AC 는 `vitest`, UI AC 는 수동 검증한다.

---

## REQ-MODBUS-013-01 — 레지스터 그룹 사용 여부

### AC-01 — 기본값은 사용 (하위 호환)
- **Given** `enabled` 키가 없는 기존 `register_groups` 설정
- **When** `parseRegisterGroupConfig` 로 파싱하면
- **Then** `Enabled == nil` 이고 `IsEnabled() == true` 이다.
- **And** 폴링 시 해당 그룹이 병합 이전과 동일하게 읽힌다.

### AC-02 — 미사용 그룹은 읽지 않음
- **Given** 3개 그룹 중 2번째가 `enabled: false` 인 디바이스
- **When** 폴링 1주기가 경과하면
- **Then** 2번째 그룹에 대한 `ReadRegisters` 호출이 0회이고, 캐시 항목과 방출 메시지가 생성되지 않는다.
- **And** 1·3번째 그룹은 정상 동작한다.
- **And** 설정에서 2번째 그룹의 정의(주소·개수·타입·설명)는 그대로 보존된다.

### AC-03 — 전체 미사용 디바이스는 무동작
- **Given** 모든 그룹이 `enabled: false` 인 디바이스
- **When** 폴링 1주기가 경과하면
- **Then** 해당 디바이스에 대한 물리 읽기가 0회이고, 오류 로그·에러 통계가 증가하지 않는다.

---

## REQ-MODBUS-013-02 — 블록 병합 읽기

### AC-04 — 연속 그룹 병합
- **Given** fc=4, 폴링주기 동일, 주소 `4(2) / 6(2) / 8(2)` 인 3개 그룹, `max_block_registers=32`
- **When** `buildReadPlan` 을 호출하면
- **Then** `ReadBlock{FunctionCode:4, StartAddress:4, Quantity:6, Members:[3개]}` 1개가 산출된다.
- **And** 폴링 시 물리 읽기가 3회가 아닌 **1회** 발생한다.

### AC-05 — 간극이 있으면 병합 안 함
- **Given** fc=4, 주소 `4(2) / 8(2)` (주소 6-7 미정의)
- **When** `buildReadPlan` 을 호출하면
- **Then** 블록 2개가 산출된다(`start=4,qty=2` / `start=8,qty=2`).

### AC-06 — fc·폴링주기가 다르면 병합 안 함
- **Given** 주소는 연속이나 `fc` 가 3과 4로 다른 두 그룹, 그리고 `fc` 는 같으나 `poll_interval` 이 `2s`/`300s` 로 다른 두 그룹
- **When** `buildReadPlan` 을 호출하면
- **Then** 각각 별도 블록으로 분리된다(총 4블록).

### AC-07 — 미사용 그룹은 블록에서 제외되고 경계를 나눔
- **Given** fc=4, 주소 `4(2) / 6(2) / 8(2)` 중 가운데 `6(2)` 가 `enabled: false`
- **When** `buildReadPlan` 을 호출하면
- **Then** 블록 2개(`start=4,qty=2` / `start=8,qty=2`)가 산출되고, 주소 6-7 은 어떤 블록에도 포함되지 않는다.

### AC-08 — 블록 결과 분배가 그룹 단위 계약을 보존
- **Given** AC-04 의 병합 블록 1개와 `interval` 모드
- **When** 블록 읽기가 성공하면
- **Then** 멤버 3개 각각에 대해 `cache.UpdateFromRead(fc, 멤버주소, 멤버슬라이스, 멤버quantity)` 가 1회씩 호출된다.
- **And** 방출 메시지 3건의 `group`·`quantity`·`start_address` 가 병합 이전과 바이트 동일하다.
- **And** 각 멤버 슬라이스의 바이트 오프셋은 `(멤버주소 - 블록시작주소) * 2` 이다.

### AC-09 — 블록 실패는 멤버 전체 실패
- **Given** 멤버 3개인 블록
- **When** 물리 읽기가 오류를 반환하면
- **Then** 멤버 3개 모두 실패로 기록되고(`recordRequestStat` success=false 3회), 캐시 갱신·메시지 방출이 발생하지 않는다.

---

## REQ-MODBUS-013-03 — 블록 최대 레지스터 수

### AC-10 — 기본값 32
- **Given** `max_block_registers` 를 지정하지 않은 설정
- **When** 파싱하면
- **Then** `ModbusConfig.MaxBlockRegisters == 32` 이다.

### AC-11 — 상한 초과 시 분할
- **Given** fc=4, 주소 `0` 부터 1워드 그룹 40개(연속), `max_block_registers=32`
- **When** `buildReadPlan` 을 호출하면
- **Then** 블록 2개(`qty=32` / `qty=8`)가 산출되고 모든 블록의 `Quantity <= 32` 이다.

### AC-12 — 디바이스 오버라이드 상속
- **Given** 에이전트 `max_block_registers: 32`, 디바이스 A 는 미지정, 디바이스 B 는 `56`
- **When** 각 디바이스의 읽기 계획을 세우면
- **Then** A 는 32, B 는 56 을 상한으로 사용한다.

### AC-13 — 범위 클램프
- **Given** `max_block_registers: 0` 과 `max_block_registers: 500`
- **When** 파싱하면
- **Then** 각각 `1` 과 `125` 로 클램프된다.

### AC-14 — 단일 그룹이 상한보다 크면 그대로 통과
- **Given** `quantity: 50` 인 단일 그룹, `max_block_registers=32`
- **When** `buildReadPlan` 을 호출하면
- **Then** `Quantity:50` 인 단독 블록 1개가 산출된다(분할하지 않음).

---

## REQ-MODBUS-013-04 — 디바이스 모델 카탈로그

### AC-15 — 디렉터리 스캔
- **Given** `~/.xflow/models/` 에 유효한 모델 JSON 2개
- **When** `list_models` exec 을 호출하면
- **Then** 모델 2개의 `id`·`name`·`vendor`·`description`·레지스터 수가 반환된다.

### AC-16 — 무효 파일 fail-open
- **Given** 유효 파일 2개 + JSON 파싱 실패 파일 1개 + `fc:9` 인 무효 파일 1개
- **When** `list_models` 를 호출하면
- **Then** 유효 모델 2개만 반환되고, 무효 파일 2건은 경고 로그가 남으며, 호출은 성공한다.

### AC-17 — 디렉터리 부재
- **Given** `~/.xflow/models/` 가 존재하지 않음
- **When** `list_models` 를 호출하면
- **Then** 빈 배열이 반환되고 오류가 발생하지 않는다.

### AC-18 — id 중복
- **Given** 동일 `id` 를 가진 모델 파일 2개
- **When** 로드하면
- **Then** 먼저 로드된 1개만 카탈로그에 남고 나머지는 경고 로그 후 스킵된다.

### AC-19 — GIPAM 샘플 동봉
- **Given** 저장소에 동봉된 `assets/models/gipam-115fi.json`
- **When** 그 디렉터리를 카탈로그로 로드하면
- **Then** `gipam-115fi` 모델이 검증을 통과하고, fc=4 레지스터 25건·fc=3 레지스터 47건(합계 72건)을 포함하며, 워드 합계가 입력 46 / 보유 50 이고 `max_block_registers` 가 56 이다.
- **And** 이 모델을 레지스터별 그룹으로 등록했을 때 읽기 계획이 20회 미만의 물리 읽기로 합쳐진다(실측 11회).

---

## REQ-MODBUS-013-05 — 모델 선택 UI

### AC-20 — 모델 선택 시 행 채우기
- **Given** 그룹이 비어 있는 신규 디바이스 편집 화면
- **When** 모델 `GIPAM-115FI` 를 선택하면
- **Then** 레지스터 그룹 행 72개가 채워지고, `max_block_registers` 가 56 으로 설정된다.
- **And** 저장은 자동으로 수행되지 않는다.

### AC-21 — 기존 그룹 덮어쓰기 확인
- **Given** 이미 그룹이 3개 있는 디바이스 편집 화면
- **When** 모델을 선택하면
- **Then** 덮어쓰기 확인 다이얼로그가 표시되고, 취소 시 기존 3개 그룹이 유지된다.

---

## REQ-MODBUS-013-06 — 일괄등록 포맷 확장

### AC-22 — 7열 파싱
- **Given** 텍스트
  ```
  4,4,2,float32,2s,상전압 R상 [V],1
  4,32,2,float32,2s,총 역률,0
  ```
- **When** `parseBulkGroups(text)` 를 호출하면
- **Then** 그룹 2개가 산출되고 `enabled` 가 각각 `true`·`false` 이며 `errors` 는 비어 있다.

### AC-23 — 5·6열 하위 호환
- **Given** 기존 6열 텍스트 `3,0,10,uint16,5s,온도` 와 5열 텍스트 `1,0,8,uint16,`
- **When** 파싱하면
- **Then** 각각 유효 그룹으로 산출되고 `enabled == true` 이며, 나머지 필드는 기존 테스트 기대값과 동일하다.

### AC-24 — 사용 열 값 허용 범위
- **Given** `사용` 열 값이 `1/0/true/false/y/n/on/off/TRUE/Off/빈값` 인 행들
- **When** 파싱하면
- **Then** 모두 유효하게 해석되고(빈값=true), 대소문자를 구분하지 않는다.

### AC-25 — 무효 사용 값
- **Given** `4,4,2,float32,2s,설명,maybe`
- **When** 파싱하면
- **Then** 해당 행이 `invalidEnabled` 코드와 원본 줄 번호로 `errors` 에 집계되고 그룹으로 산출되지 않는다.

---

## REQ-MODBUS-013-07 — 품질·하위 호환

### AC-26 — 기존 스위트 무회귀
- **When** `go test ./internal/agent/modbus/... ./internal/modbus/...` 와 `npm test -- modbus` 를 실행하면
- **Then** 신규 테스트 포함 전량 통과하고 실패 0건이다.

### AC-27 — 커버리지
- **When** `go test -coverprofile` 로 `internal/agent/modbus` 커버리지를 측정하면
- **Then** 신규 파일(`readplan.go`, `models.go`)의 커버리지가 85% 이상이다.

### AC-28 — 설정 무변경 시 동작 동일
- **Given** `enabled`·`max_block_registers`·모델을 사용하지 않는 기존 설정
- **When** 폴링하면
- **Then** 방출 메시지의 `group`·`start_address`·`quantity`·타입 해석 결과가 변경 이전과 동일하다(특성 테스트로 고정).

---

## Definition of Done

- [ ] AC-01 ~ AC-28 전량 통과
- [ ] `go build ./...` · `go vet ./...` 성공
- [ ] `golangci-lint run` 신규 경고 0건
- [ ] `npm run build` · `npm run lint` 성공
- [ ] i18n ko/en 키 정합 (누락 0건)
- [ ] `assets/models/gipam-115fi.json` 스키마 검증 통과
- [ ] CHANGELOG 항목 추가
