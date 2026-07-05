---
id: SPEC-STORE-004
title: 수용 기준 — Store 복합 식별(시리즈) 모델
version: 0.2.0
status: draft
created: 2026-06-15
updated: 2026-07-05
author: xtra
related_spec: SPEC-STORE-004
---

# SPEC-STORE-004 수용 기준 (acceptance.md)

> 모든 시나리오는 Given-When-Then 형식이다. 별도 명시가 없으면 `namespace="default"`,
> `registration_type="auto"` 를 가정한다. 시리즈 식별자 = `(key, metric_type, sorted(tags))`.

## 1. 시리즈 식별 — metric_type 차이

### AC-1: 같은 key + 다른 metric_type → 별개 시리즈 (E2/N1)

- **Given** key `room` 에 `metric_type=temperature`, tags `{}` 로 값 `22` 를 쓴 상태
- **When** 같은 key `room` 에 `metric_type=humidity`, tags `{}` 로 값 `55` 를 쓴다
- **Then** 두 시리즈가 독립적으로 존재한다:
  - `(room, temperature, {})` → 현재값 `22`
  - `(room, humidity, {})` → 현재값 `55`
- **And** temperature 시리즈의 값/history 가 humidity 쓰기로 인해 변경되지 않는다 (N1).

## 2. 시리즈 식별 — tags 차이

### AC-2: 같은 key + 다른 tags → 별개 시리즈 (E3/N2)

- **Given** key `temp`, `metric_type=temperature`, tags `{room:"1"}` 로 값 `21` 을 쓴 상태
- **When** 같은 key `temp`, 같은 metric_type, tags `{room:"2"}` 로 값 `26` 을 쓴다
- **Then** 두 시리즈가 독립 존재한다: `(temp, temperature, {room:1})`=21,
  `(temp, temperature, {room:2})`=26.
- **And** `{room:1}` 시리즈가 `{room:2}` 쓰기로 덮어써지지 않는다 (N2).

### AC-3: tags 순서 무관 동일 시리즈 (U2)

- **Given** tags `{room:"1", floor:"2"}` 로 값을 쓴 시리즈
- **When** 같은 key/metric 에 tags `{floor:"2", room:"1"}`(순서만 다름) 로 값을 쓴다
- **Then** 동일 시리즈로 식별되어 **같은 시리즈가 갱신**된다 (새 시리즈가 생기지 않는다).

## 3. data_type 비식별성

### AC-4: data_type 차이는 식별에 영향 없음 (U3/N3)

- **Given** `(sensor, temperature, {})` 시리즈가 존재
- **When** 같은 `(key, metric_type, tags)` 에 data_type 이 다른 값(예: int `22` 후 float `22.5`)
  을 쓴다
- **Then** **새 시리즈가 생성되지 않고** 동일 시리즈가 갱신된다 (data_type 은 식별 차원 아님).
- **And** data_type 충돌 시 동작은 기존 SPEC-STORE-003 타입 정책(coercion/ErrTypeMismatch)을
  시리즈 내부에서 그대로 따른다.

## 4. 키 조회 — 전체 시리즈 반환

### AC-5: 키 조회 시 모든 시리즈 반환 (E4)

- **Given** key `room` 아래 `(room, temperature, {})`, `(room, humidity, {})`,
  `(room, temperature, {area:"a"})` 3개 시리즈가 존재
- **When** `POST /api/v1/store/{name}/query` 를 `key=room`, metric/tags 필터 없이 호출한다
- **Then** 3개 시리즈의 데이터가 모두 반환된다.
- **And** 각 엔트리는 소속 시리즈를 식별하는 `labels`(metric_type + tags)를 포함한다.

## 5. 필터로 단일 시리즈 선택

### AC-6: metric+tags 필터로 단일 시리즈 (E5)

- **Given** AC-5 의 3개 시리즈
- **When** `key=room`, `metric_type=temperature`, `tags={area:"a"}` 필터로 조회한다
- **Then** `(room, temperature, {area:a})` 시리즈만 반환된다.

### AC-7: metric_type 만 필터(tags 생략) → 해당 metric 전 tags 시리즈 (S3)

- **Given** AC-5 의 3개 시리즈
- **When** `key=room`, `metric_type=temperature`(tags 생략)로 조회한다
- **Then** `(room, temperature, {})` 와 `(room, temperature, {area:a})` 두 시리즈가 반환된다
  (humidity 는 제외).

### AC-8: 미일치 필터 → 200 빈 결과 (S4)

- **Given** AC-5 의 3개 시리즈
- **When** `key=room`, `metric_type=pressure`(존재하지 않음)로 조회한다
- **Then** HTTP 200 과 엔트리 0개가 반환된다 (에러가 아니다 — ErrKeyNotFound→200 정책 보존).

## 6. 시리즈 내 시계열(history)

### AC-9: 시리즈 단위 독립 history (U5/E6/A5)

- **Given** `(room, temperature, {})` 에 `20`, `21`, `22` 를 순차로 쓰고,
  `(room, humidity, {})` 에 `50`, `51` 을 순차로 쓴 상태
- **When** 각 시리즈를 `mode=last_n, count=10` 으로 조회한다
- **Then** temperature 시리즈는 `[22,21,20]`(최신순, 현재값 포함), humidity 시리즈는 `[51,50]`
  을 각각 독립적으로 반환한다.
- **And** 두 시리즈의 history 가 서로 섞이지 않는다.

## 7. 등록 정책 (manual/auto)

### AC-10: manual 모드 미등록 시리즈 쓰기 거부 (S1/N4)

- **Given** `registration_type=manual`, 정의된 시리즈는 `(room, temperature, {})` 뿐
- **When** 미정의 시리즈 `(room, humidity, {})` 로 값을 쓴다
- **Then** 쓰기가 거부된다(`ErrKeyNotAllowed` 계열).
- **And** 거부된 시리즈에 엔트리/history 흔적이 남지 않는다 (N4).

### AC-11: auto 모드 미등록 시리즈 자동 등록 (S2)

- **Given** `registration_type=auto`
- **When** 미등록 시리즈 `(new, temperature, {room:"1"})` 로 첫 값을 쓴다
- **Then** 해당 시리즈가 SourceAuto 로 자동 등록되고 값이 저장된다.

## 8. store-write 노드 다중 시리즈 (A4)

### AC-12: 한 store-write 가 여러 시리즈에 기록

- **Given** store-write 노드가 `key_mappings` 로 2개 키, 공유 `metric_type=$.payload.metric`,
  `tags={room:$.payload.room}` 설정
- **When** 메시지 `payload={metric:"temperature", room:"1", a:10, b:20}` 가 처리된다
- **Then** 각 (키,값) 이 `metric_type=temperature, tags={room:1}` 시리즈로 기록되어 총 2개
  시리즈가 갱신된다.
- **And** 같은 키에 다음 메시지가 `metric:"humidity"` 로 오면 별개 시리즈가 추가된다.

## 9. GET /keys 시리즈 행

### AC-13: keys 응답이 시리즈 행 (E7)

- **Given** `(room, temperature, {})`, `(room, humidity, {area:"a"})` 2개 시리즈
- **When** `GET /api/v1/store/{name}/keys` 를 호출한다
- **Then** 응답에 2개의 행이 있고 각 행은 `(key, metric_type, tags, data_type, registration)`
  을 포함한다.
- **And** 같은 key `room` 이 metric/tags 가 다른 2개 행으로 노출된다.

### AC-14: keys 필터 재사용

- **Given** AC-13 의 시리즈
- **When** `GET /keys?metric_type=temperature` 로 조회한다
- **Then** temperature 시리즈 행만 반환된다.

## 10. 단일 시리즈 reset (E8)

### AC-15: 단일 시리즈만 reset

- **Given** `(room, temperature, {})` 와 `(room, humidity, {})` 2개 시리즈
- **When** `(room, humidity, {})` 단일 시리즈를 reset(DELETE) 한다
- **Then** humidity 시리즈만 처리(정적: history clear / 동적: entry delete)되고
  temperature 시리즈는 영향받지 않는다.

## 11. 하위 호환 — 메타 없는 쓰기

### AC-16: 메타 미지정 쓰기 = 기본 시리즈

- **Given** 메타(metric/tags) 없이 일반 `Set(key, value)` 호출
- **When** key `simple` 에 값 `1` 을 쓴다
- **Then** `(simple, "unknown", {})` 기본 시리즈로 저장된다.
- **And** 같은 key 에 메타 없이 다시 쓰면 같은 기본 시리즈가 갱신된다 (시리즈 폭증 없음).

## 12. 동시성/안정성

### AC-17: 동시 다른 시리즈 쓰기 데이터 레이스 없음

- **Given** 같은 key 에 서로 다른 metric/tags 로 동시 다수 goroutine 쓰기
- **When** `go test -race` 로 실행한다
- **Then** 데이터 레이스/panic 없이 각 시리즈가 독립적으로 일관되게 저장된다.

## 13. 인코딩 안전성 (N5)

### AC-18: 시리즈 키 인코딩 충돌 부재

- **Given** 서로 다른 시리즈 식별자 집합(metric/tag 값에 구분자 유사 문자 포함 시도)
- **When** 각 식별자를 `EncodeSeriesKey` 로 인코딩한다
- **Then** 서로 다른 식별자는 항상 서로 다른 인코딩을 생성한다(단사성).
- **And** 동일 식별자(tags 순서만 다른 경우 포함)는 항상 동일 인코딩을 생성한다(결정성, U4/U2).

## 확장 수용 기준 (v0.2.0)

### AC-KT1 — key_tag 로 지정 태그 값을 키로 사용

- **Given** store 에이전트에 `key_tag: "name"` 설정
- **When** 쓰기 태그 `{name: "livingroom"}` 로 `SetWithMeta("dev-uuid", ...)` 실행
- **Then** 시리즈 key 는 `"livingroom"` 이 되고, 원래 key(`"dev-uuid"`)는 `id` 태그로 보존된다.

### AC-KT2 — key_tag 폴백

- **Given** `key_tag: "name"` 설정
- **When** 쓰기 태그에 `name` 이 없거나 값이 빈 문자열
- **Then** 시리즈 key 는 호출자 제공 key(생성된 id)로 유지되고 `id` 태그 주입은 없다.

### AC-RN1 — 키의 모든 시리즈를 새 키로 이동(값+히스토리+메타 보존)

- **Given** key `A` 아래 metric 이 다른 여러 시리즈(값+히스토리 존재)
- **When** `POST /store/{agent}/keys/A/rename` body `{"new_key":"B"}`
- **Then** 모든 시리즈가 key `B` 로 이동하며 값·히스토리·메타(data_type/metric/tags/source)가 보존되고, key `A` 는 사라진다. 응답 `{old_key, new_key, moved}` (200).

### AC-RN2 — 대상 키 충돌 시 전체 거부

- **Given** key `B` 에 이동 대상과 동일한 시리즈가 이미 존재
- **When** `A → B` rename 요청
- **Then** 409(`ErrKeyExists`)를 반환하고 **아무 시리즈도 이동하지 않는다**(사전 검사).
- **And** 빈 `new_key` 또는 `new_key == oldKey` 는 400, 일치 시리즈 0개는 404.

### AC-LK1 — GET /keys 데이터 없는 시리즈 제외

- **Given** 레지스트리에 실데이터 있는 시리즈 + 데이터 없는 항목(auto 유령 / bare 정적 정의) 혼재
- **When** `GET /store/{agent}/keys`
- **Then** **실데이터 있는 시리즈만** 반환된다(manual/auto 무관, 데이터 없는 항목 제외).
- **And** 라인차트 Store 선택기 목록이 저장소 탭(실데이터 기준)과 일치한다.

### AC-LK2 — 미구현 에이전트 폴백

- **Given** `LiveSeriesKeys` 를 구현하지 않는 store 에이전트(원격 등)
- **When** `GET /keys`
- **Then** 필터 없이 기존 동작(레지스트리 전체 노출)으로 폴백한다(400 없음).

## Definition of Done

- 위 모든 AC 가 자동화 테스트로 통과한다 (`go test -race ./internal/agent/system/...`,
  `./internal/node/...`, `./internal/api/handler/...`).
- 신규 코드 커버리지 85%+, 기존 경로 characterization 테스트 통과(회귀 없음).
- golangci-lint 무경고, gofmt/goimports 정합.
- 모든 변경에 `@spec SPEC-STORE-004` 태그 부착.
- `POST /query`(metric/tags 필터·labels), `GET /keys`(시리즈 행), `DELETE`(단일 시리즈) API
  동작이 문서/UI 와 정합.
- store vs tsdb 역할 구분 문서화 완료, 통합 여부 결정 사항 기록.

## 품질 게이트 기준 (Quality Gates)

- 시리즈 식별 정확성: metric/tags 차이 → 별개 시리즈, data_type 차이 → 동일 시리즈 (100% 통과).
- 조회 완전성: 키 조회 시 전 시리즈 반환, 필터 시 정확한 부분집합.
- 무손실: 한 시리즈 쓰기가 다른 시리즈를 덮어쓰지 않음 (N1/N2).
- 안정성: race 무검출, 거부 쓰기 흔적 없음 (N4).

## 검증 방법 및 도구

- 단위/통합: Go table-driven 테스트, testify/go-cmp.
- 동시성: `go test -race`.
- API: 기존 handler 테스트 하니스(`store_query_test.go` 등) 확장.
- 인코딩 단사/결정성: 속성 기반(다양한 tag 순열) 테스트.
