# M1 실측 프로브 — 실행 지시서

SPEC-TSDB-003 M1(실측 스파이크)의 질의 세트다. **산출물은 코드가 아니라 관측 기록**이므로,
아래 질의를 실행하고 **출력을 그대로(가공하지 말고)** 돌려주면 된다.

가장 중요한 두 가지를 먼저 적는다.

- **P5 가 OQ2 를 닫는다** — v3 열거를 SQL `DISTINCT` 3질의로 갈지 InfluxQL `GROUP BY *` 1질의로 갈지가 여기서 결정된다.
- **P7 이 이미 커밋된 M2 코드의 가정을 검증한다** — M2 는 실서버 관측 없이 작성되었고, `group()` 이후의 컬럼 형상을 **가정**하고 있다. 가정이 틀리면 시리즈가 잘못 쪼개진다.

---

## 0. 먼저 알려줄 것

| 항목 | 예 |
|------|-----|
| 버전 | `v2` 또는 `v3` |
| URL | `http://10.0.1.5:8086` |
| database(v3) 또는 bucket+org(v2) | `metrics` / `metrics` + `myorg` |
| 태그가 2개 이상 있는 measurement 이름 | `cpu` |
| (있다면) 태그가 **0개**인 measurement 이름 | `heartbeat` |

아래 질의의 `<...>` 자리를 위 값으로 바꿔 실행한다.

### 시간 범위

질의에 시간 창이 필요하다. 데이터가 있는 아무 구간이나 좋다. 아래 예시는 **최근 30일**을 쓴다
(SPEC 의 탐색 창 기본값과 같다).

---

## A. v3 인 경우 — P1 ~ P6

InfluxDB 3 의 HTTP 질의 엔드포인트는 두 개다. `q` 를 바꿔 가며 같은 형태로 던지면 된다.

```bash
# InfluxQL
curl -s -X POST "$URL/api/v3/query_influxql" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"db":"<database>","q":"<질의>","format":"json"}'

# SQL
curl -s -X POST "$URL/api/v3/query_sql" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"db":"<database>","q":"<질의>","format":"json"}'
```

### P1 (M1.2) — `SHOW SERIES` 가 정말 안 되는지

문서상 미지원이다. **실패를 관측하는 것이 목적**이므로 오류 문구를 그대로 받고 싶다.

```sql
-- query_influxql
SHOW SERIES FROM "<measurement>"
```

> 돌려줄 것: 전체 응답(오류 메시지 원문). 만약 **성공한다면** 그것이 훨씬 큰 뉴스다 —
> 설계가 통째로 단순해지므로 즉시 알려달라.

### P2 (M1.3) — 기존 D2·D4 회귀 확인

```sql
-- query_influxql, 2건 각각
SHOW TAG KEYS FROM "<measurement>"
SHOW FIELD KEYS FROM "<measurement>"
```

> 돌려줄 것: 두 응답 전체. (컬럼 이름이 `tagKey` / `fieldKey` 인지가 확인 대상이다.)

### P3 (M1.4) — SQL `DISTINCT` 후보의 행 형상

`<tagkey>` 는 P2 가 돌려준 태그 키 중 하나를 쓴다.

```sql
-- query_sql
SELECT DISTINCT "<tagkey>" FROM "<measurement>"
WHERE time >= now() - INTERVAL '30 days'
LIMIT 10
```

> 돌려줄 것: 응답 전체. **키 이름이 무엇으로 오는지**가 핵심이다
> (`<tagkey>` 그대로인지, 아니면 `DISTINCT <tagkey>` 같은 형태인지).

### P4 (M1.4 보조) — v3 에서 태그가 컬럼인지

```sql
-- query_sql
SELECT * FROM "<measurement>"
WHERE time >= now() - INTERVAL '30 days'
LIMIT 1
```

> 돌려줄 것: 응답 전체(1행). 태그가 일반 컬럼으로 나오는지 본다.

### P5 (M1.5) — **OQ2 를 닫는 질의**

`<field>` 는 P2 의 `SHOW FIELD KEYS` 가 돌려준 필드 중 하나.

```sql
-- query_influxql
SELECT "<field>" FROM "<measurement>"
WHERE time >= now() - 30d
GROUP BY *
LIMIT 1
```

> 돌려줄 것: **응답 전체**. 축약하지 말 것.
> 확인하려는 것은 딱 하나 — **태그가 각 행의 컬럼으로 나타나는가, 아니면 별도의 `tags`
> 객체/그룹 메타로 나타나는가.** 전자면 `GROUP BY *` 1질의 경로를 택하고, 후자면
> SQL `DISTINCT` 3질의 경로를 택한다. 응답 구조가 그대로 답이다.

### P6 (M1.6) — 태그 0개 measurement

태그가 없는 measurement 가 있다면 P2 · P5 를 그 measurement 로 한 번 더 실행한다.
없으면 "해당 measurement 없음" 이라고만 알려주면 된다.

---

## B. v2 인 경우 (또는 v2 도 함께 있는 경우) — P7

**P7 은 v3 여부와 무관하게, v2 인스턴스가 있다면 반드시 받고 싶다.**
이미 커밋된 M2 코드가 이 응답의 형상을 가정하고 작성되었기 때문이다.

아래는 `BuildFluxSeriesEnumQuery` 가 **실제로 생성하는 질의**다. `<bucket>` · `<measurement>` 만
바꾸고 나머지는 그대로 둔다.

```bash
curl -s -X POST "$URL/api/v2/query?org=<org>" \
  -H "Authorization: Token $TOKEN" \
  -H "Content-Type: application/vnd.flux" \
  -H "Accept: application/csv" \
  --data-binary '
from(bucket: "<bucket>")
  |> range(start: -30d)
  |> filter(fn: (r) => r._measurement == "<measurement>")
  |> first()
  |> group()
  |> limit(n: 50)
'
```

> 돌려줄 것: **주석 CSV 원문 그대로**(맨 위 `#datatype` · `#group` · `#default` 주석 줄 포함).
> 파싱하거나 정리하지 말 것 — 주석 줄이 바로 확인 대상이다.
>
> 확인하려는 것 3가지:
> 1. `result` · `table` 컬럼이 실제로 존재하는가 (M2 는 존재한다고 보고 명시적으로 제외한다)
> 2. 서로 다른 태그 집합의 행이 한 테이블로 합쳐질 때, **없는 태그 컬럼이 어떻게 채워지는가** —
>    빈 문자열인가, `null` 인가, 아니면 아예 **별도 result 섹션**으로 쪼개지는가
>    (M2 는 "빈 문자열" 을 가정하고 "빈 값 = 태그 아님" 규칙을 넣었다)
> 3. `_field` 가 컬럼으로 있는가

`-30d` 대신 M2 가 쓰는 절대 나노초 형식을 그대로 시험하고 싶다면 아래를 쓴다
(형식 자체가 서버에 받아들여지는지 확인용).

```flux
  |> range(start: time(v: 1755000000000000000), stop: time(v: 1757592000000000000))
```

---

## C. (선택) P8 — 카디널리티 감

OQ4 의 상한 1,000 은 측정값이 아니라 추정이다. 큰 measurement 가 있다면:

```sql
-- v3 / query_influxql
SHOW TAG VALUES FROM "<measurement>" WITH KEY = "<가장 카디널리티 큰 태그키>"
```

> 돌려줄 것: 값의 **개수**와 체감 응답 시간. 전체 목록은 필요 없다.

---

## 돌려받고 싶은 것 요약

| # | 질의 | 무엇을 정하는가 | 필수 |
|---|------|----------------|------|
| P1 | `SHOW SERIES` | 문서와 실제의 일치 확인 | v3 필수 |
| P2 | `SHOW TAG KEYS` · `SHOW FIELD KEYS` | 기존 D2·D4 회귀 + 컬럼명 | v3 필수 |
| P3 | SQL `DISTINCT` | SQL 경로의 행 형상 | v3 필수 |
| P4 | SQL `SELECT *` | v3 에서 태그가 컬럼인지 | v3 필수 |
| **P5** | InfluxQL `GROUP BY *` | **OQ2 확정** | **v3 필수** |
| P6 | 태그 0개 measurement | 예외 분기 | 있으면 |
| **P7** | v2 Flux 열거 (주석 CSV 원문) | **M2 가정 검증** | **v2 있으면 필수** |
| P8 | 태그 값 개수 | OQ4 상한 보정 | 선택 |

출력은 길어도 좋다 — 축약된 요약보다 원문이 필요하다.
