package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- 테스트 헬퍼 ---

// setupStoreTest 는 mock 서버와 store 커맨드를 세팅한다.
// confirmFn 은 항상 true (리셋 허용) 로 기본 설정한다.
func setupStoreTest(t *testing.T, handler http.HandlerFunc) (*bytes.Buffer, *cobra.Command, func()) {
	t.Helper()
	return setupStoreTestWithConfirm(t, handler, func(string, io.Reader) bool { return true })
}

// setupStoreTestWithConfirm 은 confirmFn 을 커스텀할 수 있는 세팅 헬퍼이다.
func setupStoreTestWithConfirm(t *testing.T, handler http.HandlerFunc, confirmFn func(string, io.Reader) bool) (*bytes.Buffer, *cobra.Command, func()) {
	t.Helper()

	server := httptest.NewServer(handler)

	client := NewClient(server.URL, "test-token", 5*time.Second, false)
	clientPtr := &client

	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("format", "table", "출력 형식")
	rootCmd.AddCommand(newStoreCmd(clientPtr, confirmFn))

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	return &buf, rootCmd, server.Close
}

// decodeBody 는 요청 바디를 map 으로 디코딩한다.
func decodeBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("요청 바디 디코딩 실패: %v", err)
	}
	return body
}

// --- store keys 테스트 ---

func TestStoreKeys(t *testing.T) {
	keysData := map[string]any{
		"count": 2,
		"keys": []map[string]any{
			{
				"key":          "indoor:1:room_temp",
				"registration": "manual",
				"data_type":    "float",
				"metric_type":  "temperature",
				"tags":         map[string]any{"room": "1"},
			},
			{
				"key":          "outdoor:humidity",
				"registration": "auto",
				"data_type":    "float",
				"metric_type":  "humidity",
				"tags":         map[string]any{},
			},
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/store/sensor-store/keys", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(keysData))
	})

	buf, cmd, cleanup := setupStoreTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"store", "keys", "sensor-store"})
	require.NoError(t, cmd.Execute())

	output := buf.String()
	assert.Contains(t, output, "KEY")
	assert.Contains(t, output, "indoor:1:room_temp")
	assert.Contains(t, output, "outdoor:humidity")
	assert.Contains(t, output, "temperature")
	// 태그 맵 포매팅 검증.
	assert.Contains(t, output, "room=1")
}

func TestStoreKeys_FilterClientSide(t *testing.T) {
	keysData := map[string]any{
		"count": 2,
		"keys": []map[string]any{
			{"key": "indoor:1:room_temp", "registration": "manual", "data_type": "float", "metric_type": "temperature", "tags": map[string]any{}},
			{"key": "outdoor:humidity", "registration": "auto", "data_type": "float", "metric_type": "humidity", "tags": map[string]any{}},
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(keysData))
	})

	buf, cmd, cleanup := setupStoreTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"store", "keys", "sensor-store", "--filter", "indoor"})
	require.NoError(t, cmd.Execute())

	output := buf.String()
	assert.Contains(t, output, "indoor:1:room_temp", "필터에 일치하는 키는 표시되어야 합니다")
	assert.NotContains(t, output, "outdoor:humidity", "필터에 불일치하는 키는 제외되어야 합니다")
}

func TestStoreKeys_PaginationClientSide(t *testing.T) {
	keys := make([]map[string]any, 0, 5)
	for _, name := range []string{"k1", "k2", "k3", "k4", "k5"} {
		keys = append(keys, map[string]any{
			"key": name, "registration": "auto", "data_type": "int", "metric_type": "unknown", "tags": map[string]any{},
		})
	}
	keysData := map[string]any{"count": 5, "keys": keys}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(keysData))
	})

	buf, cmd, cleanup := setupStoreTest(t, handler)
	defer cleanup()

	// page=2, size=2 → k3, k4 만.
	cmd.SetArgs([]string{"store", "keys", "sensor-store", "--page", "2", "--size", "2"})
	require.NoError(t, cmd.Execute())

	output := buf.String()
	assert.Contains(t, output, "k3")
	assert.Contains(t, output, "k4")
	assert.NotContains(t, output, "k1")
	assert.NotContains(t, output, "k5")
}

func TestStoreKeys_JSONFormat(t *testing.T) {
	keysData := map[string]any{
		"count": 1,
		"keys": []map[string]any{
			{"key": "k1", "registration": "auto", "data_type": "int", "metric_type": "unknown", "tags": map[string]any{}},
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(keysData))
	})

	buf, cmd, cleanup := setupStoreTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"store", "keys", "sensor-store", "--format", "json"})
	require.NoError(t, cmd.Execute())

	var got []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	require.Len(t, got, 1)
	assert.Equal(t, "k1", got[0]["key"])
}

// --- store tags 테스트 ---

func TestStoreTags(t *testing.T) {
	tagsData := map[string]any{
		"pairs": []map[string]any{
			{"key": "room", "values": []string{"1", "2"}},
			{"key": "type", "values": []string{"temperature"}},
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/store/sensor-store/tags", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(tagsData))
	})

	buf, cmd, cleanup := setupStoreTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"store", "tags", "sensor-store"})
	require.NoError(t, cmd.Execute())

	output := buf.String()
	assert.Contains(t, output, "room")
	assert.Contains(t, output, "1, 2")
	assert.Contains(t, output, "type")
	assert.Contains(t, output, "temperature")
}

func TestStoreTags_Empty(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"pairs": []map[string]any{}}))
	})

	buf, cmd, cleanup := setupStoreTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"store", "tags", "sensor-store"})
	require.NoError(t, cmd.Execute())

	assert.Contains(t, buf.String(), "태그가 없습니다")
}

// --- store query 테스트 ---

func TestStoreQuery_FlagMapping(t *testing.T) {
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/store/sensor-store/query", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		gotBody = decodeBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{
			"entries":   []map[string]any{{"timestamp": float64(1700000000000), "value": float64(23)}},
			"count":     1,
			"truncated": false,
		}))
	})

	buf, cmd, cleanup := setupStoreTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"store", "query", "sensor-store",
		"--key", "indoor:1:room_temp",
		"--mode", "last_n",
		"--count", "10",
		"--namespace", "default",
		"--metric-type", "temperature",
		"--tag", "room=1",
		"--format", "json",
	})
	require.NoError(t, cmd.Execute())

	// 바디 매핑 검증.
	require.NotNil(t, gotBody)
	assert.Equal(t, "indoor:1:room_temp", gotBody["key"])
	assert.Equal(t, "last_n", gotBody["mode"])
	assert.Equal(t, float64(10), gotBody["count"])
	assert.Equal(t, "default", gotBody["namespace"])
	assert.Equal(t, "temperature", gotBody["metric_type"])
	tags, ok := gotBody["tags"].(map[string]any)
	require.True(t, ok, "tags 는 객체여야 합니다")
	assert.Equal(t, "1", tags["room"])

	// 응답 출력 검증.
	assert.Contains(t, buf.String(), "entries")
}

func TestStoreQuery_TimeRangeMapping(t *testing.T) {
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody = decodeBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"entries": []any{}, "count": 0, "truncated": false}))
	})

	buf, cmd, cleanup := setupStoreTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"store", "query", "sensor-store",
		"--key", "k1",
		"--mode", "time_range",
		"--start-ms", "1700000000000",
		"--end-ms", "1700000060000",
	})
	require.NoError(t, cmd.Execute())
	_ = buf

	require.NotNil(t, gotBody)
	assert.Equal(t, "time_range", gotBody["mode"])
	assert.Equal(t, float64(1700000000000), gotBody["start_ms"])
	assert.Equal(t, float64(1700000060000), gotBody["end_ms"])
	// 미지정 필드는 바디에 포함되지 않아야 한다 (omitempty 의도).
	_, hasCount := gotBody["count"]
	assert.False(t, hasCount, "count 미지정 시 바디에 없어야 합니다")
}

func TestStoreQuery_RawJSONBody(t *testing.T) {
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody = decodeBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"entries": []any{}, "count": 0, "truncated": false}))
	})

	buf, cmd, cleanup := setupStoreTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"store", "query", "sensor-store",
		"--json", `{"key":"raw:key","mode":"latest"}`,
	})
	require.NoError(t, cmd.Execute())
	_ = buf

	require.NotNil(t, gotBody)
	assert.Equal(t, "raw:key", gotBody["key"])
	assert.Equal(t, "latest", gotBody["mode"])
}

func TestStoreQuery_MissingKeyError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("--key 누락 시 서버에 요청하지 않아야 합니다")
	})

	_, cmd, cleanup := setupStoreTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"store", "query", "sensor-store", "--mode", "latest"})
	err := cmd.Execute()
	require.Error(t, err, "--key 누락은 에러여야 합니다")
	assert.Contains(t, err.Error(), "--key")
}

// --- store meta 테스트 ---

func TestStoreMeta(t *testing.T) {
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/store/sensor-store/keys/indoor:1:room_temp/meta", r.URL.Path)
		assert.Equal(t, http.MethodPut, r.Method)
		gotBody = decodeBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{
			"key":         "indoor:1:room_temp",
			"metric_type": "temperature",
			"tags":        map[string]any{"room": "1", "floor": "2"},
		}))
	})

	buf, cmd, cleanup := setupStoreTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"store", "meta", "sensor-store", "indoor:1:room_temp",
		"--metric-type", "temperature",
		"--tag", "room=1",
		"--tag", "floor=2",
	})
	require.NoError(t, cmd.Execute())

	// 바디 구조 검증: {metric_type, tags{...}}.
	require.NotNil(t, gotBody)
	assert.Equal(t, "temperature", gotBody["metric_type"])
	tags, ok := gotBody["tags"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "1", tags["room"])
	assert.Equal(t, "2", tags["floor"])

	assert.Contains(t, buf.String(), "temperature")
}

func TestStoreMeta_EmptyTagsSendsEmptyMap(t *testing.T) {
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody = decodeBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"key": "k1", "metric_type": "unknown", "tags": map[string]any{}}))
	})

	_, cmd, cleanup := setupStoreTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"store", "meta", "sensor-store", "k1"})
	require.NoError(t, cmd.Execute())

	require.NotNil(t, gotBody)
	// tags 는 항상 객체여야 한다 (null 아님, replace 시맨틱).
	tags, ok := gotBody["tags"].(map[string]any)
	require.True(t, ok, "tags 는 빈 객체로라도 전송되어야 합니다")
	assert.Empty(t, tags)
}

func TestStoreMeta_InvalidTagError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("잘못된 태그 형식 시 서버에 요청하지 않아야 합니다")
	})

	_, cmd, cleanup := setupStoreTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"store", "meta", "sensor-store", "k1", "--tag", "noequalsign"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key=value")
}

// --- store reset 테스트 ---

func TestStoreReset_SingleKey(t *testing.T) {
	var deletePath string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		deletePath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"action": "history_cleared", "key": "indoor:1:room_temp"}))
	})

	buf, cmd, cleanup := setupStoreTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"store", "reset", "sensor-store", "indoor:1:room_temp"})
	require.NoError(t, cmd.Execute())

	// 단일 키 경로 검증 (PathEscape: 콜론은 그대로 유지됨).
	assert.Equal(t, "/api/v1/store/sensor-store/keys/indoor:1:room_temp", deletePath)
	assert.Contains(t, buf.String(), "리셋")
}

func TestStoreReset_All(t *testing.T) {
	var deletePath string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		deletePath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"history_cleared": 3, "entries_deleted": 2}))
	})

	buf, cmd, cleanup := setupStoreTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"store", "reset", "sensor-store", "--all"})
	require.NoError(t, cmd.Execute())

	// 벌크 경로 검증 (키 세그먼트 없음).
	assert.Equal(t, "/api/v1/store/sensor-store/keys", deletePath)
	assert.Contains(t, buf.String(), "모든 키")
}

func TestStoreReset_WithNamespace(t *testing.T) {
	var rawQuery string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"action": "entry_deleted", "key": "k1"}))
	})

	_, cmd, cleanup := setupStoreTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"store", "reset", "sensor-store", "k1", "--namespace", "prod"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "namespace=prod", rawQuery)
}

func TestStoreReset_Cancelled(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("리셋 취소 시 서버에 요청하지 않아야 합니다")
	})

	confirmFn := func(string, io.Reader) bool { return false }
	buf, cmd, cleanup := setupStoreTestWithConfirm(t, handler, confirmFn)
	defer cleanup()

	cmd.SetArgs([]string{"store", "reset", "sensor-store", "k1"})
	require.NoError(t, cmd.Execute(), "리셋 취소는 에러가 아닙니다")

	assert.Contains(t, buf.String(), "취소")
}

func TestStoreReset_WithYesFlag(t *testing.T) {
	var requested bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = true
		w.Header().Set("Content-Type", "application/json")
		w.Write(apiEnvelope(map[string]any{"action": "history_cleared", "key": "k1"}))
	})

	// confirmFn 이 false 여도 --yes 면 호출되지 않아야 한다.
	confirmFn := func(string, io.Reader) bool {
		t.Fatal("--yes 사용 시 confirmFn 이 호출되면 안 됩니다")
		return false
	}
	_, cmd, cleanup := setupStoreTestWithConfirm(t, handler, confirmFn)
	defer cleanup()

	cmd.SetArgs([]string{"store", "reset", "sensor-store", "k1", "--yes"})
	require.NoError(t, cmd.Execute())

	assert.True(t, requested, "--yes 시 확인 없이 삭제 요청을 보내야 합니다")
}

func TestStoreReset_KeyAndAllMutuallyExclusive(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("상호 배타 위반 시 서버에 요청하지 않아야 합니다")
	})

	_, cmd, cleanup := setupStoreTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"store", "reset", "sensor-store", "k1", "--all"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "동시에 사용할 수 없습니다")
}

func TestStoreReset_NoKeyNoAllError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("키/--all 둘 다 없을 때 서버에 요청하지 않아야 합니다")
	})

	_, cmd, cleanup := setupStoreTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"store", "reset", "sensor-store"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--all")
}

// --- 헬퍼 단위 테스트 ---

func TestParseTagKV(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    map[string]string
		wantErr bool
	}{
		{"빈 입력", nil, nil, false},
		{"단일", []string{"room=1"}, map[string]string{"room": "1"}, false},
		{"다중", []string{"room=1", "floor=2"}, map[string]string{"room": "1", "floor": "2"}, false},
		{"value 내 등호", []string{"expr=a=b"}, map[string]string{"expr": "a=b"}, false},
		{"등호 없음", []string{"invalid"}, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTagKV(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPaginateKeys(t *testing.T) {
	mk := func(names ...string) []map[string]any {
		out := make([]map[string]any, 0, len(names))
		for _, n := range names {
			out = append(out, map[string]any{"key": n})
		}
		return out
	}
	all := mk("a", "b", "c", "d", "e")

	tests := []struct {
		name      string
		page, sz  int
		wantNames []string
	}{
		{"비활성(전체)", 0, 0, []string{"a", "b", "c", "d", "e"}},
		{"첫 페이지", 1, 2, []string{"a", "b"}},
		{"둘째 페이지", 2, 2, []string{"c", "d"}},
		{"마지막 부분 페이지", 3, 2, []string{"e"}},
		{"범위 초과", 4, 2, []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paginateKeys(all, tt.page, tt.sz)
			names := make([]string, 0, len(got))
			for _, m := range got {
				names = append(names, m["key"].(string))
			}
			assert.Equal(t, tt.wantNames, names)
		})
	}
}
