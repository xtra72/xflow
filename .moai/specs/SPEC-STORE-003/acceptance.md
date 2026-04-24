---
id: SPEC-STORE-003
title: Store 에이전트 정적 키 정의 및 태그 메타데이터 — 수용 기준
version: 0.1.0
status: draft
created: 2026-04-24
updated: 2026-04-24
author: xtra
priority: medium
---

# SPEC-STORE-003: 수용 기준 (Acceptance Criteria)

## 시나리오 (Given-When-Then)

### Scenario 1: 정적 키 + 태그 정상 동작 (M1, M3)

**Given**
- Store 에이전트 설정에 `keys: [{key: "indoor:1:room_temp", tags: {room: "1", type: "temperature"}}]`이 정의되어 있다
- `allow_dynamic_keys: true` (기본값)
- 에이전트가 정상 부팅되었다

**When**
- 클라이언트가 `SET indoor:1:room_temp = 22.5` 쓰기 요청을 전송한다
- 이후 `GET /api/v1/agents/{id}`를 호출하여 `state.entries`를 조회한다

**Then**
- 쓰기가 성공한다 (HTTP 200 또는 프로토콜별 성공 코드)
- 응답의 `entries` 배열에 해당 키 엔트리가 존재한다
- 해당 엔트리의 `tags` 필드가 `{"room": "1", "type": "temperature"}`와 정확히 일치한다
- 엔트리의 `value`가 `22.5`로 저장되어 있다

---

### Scenario 2: 동적 키 거부 (Strict 모드, M2)

**Given**
- Store 에이전트 설정에 `allow_dynamic_keys: false`가 설정되어 있다
- `keys: [{key: "indoor:1:room_temp", tags: {room: "1", type: "temperature"}}]`만 정의되어 있다
- 에이전트가 정상 부팅되었다

**When**
- 클라이언트가 `SET outdoor:temperature = 35.0` 쓰기 요청을 전송한다 (미등록 키)

**Then**
- 쓰기가 거부된다 (`ErrKeyNotAllowed` 에러 반환)
- `state.entries`에 `outdoor:temperature` 키가 존재하지 않는다
- 히스토리에도 해당 쓰기 기록이 없다 (`GET /api/v1/store/{name}/history?key=outdoor:temperature` 응답이 빈 배열)
- 기존 정적 키 `indoor:1:room_temp`에 대한 쓰기는 정상적으로 허용된다

---

### Scenario 3: 동적 키 허용 (Permissive 모드, M2, M3)

**Given**
- Store 에이전트 설정에 `allow_dynamic_keys: true` (또는 미설정 → 기본값)
- `keys: [{key: "indoor:1:room_temp", tags: {room: "1", type: "temperature"}}]`가 정의되어 있다

**When**
- 클라이언트가 `SET outdoor:temperature = 35.0` 쓰기 요청을 전송한다 (미등록 키)
- 이후 `GET /api/v1/store/{name}/keys`를 호출한다

**Then**
- 쓰기가 성공한다
- 응답의 `keys` 배열에 `indoor:1:room_temp`와 `outdoor:temperature` 둘 다 포함된다
- 응답의 `tags` 맵에는 `indoor:1:room_temp`만 포함되고 `outdoor:temperature`는 생략된다 (또는 빈 맵)
- 정적 키의 태그(`{"room": "1", "type": "temperature"}`)는 그대로 유지된다

---

### Scenario 4: 태그 기반 키 필터링 (M4)

**Given**
- Store 에이전트 설정에 다음 정적 키가 정의되어 있다:
  - `indoor:1:room_temp` with tags `{room: "1", type: "temperature"}`
  - `indoor:2:room_temp` with tags `{room: "2", type: "temperature"}`
  - `outdoor:temperature` with tags `{location: "outside", type: "temperature"}`
- 모든 키에 값이 쓰여져 있다

**When**
- 클라이언트가 `GET /api/v1/store/{name}/keys?tag=room:1`을 호출한다

**Then**
- 응답 HTTP 상태 코드는 200이다
- 응답 JSON에서 `count == 1`이다
- `keys` 배열은 정확히 `["indoor:1:room_temp"]` 하나만 포함한다
- `tags["indoor:1:room_temp"]`는 `{"room": "1", "type": "temperature"}`이다

**When (follow-up)**
- 클라이언트가 `GET /api/v1/store/{name}/keys?tag=type:temperature&tag=room:2`를 호출한다 (AND 조건)

**Then**
- 응답 JSON에서 `count == 1`이다
- `keys` 배열은 정확히 `["indoor:2:room_temp"]`이다 (두 태그 모두 매칭되는 키만)

**When (edge case)**
- 클라이언트가 `GET /api/v1/store/{name}/keys?tag=invalidformat`을 호출한다 (콜론 없음)

**Then**
- 응답 HTTP 상태 코드는 400이다
- 응답 바디에 에러 메시지 `"invalid tag format: expected key:value"`가 포함된다

---

### Scenario 5: 설정 하위호환성 (M5)

**Given**
- 기존 Store 에이전트 설정 파일에 `keys`, `allow_dynamic_keys` 필드가 **전혀 없다**
- 다른 기존 필드(`history_ttl`, `max_history_size`, `max_key_length`, `scan_interval`)만 존재한다

**When**
- 에이전트를 시작하고, 임의의 키 `arbitrary_key = some_value`를 쓴다
- 이후 `GET /api/v1/store/{name}/keys`와 `GET /api/v1/agents/{id}`를 호출한다

**Then**
- 에이전트가 에러 없이 정상 부팅된다
- 쓰기가 성공한다 (기본값 `allow_dynamic_keys=true`로 인해 모든 키 허용)
- `GET /keys` 응답에 `arbitrary_key`가 포함된다
- `tags` 필드가 응답에 생략되거나 빈 맵이다 (정적 키 정의가 없으므로)
- `state.entries`의 해당 엔트리에 `tags` 필드가 생략되거나 빈 객체이다
- 이후 설정을 reload하여 `keys`를 추가해도 기존 저장 데이터(`arbitrary_key` 값과 히스토리)는 보존된다

---

## Edge Case Checklist

- [ ] 동일 key가 `keys` 목록에 두 번 정의 → 설정 로드 에러 (`ErrDuplicateStaticKey`)
- [ ] 태그 key에 특수문자 포함 (예: `room.1`, `room 1`, `room:sub`) → 설정 로드 에러 (`ErrInvalidTagKey`)
- [ ] `keys` 배열이 빈 배열(`[]`)로 명시 → `allow_dynamic_keys=true` 조건에서는 기존 동작과 동일
- [ ] `keys` 배열이 빈 배열(`[]`)로 명시 + `allow_dynamic_keys=false` → 모든 쓰기가 거부됨 (잠금 모드)
- [ ] 태그 value에 빈 문자열(`""`) → 허용 (key 검증만 수행)
- [ ] `?tag=key:` (value 없음) → 빈 value로 매칭 시도, 매칭되는 키가 없으면 빈 결과 반환
- [ ] `?tag=key:value:extra` (콜론 두 개 이상) → `SplitN(":", 2)`로 `value:extra`를 value로 취급
- [ ] URL 인코딩된 태그 (`?tag=room%3A1`) → 정상 디코딩 후 `room:1`로 파싱
- [ ] 대소문자 구분: `tag=Room:1`과 `tag=room:1`은 다른 태그로 취급 (case-sensitive)
- [ ] 동시성: strict 모드에서 다수의 고루틴이 미등록 키를 쓰려 해도 모두 거부되어야 함 (`go test -race` 통과)
- [ ] 설정 reload 중 진행 중인 쓰기 → 기존 구현의 locking 패턴 준수, 데이터 일관성 유지
- [ ] `/tags` 엔드포인트에서 정적 키가 하나도 없을 때 → `{"pairs": []}` 반환 (빈 배열)
- [ ] 매우 많은 정적 키(1000+) + 태그 필터링 → 성능 저하 없이 반환 (선형 탐색 허용)

---

## TRUST 5 품질 게이트

### T — Tested (테스트)

- [ ] `internal/agent/system/store.go` 커버리지 ≥ 85%
- [ ] `internal/agent/system/store_user_agent.go` 커버리지 ≥ 85%
- [ ] `internal/agent/system/store_query.go` 커버리지 ≥ 85%
- [ ] `internal/api/handlers/store_*.go` 커버리지 ≥ 85%
- [ ] DDD characterization test: 기존 설정 로드/쓰기/조회 경로 동작 보존 확인
- [ ] TDD test: 신규 필드 파싱, 거부 로직, 태그 응답, 필터 쿼리 모두 커버
- [ ] `go test -race ./...` 통과 (동시성 안전성)

### R — Readable (가독성)

- [ ] 신규 struct 필드, 함수, 에러 변수에 Go doc 코멘트 추가
- [ ] 복잡한 파싱 로직(예: 중복 검증, 태그 key 검증)에 설명 주석
- [ ] 한국어 주석 가능 (프로젝트 언어 설정에 따름, code_comments=ko)
- [ ] `gofmt`, `goimports` 준수

### U — Unified (통일성)

- [ ] 기존 에러 네이밍 패턴(`ErrXxx`) 준수
- [ ] 기존 config 파싱 패턴(`parseXxxConfig`) 준수
- [ ] HTTP 핸들러의 에러 응답 포맷 기존 convention과 일치 (`{"error": "..."}`)
- [ ] JSON 필드 네이밍 (snake_case) 기존 API와 일치

### S — Secured (보안)

- [ ] URL 쿼리 파싱 시 부적절한 입력 방어 (예: 매우 긴 tag 값, null byte)
- [ ] 설정 파일 파싱 시 타입 캐스팅 에러 안전 처리 (panic 없음)
- [ ] strict 모드 거부 시 스택 트레이스나 내부 경로 누출 없음
- [ ] Tag key 정규식으로 injection 가능성 제거 (`^[a-zA-Z0-9_-]+$`만 허용)

### T — Trackable (추적성)

- [ ] SPEC @TAG 주석: `// @SPEC:SPEC-STORE-003` 주요 신규 코드 블록에 삽입
- [ ] Conventional commit 메시지 (`feat(store): add static key definition and tag metadata [SPEC-STORE-003]`)
- [ ] CHANGELOG.md에 변경 항목 기재 (신규 config 필드, 신규 API 엔드포인트, 신규 에러)
- [ ] 설정 예시 문서(`references/design/` 또는 관련 도큐먼트)에 정적 키 및 태그 사용법 예시 추가

---

## Definition of Done (DoD)

본 SPEC이 완료되려면 다음 조건을 모두 만족해야 한다.

1. **기능 완전성**
   - [ ] 5개 EARS 모듈(M1~M5)의 모든 요구사항이 구현됨
   - [ ] 5개 Given-When-Then 시나리오가 자동화 테스트로 통과
   - [ ] Edge case 체크리스트 항목이 모두 검증됨

2. **품질 기준**
   - [ ] TRUST 5 품질 게이트 모든 항목 통과
   - [ ] `go test -race ./...` 전체 통과
   - [ ] `go vet` 및 `golangci-lint` 경고 0

3. **하위호환성**
   - [ ] 기존 설정 파일(신규 필드 없음)로 부팅 및 동작 확인
   - [ ] 기존 API 응답 소비자가 변경 없이 계속 동작 (추가 필드는 optional)

4. **문서화**
   - [ ] spec.md, plan.md, acceptance.md 3개 파일 완성
   - [ ] CHANGELOG 갱신
   - [ ] 설정 예시 및 API 문서 업데이트

5. **통합**
   - [ ] SPEC-WEB-005 v0.4.0 UI 팀과 API 계약 확인
   - [ ] 기존 SPEC-STORE-001, SPEC-STORE-002 기능 회귀 없음

6. **릴리즈 준비**
   - [ ] 변경 요약을 포함한 PR 생성
   - [ ] 코드 리뷰 1회 이상 승인
   - [ ] 병합 가능 상태 (merge-ready)
