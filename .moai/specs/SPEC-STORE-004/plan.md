---
id: SPEC-STORE-004
title: 구현 계획 — Store 복합 식별(시리즈) 모델
version: 0.1.0
status: draft
created: 2026-06-15
updated: 2026-06-15
author: xtra
related_spec: SPEC-STORE-004
---

# SPEC-STORE-004 구현 계획 (plan.md)

> 본 계획은 `@spec SPEC-STORE-004` 의 구현 로드맵이다. 일정은 시간 단위가 아니라 **우선순위
> 기반 마일스톤**으로 표현한다. 개발 방법론은 프로젝트 설정(`quality.yaml`: hybrid — 신규=TDD,
> 기존=DDD)을 따른다. 본 SPEC 은 기존 store 모델을 재설계하므로 대부분 **DDD(ANALYZE-PRESERVE-
> IMPROVE)** 경로이며, 신규 시리즈 인코딩/레지스트리는 **TDD** 로 작성한다.

## 기술 접근 (Technical Approach)

1. **시리즈 식별을 1급 타입으로 도입**한다. `SeriesID{Key, MetricType, Tags}` (정규화 메서드
   포함) 와 `EncodeSeriesKey(SeriesID) string` 를 신설하여, 저장/레지스트리/조회가 동일한
   인코딩 규칙을 공유하게 한다. `tsdb.BuildSeriesKey` 규약(`measurement,k=v` 정렬)을 참조한다.
2. **쓰기 경로는 이미 metric/tags 를 보유**(`SetWithMeta`)하므로, 이를 "키 메타 덮어쓰기"에서
   "시리즈 라우팅"으로 전환한다. 즉 같은 key + 다른 metric/tags → 다른 저장 키.
3. **조회 경로는 key → 다중 시리즈 fan-out** 으로 확장하고, metric/tags 필터로 좁힌다.
   응답에 `labels`(metric_type + tags)를 채워 시리즈 출처를 식별한다.
4. **레지스트리(staticKeys) 키잉을 시리즈 단위로 승격**한다. `StaticKeyMeta` 는 이미 metric/tags
   를 보유하므로, 맵의 키를 bare key → 시리즈 인코딩으로 바꾸는 것이 핵심 변경이다.
5. **PRESERVE(DDD)**: 기존 동작(메타 없는 단일 키 쓰기 = 기본 시리즈, ErrKeyNotFound→200,
   gatekeeper 거부 시 흔적 없음, 동적=string 정책, 숫자 coercion)을 characterization 테스트로
   먼저 포착한 뒤 시리즈 모델로 진화한다.

## 아키텍처 설계 방향

- **레이어 경계 유지**: `node` → `system`(StoreAgent/NamespacedStore/VolatileStore) → 저장.
  시리즈 인코딩은 `system` 계층 내부 책임으로 둔다. node 패키지는 기존 `StoreWriteMeta`(metric/
  tags) 전달만 유지하고 시리즈 개념을 알 필요가 없다(캡슐화).
- **네임스페이스 결합 순서**: 최종 VolatileStore 키 = `namespace + ":" + EncodeSeriesKey(SeriesID)`.
  prefix 가 바깥, 시리즈 인코딩이 안쪽. `NamespacedStore.Keys`/`Clear` 의 prefix glob 동작 보존.
- **레지스트리 vs 저장 분리**: `staticKeys`(메타/등록정책 레지스트리)와 `VolatileStore`(값/history)
  는 동일 시리즈 인코딩을 키로 공유하되 별개 자료구조로 유지한다 (현 구조 보존).

## 마일스톤 (우선순위 기반)

### Primary Goal (M1) — 시리즈 식별/인코딩 기반 (Priority High)

- `SeriesID` 타입 + 정규화(tags 정렬, metric_type 기본값) + `EncodeSeriesKey` 신설 (신규 → TDD).
- 인코딩 후보 1(권장)을 PoC 하여 충돌 불가능성(N5)과 namespace 결합 순서를 확정한다.
- `tsdb.BuildSeriesKey` 와의 정렬(O1) 여부 결정.
- 산출: 인코딩 단위 테스트(정렬 불변성, 충돌 회피, round-trip 가능 여부).

### Primary Goal (M2) — 쓰기 경로 시리즈화 (Priority High)

- ANALYZE/PRESERVE: `checkKeyAllowed`/`coerceWriteValue`/`SetWithMeta`/`SetKeyMeta`/
  `SetKeyDataType` 의 현재 동작 characterization 테스트.
- IMPROVE: 위 함수 시그니처를 (key) → (SeriesID) 로 확장. `SetWithMeta` 가 metric/tags 로
  시리즈를 라우팅하도록 변경.
- 검증: 같은 key + 다른 metric/tags → 독립 시리즈(N1/N2/E2/E3), data_type 차이는 동일 시리즈
  (U3/N3).

### Primary Goal (M3) — 조회 경로 시리즈 fan-out (Priority High)

- `QueryHistory` 를 key → 다중 시리즈로 확장. metric/tags 필터 파라미터 추가.
- `POST /query` 바디에 `metric_type`/`tags` 필터 추가, 응답 `labels` 채움.
- `GET /keys` 를 시리즈 행으로 변경(기존 필터 재사용).
- 검증: 키 조회 시 전체 시리즈 반환(E4), 필터로 단일 시리즈(E5/S3), 미일치 시 200 빈 결과(S4).

### Secondary Goal (M4) — reset/삭제 + 집계 정합 (Priority Medium)

- `DELETE /keys/{key}` 단일 시리즈 대상화(E8). 식별자 누락 시 정책 확정.
- SPEC-WEB-005 `bucketAggregate` 를 다중 시리즈에서 시리즈별 적용 또는 단일 시리즈 강제.
- `ResetAll`/`IsStaticKey`/`StaticTagPairs` 의 시리즈 모델 정합.

### Secondary Goal (M5) — UI 정합 (Priority Medium)

- "TSDB 데이터 뷰어" 시리즈 행 표시, `POST /query` 다중 시리즈 응답을 labels 기준 라인 분리.
- i18n(en/ko) 라벨 보강.

### Final Goal (M6) — 영속/마이그레이션/문서 (Priority Low)

- `PersistentStore`/`StoreRepository` 시리즈 인코딩 정합(인터페이스 변경 최소화).
- 영속 데이터 일회성 마이그레이션 유틸(O3): 백업 → `(key,"unknown",{})` 기본 시리즈 변환 → 검증.
- store vs tsdb 역할 구분 문서화(O4). 통합 여부 결정 사항 정리.

## 리스크 및 대응

| 리스크 | 영향 | 대응 |
| --- | --- | --- |
| **tsdb 역할 중첩** | store 와 tsdb 가 사실상 동일 기능이 되어 혼란/중복 유지비 | 명세 §store vs tsdb 경계 표로 사용처 분리. 인코딩 규약만 공유(O1). 통합 여부는 별도 결정으로 분리하여 본 SPEC 범위를 식별 모델로 한정. |
| **마이그레이션 데이터 손실** | 영속 데이터 변환 중 손상 | 변환 전 백업 필수, dry-run 모드, 변환 후 카운트/샘플 검증. 인메모리는 휘발이므로 무변환(A6). |
| **시리즈 키 인코딩 충돌(N5)** | 서로 다른 식별자가 같은 저장 키로 오병합 | metric/tag 문자집합(`^[a-zA-Z0-9_-]+$`, A3)과 겹치지 않는 구분자 사용. 충돌 불가능성 단위 테스트. 우려 시 후보 2(해시) 폴백. |
| **성능/메모리 증가** | 한 key 가 N 시리즈로 분할되어 sync.Map 엔트리/메타 수 증가 | 시리즈 수 상한·관측(HealthCheck key_count 를 series_count 로 확장) 추가. 고카디널리티(tags) 경고. tsdb 위임 가이드. |
| **시그니처 광범위 변경** | (key)→(SeriesID) 변경이 다수 함수/테스트 파급 | M1 에서 SeriesID 를 도입하되 기존 (key) 호출을 `SeriesID{Key:key}` 기본 시리즈로 어댑트하는 내부 헬퍼로 점진 전환. |
| **API BREAKING** | `GET /keys` 행 의미·`POST /query` 응답 변화로 클라이언트 파손 | 응답에 `labels` 추가는 가산적. 시리즈 행 전환은 BREAKING 으로 명시하고 UI(M5)와 동시 배포. |
| **history 단위 변경(A5)** | key 단위 history 를 기대하던 로직 회귀 | characterization 테스트로 시리즈 단위 history 보장. QueryHistory/GetMetadata 영향 검증. |

## 품질 게이트 (TRUST 5 / 프로젝트 기준)

- Tested: 신규 시리즈 인코딩/레지스트리 TDD, 기존 경로 characterization. 패키지 커버리지 85%+
  목표(`go test -race ./...`).
- Readable/Unified: gofmt/goimports, golangci-lint 무경고.
- Secured: 입력(metric_type/tag) 정규식 검증 보존, 인코딩 인젝션 차단(N5).
- Trackable: 모든 변경에 `@spec SPEC-STORE-004` 태그, conventional commit.

## 의존성 순서

- M1 → M2 → M3 (식별 → 쓰기 → 조회) 는 선형 의존.
- M4/M5 는 M3 완료 후 착수. M6 은 마지막(영속 활성 시에만 실데이터 변환).

> Git 브랜치/커밋/PR 은 core-git(manager-git) 책임이며 본 계획에 포함하지 않는다.
