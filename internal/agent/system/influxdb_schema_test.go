// @spec SPEC-TSDB-002 §2.10 (U10) — 스키마 디스커버리 D2~D4.
package system

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===== 컴파일 타임 계약 =====

func TestInfluxSchema_인터페이스_준수(t *testing.T) {
	t.Parallel()
	var _ InfluxSchemaDiscoverer = (*InfluxDBAgent)(nil)
	// 두 어댑터 모두 InfluxClient 를 만족해야 한다 — 디스커버리 3 종은 v3 도
	// 지원한다(v3 에 없는 것은 관리 API 이지 스키마 조회가 아니다).
	var _ InfluxClient = (*influxV2Client)(nil)
	var _ InfluxClient = (*influxV3Client)(nil)
}

// ===== v2 (Flux) 쿼리 생성 =====

func TestInfluxSchema_BuildFluxQueries_D2D3D4(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func() (string, error)
		want  string
	}{
		{
			name:  "D2 tag keys",
			build: func() (string, error) { return buildFluxTagKeysQuery("metrics", "cpu") },
			want: "import \"influxdata/influxdb/schema\"\n" +
				"schema.measurementTagKeys(bucket: \"metrics\", measurement: \"cpu\")",
		},
		{
			name:  "D3 tag values",
			build: func() (string, error) { return buildFluxTagValuesQuery("metrics", "cpu", "host") },
			want: "import \"influxdata/influxdb/schema\"\n" +
				"schema.measurementTagValues(bucket: \"metrics\", measurement: \"cpu\", tag: \"host\")",
		},
		{
			name:  "D4 field keys",
			build: func() (string, error) { return buildFluxFieldKeysQuery("metrics", "cpu") },
			want: "import \"influxdata/influxdb/schema\"\n" +
				"schema.measurementFieldKeys(bucket: \"metrics\", measurement: \"cpu\")",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tt.build()
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestInfluxSchema_Flux_이스케이프 는 사용자 데이터가 Flux 문자열을 벗어나지
// 못함을 확인한다(§2.7 · UB1-9).
//
// ${ 를 함께 검사하는 이유는 Flux 문자열이 그 형태를 보간으로 해석하기 때문이다.
// tsdbtags 가 쓰는 %q 는 이 형태를 이스케이프하지 않는다 — 복제하되 그 지점만
// 강화했다.
func TestInfluxSchema_Flux_이스케이프(t *testing.T) {
	t.Parallel()

	got, err := buildFluxTagValuesQuery(`b"1`, `m"2`, `t${x}`)
	require.NoError(t, err)
	assert.Contains(t, got, `bucket: "b\"1"`)
	assert.Contains(t, got, `measurement: "m\"2"`)
	assert.Contains(t, got, `tag: "t\${x}"`)
}

// ===== v3 (InfluxQL) 쿼리 생성 =====

func TestInfluxSchema_BuildInfluxQLQueries_D2D3D4(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func() (string, error)
		want  string
	}{
		{
			name:  "D2 tag keys",
			build: func() (string, error) { return buildInfluxQLTagKeysQuery("cpu") },
			want:  `SHOW TAG KEYS FROM "cpu"`,
		},
		{
			name:  "D3 tag values",
			build: func() (string, error) { return buildInfluxQLTagValuesQuery("cpu", "host") },
			want:  `SHOW TAG VALUES FROM "cpu" WITH KEY = "host"`,
		},
		{
			name:  "D4 field keys",
			build: func() (string, error) { return buildInfluxQLFieldKeysQuery("cpu") },
			want:  `SHOW FIELD KEYS FROM "cpu"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tt.build()
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestInfluxSchema_InfluxQL_이스케이프(t *testing.T) {
	t.Parallel()

	got, err := buildInfluxQLTagValuesQuery(`m"1`, `t"2`)
	require.NoError(t, err)
	assert.Equal(t, `SHOW TAG VALUES FROM "m\"1" WITH KEY = "t\"2"`, got)
}

// ===== 인자 검증 =====

func TestInfluxSchema_필수인자_검증(t *testing.T) {
	t.Parallel()

	t.Run("measurement 누락", func(t *testing.T) {
		t.Parallel()
		_, err := buildFluxTagKeysQuery("metrics", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "measurement is required")

		_, err = buildInfluxQLFieldKeysQuery("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "measurement is required")
	})

	t.Run("tag key 누락", func(t *testing.T) {
		t.Parallel()
		_, err := buildFluxTagValuesQuery("metrics", "cpu", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tag key is required")

		_, err = buildInfluxQLTagValuesQuery("cpu", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tag key is required")
	})

	t.Run("제어문자는 이스케이프 불가", func(t *testing.T) {
		t.Parallel()
		_, err := buildFluxTagKeysQuery("metrics", "cpu\n")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrUnescapableIdentifier)

		_, err = buildInfluxQLTagKeysQuery("cpu\x00")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrUnescapableIdentifier)

		_, err = buildFluxTagKeysQuery("met\trics", "cpu")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrUnescapableIdentifier)
	})
}

// ===== 결과 파싱 =====

func TestInfluxSchema_컬럼파싱(t *testing.T) {
	t.Parallel()

	t.Run("Flux 는 _value 컬럼을 읽는다", func(t *testing.T) {
		t.Parallel()
		rows := []map[string]any{
			{"_value": "host"},
			{"_value": ""},       // 빈 문자열은 버린다.
			{"_value": 42},       // 문자열이 아닌 값은 버린다.
			{"other": "ignored"}, // 컬럼 부재는 버린다.
			{"_value": "region"},
		}
		assert.Equal(t, []string{"host", "region"}, collectFluxSchemaValues(rows))
	})

	t.Run("InfluxQL 은 지정 컬럼을 읽는다", func(t *testing.T) {
		t.Parallel()
		rows := []map[string]any{
			{"fieldKey": "usage", "fieldType": "float"},
			{"fieldKey": "load", "fieldType": "float"},
			{"tagKey": "host"}, // 다른 컬럼은 읽지 않는다.
		}
		assert.Equal(t, []string{"usage", "load"}, collectInfluxQLColumn(rows, influxQLFieldKeyColumn))
	})

	t.Run("빈 결과는 빈 슬라이스", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, collectFluxSchemaValues(nil))
		assert.Empty(t, collectInfluxQLColumn(nil, influxQLTagKeyColumn))
	})
}

// TestInfluxSchema_태그키_내부컬럼_제외 는 schema.measurementTagKeys 가 함께
// 돌려주는 _start · _stop · _measurement · _field 를 걸러냄을 확인한다.
// tsdbtags/client_v2.go 가 갖는 같은 필터의 복제본이다.
func TestInfluxSchema_태그키_내부컬럼_제외(t *testing.T) {
	t.Parallel()

	in := []string{"_start", "_stop", "_measurement", "_field", "host", "region"}
	assert.Equal(t, []string{"host", "region"}, filterInternalTagKeys(in))
}

// ===== D4 — 신규 구현 (AC-30) =====

// TestInfluxSchema_ListFieldKeys_V2 는 D4 의 v2 경로를 검증한다.
// D4 는 tsdbtags 에 대응물이 없는 신규 구현이다(§1.2.10 · §2.10).
func TestInfluxSchema_ListFieldKeys_V2(t *testing.T) {
	t.Parallel()

	q, err := buildFluxFieldKeysQuery("metrics", "cpu")
	require.NoError(t, err)
	assert.Contains(t, q, "schema.measurementFieldKeys(")
	assert.Contains(t, q, `bucket: "metrics"`)
	assert.Contains(t, q, `measurement: "cpu"`)
	// 태그 함수를 잘못 부르지 않는다.
	assert.NotContains(t, q, "measurementTagKeys")
	assert.NotContains(t, q, "measurementTagValues")

	rows := []map[string]any{{"_value": "usage"}, {"_value": "load"}}
	assert.Equal(t, []string{"usage", "load"}, collectFluxSchemaValues(rows))
}

// TestInfluxSchema_ListFieldKeys_V3 는 D4 의 v3 경로를 검증한다.
func TestInfluxSchema_ListFieldKeys_V3(t *testing.T) {
	t.Parallel()

	q, err := buildInfluxQLFieldKeysQuery("cpu")
	require.NoError(t, err)
	assert.Equal(t, `SHOW FIELD KEYS FROM "cpu"`, q)

	// SHOW FIELD KEYS 는 fieldKey 와 fieldType 두 컬럼을 돌려주며, 필요한 것은
	// fieldKey 다. fieldType 을 읽으면 "float" 같은 타입 이름이 필드 목록으로
	// 새어 나간다.
	rows := []map[string]any{
		{"fieldKey": "usage", "fieldType": "float"},
		{"fieldKey": "load", "fieldType": "integer"},
	}
	assert.Equal(t, []string{"usage", "load"}, collectInfluxQLColumn(rows, influxQLFieldKeyColumn))
}

// ===== 에이전트 위임 =====

func TestInfluxDBAgent_디스커버리_위임(t *testing.T) {
	t.Parallel()

	var gotBucket, gotMeasurement, gotTagKey string
	mock := &mockInfluxClient{
		listTagKeysFunc: func(_ context.Context, bucket, measurement string) ([]string, error) {
			gotBucket, gotMeasurement = bucket, measurement
			return []string{"host"}, nil
		},
		listTagValuesFunc: func(_ context.Context, bucket, measurement, tagKey string) ([]string, error) {
			gotBucket, gotMeasurement, gotTagKey = bucket, measurement, tagKey
			return []string{"a", "b"}, nil
		},
		listFieldKeysFunc: func(_ context.Context, bucket, measurement string) ([]string, error) {
			gotBucket, gotMeasurement = bucket, measurement
			return []string{"usage"}, nil
		},
	}
	a := newTestInfluxDBAgent(mock)
	ctx := context.Background()

	keys, err := a.ListTagKeys(ctx, "explicit", "cpu")
	require.NoError(t, err)
	assert.Equal(t, []string{"host"}, keys)
	assert.Equal(t, "explicit", gotBucket)
	assert.Equal(t, "cpu", gotMeasurement)

	values, err := a.ListTagValues(ctx, "explicit", "cpu", "host")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, values)
	assert.Equal(t, "host", gotTagKey)

	fields, err := a.ListFieldKeys(ctx, "explicit", "cpu")
	require.NoError(t, err)
	assert.Equal(t, []string{"usage"}, fields)
}

// TestInfluxDBAgent_디스커버리_기본버킷_fallback 는 빈 bucket 이 에이전트 설정의
// 기본 버킷으로 채워짐을 확인한다. ListMeasurements 와 같은 규약이다.
func TestInfluxDBAgent_디스커버리_기본버킷_fallback(t *testing.T) {
	t.Parallel()

	var seen []string
	mock := &mockInfluxClient{
		listTagKeysFunc: func(_ context.Context, bucket, _ string) ([]string, error) {
			seen = append(seen, bucket)
			return nil, nil
		},
		listTagValuesFunc: func(_ context.Context, bucket, _, _ string) ([]string, error) {
			seen = append(seen, bucket)
			return nil, nil
		},
		listFieldKeysFunc: func(_ context.Context, bucket, _ string) ([]string, error) {
			seen = append(seen, bucket)
			return nil, nil
		},
	}
	a := newTestInfluxDBAgent(mock)
	ctx := context.Background()

	_, err := a.ListTagKeys(ctx, "", "cpu")
	require.NoError(t, err)
	_, err = a.ListTagValues(ctx, "", "cpu", "host")
	require.NoError(t, err)
	_, err = a.ListFieldKeys(ctx, "", "cpu")
	require.NoError(t, err)

	require.NotEmpty(t, a.defaultBucket())
	assert.Equal(t, []string{a.defaultBucket(), a.defaultBucket(), a.defaultBucket()}, seen)
}

func TestInfluxDBAgent_디스커버리_클라이언트_미초기화(t *testing.T) {
	t.Parallel()

	a := newTestInfluxDBAgent(nil)
	a.client = nil
	ctx := context.Background()

	_, err := a.ListTagKeys(ctx, "b", "cpu")
	assert.ErrorIs(t, err, errClientNotInitialized)
	_, err = a.ListTagValues(ctx, "b", "cpu", "host")
	assert.ErrorIs(t, err, errClientNotInitialized)
	_, err = a.ListFieldKeys(ctx, "b", "cpu")
	assert.ErrorIs(t, err, errClientNotInitialized)
}

func TestInfluxDBAgent_디스커버리_오류_전파(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("upstream down")
	mock := &mockInfluxClient{
		listFieldKeysFunc: func(_ context.Context, _, _ string) ([]string, error) {
			return nil, sentinel
		},
	}
	a := newTestInfluxDBAgent(mock)

	_, err := a.ListFieldKeys(context.Background(), "b", "cpu")
	assert.ErrorIs(t, err, sentinel)
}

// TestInfluxSchema_NoCache 는 디스커버리 응답을 캐시하지 않음을 확인한다
// (§2.10 · UB1-12 · AC-33).
//
// 스키마는 쓰기에 따라 변한다. 캐시하면 사용자가 방금 만든 measurement 가 목록에
// 없거나, 삭제된 시리즈를 골라 빈 결과를 보게 된다. 같은 인자로 두 번 부르면
// 클라이언트도 두 번 불려야 한다.
func TestInfluxSchema_NoCache(t *testing.T) {
	t.Parallel()

	var tagKeyCalls, tagValueCalls, fieldKeyCalls int
	mock := &mockInfluxClient{
		listTagKeysFunc: func(_ context.Context, _, _ string) ([]string, error) {
			tagKeyCalls++
			return []string{"host"}, nil
		},
		listTagValuesFunc: func(_ context.Context, _, _, _ string) ([]string, error) {
			tagValueCalls++
			return []string{"a"}, nil
		},
		listFieldKeysFunc: func(_ context.Context, _, _ string) ([]string, error) {
			fieldKeyCalls++
			return []string{"usage"}, nil
		},
	}
	a := newTestInfluxDBAgent(mock)
	ctx := context.Background()

	for range 2 {
		_, err := a.ListTagKeys(ctx, "metrics", "cpu")
		require.NoError(t, err)
		_, err = a.ListTagValues(ctx, "metrics", "cpu", "host")
		require.NoError(t, err)
		_, err = a.ListFieldKeys(ctx, "metrics", "cpu")
		require.NoError(t, err)
	}

	assert.Equal(t, 2, tagKeyCalls, "tag key 응답이 캐시되었다")
	assert.Equal(t, 2, tagValueCalls, "tag value 응답이 캐시되었다")
	assert.Equal(t, 2, fieldKeyCalls, "field key 응답이 캐시되었다")
}

// TestInfluxSchema_V2_기본버킷_해석 은 클라이언트 수준의 빈 bucket 처분을 잠근다.
func TestInfluxSchema_V2_기본버킷_해석(t *testing.T) {
	t.Parallel()

	c := &influxV2Client{bucket: "fallback"}
	got, err := c.resolveSchemaBucket("")
	require.NoError(t, err)
	assert.Equal(t, "fallback", got)

	got, err = c.resolveSchemaBucket("explicit")
	require.NoError(t, err)
	assert.Equal(t, "explicit", got)

	empty := &influxV2Client{}
	_, err = empty.resolveSchemaBucket("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bucket")
}

// ===== v2 어댑터 왕복 (M7.2) =====

// schemaValueCSV 는 schema.* 결과의 주석 CSV 를 만든다.
// schema.measurementTagKeys · measurementTagValues · measurementFieldKeys 는
// 모두 결과를 _value 한 컬럼에 담는다.
func schemaValueCSV(values ...string) string {
	var b strings.Builder
	b.WriteString("#datatype,string,long,string\r\n")
	b.WriteString("#group,false,false,false\r\n")
	b.WriteString("#default,_result,,\r\n")
	b.WriteString(",result,table,_value\r\n")
	for _, v := range values {
		b.WriteString(",,0," + v + "\r\n")
	}
	return b.String()
}

// TestInfluxSchema_V2_어댑터_왕복_D2D3D4 는 influxV2Client 의 디스커버리 3종을
// httptest 인프로세스 서버로 끝에서 끝까지 태운다.
//
// 기존 테스트는 쿼리 생성(buildFlux*)과 결과 파싱(collectFluxSchemaValues)을
// 각각 따로 고정했지만, 그 둘을 잇는 어댑터 자체는 한 번도 실행되지 않았다.
// 어댑터가 하는 일 — 기본 버킷 해석 → 쿼리 생성 → queryFlux 왕복 → 내부 컬럼
// 필터 — 의 배선은 조각 테스트로 드러나지 않는다. 배선이 어긋나면(예: 태그 키
// 경로에서 filterInternalTagKeys 를 빠뜨리면) 사용자에게 _measurement · _field
// 가 고를 수 있는 태그로 보인다.
//
// **v3 는 같은 방식을 쓸 수 없다.** influxdb3-go 는 Arrow Flight(gRPC)로
// 질의하므로 net/http/httptest 서버로 대체되지 않는다
// (influxdb_seriesenum_v3_test.go 머리말과 같은 제약).
func TestInfluxSchema_V2_어댑터_왕복_D2D3D4(t *testing.T) {
	t.Parallel()

	var seen []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(body, &req)
		seen = append(seen, req.Query)

		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		switch {
		case strings.Contains(req.Query, "measurementTagKeys"):
			// 내부 컬럼이 섞여 오는 실제 형상 — 필터 대상이다.
			_, _ = w.Write([]byte(schemaValueCSV("_measurement", "host", "_field", "region", "_start")))
		case strings.Contains(req.Query, "measurementTagValues"):
			_, _ = w.Write([]byte(schemaValueCSV("kr", "jp")))
		case strings.Contains(req.Query, "measurementFieldKeys"):
			_, _ = w.Write([]byte(schemaValueCSV("usage", "idle")))
		default:
			_, _ = w.Write([]byte(schemaValueCSV()))
		}
	}))
	defer ts.Close()

	c, err := newInfluxV2Client(InfluxDBConfig{URL: ts.URL, Token: "tok", Org: "org", Bucket: "metrics"})
	require.NoError(t, err)
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	// D2 — 태그 키. 빈 bucket 이 클라이언트 기본 버킷으로 해석되고, 밑줄로
	// 시작하는 내부 컬럼이 제외되어야 한다.
	tagKeys, err := c.ListTagKeys(ctx, "", "cpu")
	require.NoError(t, err)
	assert.Equal(t, []string{"host", "region"}, tagKeys)

	// D3 — 태그 값. 여기서는 내부 컬럼 필터가 걸리지 않는다(값은 밑줄로
	// 시작할 수 있는 사용자 데이터다).
	tagValues, err := c.ListTagValues(ctx, "explicit", "cpu", "region")
	require.NoError(t, err)
	assert.Equal(t, []string{"kr", "jp"}, tagValues)

	// D4 — 필드 키.
	fieldKeys, err := c.ListFieldKeys(ctx, "", "cpu")
	require.NoError(t, err)
	assert.Equal(t, []string{"usage", "idle"}, fieldKeys)

	require.Len(t, seen, 3)
	assert.Contains(t, seen[0], `bucket: "metrics"`, "빈 bucket 이 기본 버킷으로 해석되었다")
	assert.Contains(t, seen[1], `bucket: "explicit"`, "명시 bucket 이 그대로 실렸다")
	assert.Contains(t, seen[2], `bucket: "metrics"`)
	for _, q := range seen {
		assert.Contains(t, q, fluxSchemaImport, "schema 패키지 import 가 실렸다")
	}
}

// TestInfluxSchema_V2_어댑터_인자검증_거부 는 어댑터가 쿼리를 보내기 **전에**
// 인자 검증에서 멈추는지 고정한다. 서버가 붙어 있어도 요청이 나가면 안 된다.
func TestInfluxSchema_V2_어댑터_인자검증_거부(t *testing.T) {
	t.Parallel()

	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		_, _ = w.Write([]byte(schemaValueCSV()))
	}))
	defer ts.Close()

	c, err := newInfluxV2Client(InfluxDBConfig{URL: ts.URL, Token: "tok", Org: "org", Bucket: "metrics"})
	require.NoError(t, err)
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	_, err = c.ListTagKeys(ctx, "", "")
	require.Error(t, err)
	_, err = c.ListTagValues(ctx, "", "cpu", "")
	require.Error(t, err)
	_, err = c.ListFieldKeys(ctx, "", "")
	require.Error(t, err)

	assert.Zero(t, calls, "인자 검증 실패는 네트워크 요청을 발생시키지 않는다")
}

// TestInfluxSchema_V2_어댑터_질의오류_전파 는 서버 오류가 어댑터별 맥락과 함께
// 감싸여 올라오는지 고정한다. 세 경로가 같은 메시지를 쓰면 어느 디스커버리가
// 실패했는지 로그에서 구분할 수 없다.
func TestInfluxSchema_V2_어댑터_질의오류_전파(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	}))
	defer ts.Close()

	c, err := newInfluxV2Client(InfluxDBConfig{URL: ts.URL, Token: "tok", Org: "org", Bucket: "metrics"})
	require.NoError(t, err)
	defer func() { _ = c.Close() }()

	ctx := context.Background()

	_, err = c.ListTagKeys(ctx, "", "cpu")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list tag keys")

	_, err = c.ListTagValues(ctx, "", "cpu", "region")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list tag values")

	_, err = c.ListFieldKeys(ctx, "", "cpu")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list field keys")
}

// TestInfluxSchema_BuildFluxFieldKeysQuery_인자거부 는 D4 쿼리 생성기의 검증
// 분기를 고정한다. D2 · D3 는 이미 TestInfluxSchema_필수인자_검증 이 덮는다.
func TestInfluxSchema_BuildFluxFieldKeysQuery_인자거부(t *testing.T) {
	t.Parallel()

	// 빈 bucket 은 거부 대상이 아니다 — validateSchemaArgs 가 건너뛰고
	// 어댑터의 resolveSchemaBucket 이 기본 버킷으로 채운다.
	_, err := buildFluxFieldKeysQuery("", "cpu")
	require.NoError(t, err)

	_, err = buildFluxFieldKeysQuery("metrics", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "measurement")

	// bucket 이 비어 있지 않으면 식별자 검증을 받는다. 따옴표는 거부 대상이
	// 아니라 escapeFluxStringLiteral 의 몫이고, 실제 거부 대상은 제어문자다.
	_, err = buildFluxFieldKeysQuery("bad\nbucket", "cpu")
	require.ErrorIs(t, err, ErrUnescapableIdentifier)
	assert.Contains(t, err.Error(), "bucket")
}
