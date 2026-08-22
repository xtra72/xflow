// @spec SPEC-TSDB-002 §2.10 (U10) — 스키마 디스커버리 D2~D4.
package system

import (
	"context"
	"errors"
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
