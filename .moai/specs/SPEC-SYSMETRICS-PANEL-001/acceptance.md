# SPEC-SYSMETRICS-PANEL-001 수용 기준 (acceptance.md)

Given/When/Then 형식. 백엔드 AC 는 `go test -race`, 프론트 AC 는 `vitest`, UI AC 는 수동 검증한다.

---

## REQ-01/02/03/04/05 — 에이전트 상태 스냅샷

### AC-01 — 표본 이전 조회는 오류가 아니다
- **Given** 방금 시작해 아직 표본을 뜨지 않은 sysmetrics 에이전트
- **When** `State()` 를 호출하면
- **Then** `status == "no_sample"` 을 반환한다.
- **And** panic 하지 않고 nil 도 아니다.

### AC-02 — 표본 1회 후 스냅샷 구성
- **Given** 5종 수집이 모두 켜진 에이전트가 표본을 1회 떴을 때
- **When** `State()` 를 호출하면
- **Then** `status == "running"` 이고 `collected_at` 이 그 표본의 시각(epoch ms)이다.
- **And** `cpu` / `memory` / `disk_io` / `network` / `storage` 키가 모두 존재한다.
- **And** `interval_seconds` 가 설정된 표본 주기와 같다.

### AC-03 — 소비자 없이도 스냅샷이 갱신된다 (회귀 가드)
- **Given** 표본 채널 버퍼를 가득 채워 `emitSample` 의 `default` 분기가 타도록 만든 에이전트
- **When** 표본 주기가 2회 경과하면
- **Then** `State()` 의 `collected_at` 이 두 번 모두 갱신되어 있다.
- **And** "버퍼가 가득 차 표본을 버렸습니다" 경고 로그가 남아도 스냅샷은 최신이다.

> 이 AC 가 이 SPEC 의 핵심이다. 플로우를 만들지 않은 사용자가 패널만 쓰는 경우가 정확히 이 상태다.

### AC-04 — 수집이 꺼진 지표는 키가 없다
- **Given** `CollectStorage: false`, `CollectDiskIO: false` 로 설정된 에이전트가 표본을 떴을 때
- **When** `State()` 를 호출하면
- **Then** `storage` 와 `disk_io` 키가 존재하지 않는다 (값이 0 이거나 빈 맵이 아니라 **부재**).
- **And** `cpu` / `memory` / `network` 는 정상 존재한다.

### AC-05 — targets 는 표본에서 유도한다
- **Given** `Interfaces` 설정이 빈(=전체) 에이전트가 `en0`, `lo0` 을 수집한 표본
- **When** `State()` 를 호출하면
- **Then** `targets.interfaces == ["en0", "lo0"]` (정렬)이다.
- **And** 설정이 비어 있음에도 실제 관측 대상 이름이 드러난다.

### AC-06 — 동시 호출에 데이터 경합이 없다
- **Given** 표본 루프가 도는 에이전트
- **When** `go test -race` 하에서 `State()` 를 100회 동시 호출하면
- **Then** race detector 가 보고하지 않는다.
- **And** 반환된 각 스냅샷은 서로 섞이지 않은 일관된 하나다.

### AC-07 — API 응답에 실린다
- **Given** 실행 중인 sysmetrics 에이전트
- **When** `GET /agents/{id}?detail=summary` 또는 `detail=full` 을 호출하면
- **Then** 응답 `state` 필드에 AC-02 의 구조가 담긴다.
- **And** `detail` 을 지정하지 않은 목록 응답(`GET /agents` 의 기본 `detail=""`)에는 `state` 가 없다.

> `state` 는 summary·full **양쪽**에 실린다 (`agent_adapter.go:620-628` — summary 에서 제거되는 것은 Store 의 `entries` 뿐이다). 패널은 더 가벼운 `summary` 를 쓴다.

---

## REQ-07/11/12/14/15/21 — 시리즈 변환 (순수 함수)

### AC-08 — 종합은 전체 합산이다
- **Given** `disk0`(read 100, write 200), `disk1`(read 50, write 20) 를 담은 스냅샷
- **When** `sumDiskIO(snapshot)` 을 호출하면
- **Then** `read_bytes == 150`, `write_bytes == 220` 이다.

### AC-09 — rate 는 차분을 경과시간으로 나눈다
- **Given** `t=0` 에 `bytes_recv=1000`, `t=2000ms` 에 `bytes_recv=3000` 인 두 표본
- **When** `deltaRate(prev, next, 'second')` 를 호출하면
- **Then** `1000` (bytes/s) 을 반환한다.
- **And** `unitTime='minute'` 이면 `60000` 을 반환한다.

### AC-10 — 카운터 되감김은 0 으로 처리한다
- **Given** 직전 `bytes_recv=5000`, 새 표본 `bytes_recv=100` (인터페이스 재설정)
- **When** `deltaRate` 를 호출하면
- **Then** `0` 을 반환한다.
- **And** 음수를 반환하지 않는다.

### AC-11 — 중복 표본은 새 점을 만들지 않는다
- **Given** 직전과 `collected_at` 이 동일한 응답 (폴링이 표본 주기보다 빠름)
- **When** 시리즈에 반영하면
- **Then** 창의 점 개수가 늘지 않는다.
- **And** 마지막 rate 값이 0 으로 계단화되지 않는다.

### AC-12 — 스토리지 종합은 합계 기준 사용률이다
- **Given** `/`(used 30, total 100), `/data`(used 10, total 100) 인 스냅샷
- **When** `sumStorage(snapshot)` 을 호출하면
- **Then** `used == 40`, `total == 200`, `usage_percent == 20` 이다.
- **And** 각 마운트 사용률의 산술평균(20 과 10 의 평균 15)이 **아니다**.

### AC-13 — 같은 볼륨의 여러 마운트를 합치지 않는다
- **Given** `/` 와 `/System/Volumes/Data` 가 동일한 `used`/`total` 수치를 갖는 스냅샷
- **When** 파티션별 표시를 만들면
- **Then** 두 마운트가 각각 하나의 행으로 나온다.
- **And** 종합 합계에는 두 값이 모두 더해진다 (중복 제거 없음).

### AC-14 — 수집 꺼짐과 0 을 구분한다
- **Given** `storage` 키가 부재한 스냅샷
- **When** `isCollected(snapshot, 'storage')` 를 호출하면
- **Then** `false` 를 반환한다.
- **And** `storage` 가 빈 맵 `{}` 인 경우(수집은 켰으나 대상이 없음)에는 `true` 를 반환한다.

---

## REQ-06/08 — 시스템 패널

### AC-15 — 네 항목을 표시한다
- **Given** 5종 수집이 모두 켜진 에이전트에 바인딩된 `sysmetrics-system` 패널
- **When** 스냅샷이 도착하면
- **Then** CPU 사용률, 메모리, 디스크 I/O 종합, 네트워크 종합 항목이 렌더된다.

### AC-16 — items 선택을 존중한다
- **Given** `config.items = ['cpu']`
- **When** 패널을 렌더하면
- **Then** CPU 항목만 나오고 나머지 셋은 나오지 않는다.

### AC-17 — 빈 items 는 "모두 끔"이다
- **Given** `config.items = []`
- **When** 패널을 렌더하면
- **Then** 항목이 하나도 렌더되지 않는다 (기본값으로 되돌리지 않는다).

---

## REQ-09/10 — 네트워크 패널

### AC-18 — 대상 미선택이면 합산 1개
- **Given** `config.interfaces` 가 비어 있고 `en0`/`en1` 을 수집 중인 스냅샷
- **When** 패널을 렌더하면
- **Then** 시리즈가 정확히 1개이고 그 값은 두 인터페이스의 합이다.

### AC-19 — 선택한 인터페이스마다 시리즈
- **Given** `config.interfaces = ['en0', 'en1']`
- **When** 패널을 렌더하면
- **Then** 시리즈가 2개이고 각각 `en0`, `en1` 라벨을 갖는다.

### AC-20 — 사라진 인터페이스는 건너뛴다
- **Given** `config.interfaces = ['en0', 'en9']` 인데 스냅샷에 `en9` 가 없을 때
- **When** 패널을 렌더하면
- **Then** `en0` 시리즈만 그려지고 오류를 던지지 않는다.

---

## REQ-13/14 — 스토리지 패널

### AC-21 — 대상 미선택이면 합계 표시
- **Given** `config.mountpoints` 가 비어 있고 마운트 3개를 수집 중인 스냅샷
- **When** 패널을 렌더하면
- **Then** 합계 사용률·사용량·여유·전체가 한 덩어리로 표시된다.

### AC-22 — 선택한 마운트마다 행
- **Given** `config.mountpoints = ['/', '/data']`
- **When** 패널을 렌더하면
- **Then** 두 행이 각각 사용률·사용량·여유·전체를 표시한다.

---

## REQ-16/17/19/20/21 — 배선과 폴백

### AC-23 — 카탈로그에 3종이 하위 그룹으로 등장
- **Given** 패널 추가 다이얼로그의 `system` 카테고리
- **When** 열면
- **Then** 기존 monitor 5종과 신규 sysmetrics 3종이 **각각 제목 있는 그룹**으로 나뉘어 보인다.
- **And** 기존 5종의 타입·순서·라벨은 변하지 않는다.

### AC-24 — sysmetrics 에이전트만 선택 대상이다
- **Given** store·influxdb·sysmetrics 에이전트가 공존하는 프로젝트
- **When** `sysmetrics-system` 패널 추가의 에이전트 선택 스텝을 열면
- **Then** sysmetrics 타입 에이전트만 목록에 나온다.

### AC-25 — agent_id 를 정본으로 저장한다
- **Given** 에이전트를 선택해 패널을 추가한 뒤 그 에이전트의 이름을 바꿨을 때
- **When** 패널을 다시 렌더하면
- **Then** 여전히 같은 에이전트의 지표가 그려진다.
- **And** 표시 이름은 새 이름으로 갱신된다.

### AC-26 — 에이전트 부재 폴백
- **Given** 바인딩된 에이전트가 삭제된 패널
- **When** 대시보드를 열면
- **Then** 패널에 상태 안내가 표시된다.
- **And** 대시보드의 다른 패널은 정상 렌더된다 (오류 경계가 전파되지 않는다).

### AC-27 — 중지 상태 안내
- **Given** 바인딩된 에이전트가 중지 상태
- **When** 패널을 렌더하면
- **Then** 마지막 값을 0 으로 덮어쓰지 않고 중지 상태임을 알린다.

### AC-28 — 수집 꺼짐 표시
- **Given** `CollectStorage: false` 인 에이전트에 바인딩된 `sysmetrics-storage` 패널
- **When** 패널을 렌더하면
- **Then** "수집 꺼짐" 안내가 표시된다.
- **And** 사용률 0% 로 그려지지 않는다.

### AC-29 — 같은 에이전트를 보는 패널들이 요청을 공유한다
- **Given** 같은 에이전트에 바인딩된 패널 3개가 한 대시보드에 있을 때
- **When** 한 폴링 주기가 지나면
- **Then** 해당 에이전트에 대한 HTTP 요청이 1회다 (3회가 아니다).

---

## 통합 검증

### AC-30 — 플로우 없이 동작한다 (수동)
- **Given** sysmetrics 에이전트 1개만 생성하고 **플로우는 만들지 않은** 상태
- **When** 세 패널을 대시보드에 배치하면
- **Then** 세 패널 모두 실제 값을 그린다.
- **And** 브라우저를 새로고침해도 다음 표본부터 다시 그린다.

### AC-31 — 기존 대시보드 무영향
- **Given** 기존 monitor 패널 5종이 배치된 대시보드
- **When** 이 SPEC 구현 후 대시보드를 열면
- **Then** 다섯 패널의 표시 내용과 설정이 이전과 동일하다.
- **And** 패널 config 마이그레이션이 실행되지 않는다.

### AC-32 — 품질 게이트
- **When** 검증 배치를 실행하면
- **Then** `go test -race ./internal/...` 통과, `npx vitest run` 통과, `npx tsc --noEmit` exit 0 이다.
