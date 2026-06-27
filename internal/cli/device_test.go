package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- 테스트 헬퍼 ---

// setupDeviceTest 는 mock 서버와 device 커맨드 루트를 세팅하는 헬퍼이다.
// confirmFn 은 기본적으로 true(삭제 승인)를 반환한다.
func setupDeviceTest(t *testing.T, handler http.HandlerFunc) (*bytes.Buffer, *cobra.Command, func()) {
	t.Helper()
	return setupDeviceTestWithConfirm(t, handler, func(string, io.Reader) bool { return true })
}

// setupDeviceTestWithConfirm 은 confirmFn 을 커스텀할 수 있는 세팅 헬퍼이다.
func setupDeviceTestWithConfirm(t *testing.T, handler http.HandlerFunc, confirmFn func(string, io.Reader) bool) (*bytes.Buffer, *cobra.Command, func()) {
	t.Helper()

	server := httptest.NewServer(handler)

	client := NewClient(server.URL, "test-token", 5*time.Second, false)
	clientPtr := &client

	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("format", "table", "출력 형식")
	rootCmd.AddCommand(newDeviceCmd(clientPtr, confirmFn))

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	return &buf, rootCmd, server.Close
}

// deviceEnvelope 은 디바이스 API 응답 엔벨로프를 생성한다.
func deviceEnvelope(data any) []byte {
	resp := map[string]any{"success": true, "data": data}
	b, _ := json.Marshal(resp)
	return b
}

const testDeviceUUID = "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"

// --- device list ---

func TestDeviceList(t *testing.T) {
	devices := []map[string]any{
		{
			"id":         testDeviceUUID,
			"name":       "거실 에어컨",
			"type":       "indoor",
			"protocol":   "nasa",
			"agent_name": "lg_icp01",
			"online":     true,
		},
		{
			"id":         "b58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
			"name":       "주방 에어컨",
			"type":       "indoor",
			"protocol":   "nasa",
			"agent_name": "lg_icp01",
			"online":     false,
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/devices", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write(deviceEnvelope(devices))
	})

	buf, cmd, cleanup := setupDeviceTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"device", "list"})
	require.NoError(t, cmd.Execute())

	out := buf.String()
	assert.Contains(t, out, "ID")
	assert.Contains(t, out, "PROTOCOL")
	assert.Contains(t, out, "거실 에어컨")
	assert.Contains(t, out, "yes")
	assert.Contains(t, out, "no")
}

func TestDeviceList_Filters(t *testing.T) {
	var gotQuery url.Values

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/devices", r.URL.Path)
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.Write(deviceEnvelope([]map[string]any{}))
	})

	buf, cmd, cleanup := setupDeviceTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"device", "list",
		"--protocol", "nasa",
		"--agent", "lg_icp01",
		"--type", "indoor",
		"--group", "1f",
		"--tag", "hvac",
		"--tag", "floor1",
		"--online",
	})
	require.NoError(t, cmd.Execute())
	_ = buf

	assert.Equal(t, "nasa", gotQuery.Get("protocol"))
	assert.Equal(t, "lg_icp01", gotQuery.Get("agent"))
	assert.Equal(t, "indoor", gotQuery.Get("type"))
	assert.Equal(t, "1f", gotQuery.Get("group"))
	assert.Equal(t, "hvac,floor1", gotQuery.Get("tags"))
	assert.Equal(t, "true", gotQuery.Get("online"))
}

func TestDeviceList_OnlineNotSetOmitsFilter(t *testing.T) {
	var gotQuery url.Values

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.Write(deviceEnvelope([]map[string]any{}))
	})

	_, cmd, cleanup := setupDeviceTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"device", "list"})
	require.NoError(t, cmd.Execute())

	_, hasOnline := gotQuery["online"]
	assert.False(t, hasOnline, "--online 미지정 시 online 쿼리가 없어야 합니다")
}

// --- device get ---

func TestDeviceGet_UUID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/devices/"+testDeviceUUID, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write(deviceEnvelope(map[string]any{
			"id":   testDeviceUUID,
			"name": "거실 에어컨",
			"type": "indoor",
		}))
	})

	buf, cmd, cleanup := setupDeviceTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"device", "get", testDeviceUUID})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "거실 에어컨")
}

func TestDeviceGet_TwoArgs_Composite(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/devices/lg_icp01/indoor1", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write(deviceEnvelope(map[string]any{"id": testDeviceUUID, "name": "indoor1"}))
	})

	buf, cmd, cleanup := setupDeviceTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"device", "get", "lg_icp01", "indoor1"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "indoor1")
}

func TestDeviceGet_SlashRef_SplitsToTwoSegments(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/devices/lg_icp01/indoor1", r.URL.Path,
			"agent/name 단일 인자는 2-세그먼트 경로로 분해되어야 합니다")
		w.Header().Set("Content-Type", "application/json")
		w.Write(deviceEnvelope(map[string]any{"id": testDeviceUUID}))
	})

	_, cmd, cleanup := setupDeviceTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"device", "get", "lg_icp01/indoor1"})
	require.NoError(t, cmd.Execute())
}

func TestDeviceGet_NoArgs(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("인자 없이 get 호출 시 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupDeviceTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"device", "get"})
	require.Error(t, cmd.Execute())
}

// --- device resolve ---

func TestDeviceResolve_AgentOnly_BulkList(t *testing.T) {
	var gotPath string
	var gotQuery url.Values

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.Write(deviceEnvelope([]map[string]any{
			{"id": testDeviceUUID, "name": "indoor1", "agent_name": "lg_icp01"},
		}))
	})

	buf, cmd, cleanup := setupDeviceTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"device", "resolve", "lg_icp01"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "/api/v1/devices", gotPath)
	assert.Equal(t, "lg_icp01", gotQuery.Get("agent"))
	assert.Contains(t, buf.String(), "indoor1")
}

func TestDeviceResolve_AgentAndName_SingleDevice(t *testing.T) {
	var gotPath string
	var gotQuery url.Values

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.Write(deviceEnvelope(map[string]any{"id": testDeviceUUID, "name": "indoor1"}))
	})

	buf, cmd, cleanup := setupDeviceTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"device", "resolve", "lg_icp01", "indoor1"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "/api/v1/devices:resolve", gotPath)
	assert.Equal(t, "lg_icp01", gotQuery.Get("agent"))
	assert.Equal(t, "indoor1", gotQuery.Get("name"))
	assert.Contains(t, buf.String(), "indoor1")
}

// --- device execute ---

func TestDeviceExecute_CommandWithParams(t *testing.T) {
	var gotPath string
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		gotPath = r.URL.Path
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(deviceEnvelope(map[string]any{"ok": true}))
	})

	buf, cmd, cleanup := setupDeviceTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"--format", "json",
		"device", "execute", testDeviceUUID,
		"set_temperature", "value=24", "unit=celsius",
	})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "/api/v1/devices/"+testDeviceUUID+"/execute", gotPath)
	assert.Equal(t, "set_temperature", gotBody["command"])

	params, ok := gotBody["params"].(map[string]any)
	require.True(t, ok, "params 가 객체여야 합니다")
	// value=24 는 정수로 파싱됨
	assert.Equal(t, float64(24), params["value"])
	assert.Equal(t, "celsius", params["unit"])
	assert.Contains(t, buf.String(), "ok")
}

func TestDeviceExecute_CommandOnly(t *testing.T) {
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(deviceEnvelope(map[string]any{"ok": true}))
	})

	_, cmd, cleanup := setupDeviceTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"device", "execute", testDeviceUUID, "power_on"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "power_on", gotBody["command"])
	_, hasParams := gotBody["params"]
	assert.False(t, hasParams, "파라미터 없는 커맨드는 params 키가 없어야 합니다")
}

func TestDeviceExecute_RawJSON(t *testing.T) {
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(deviceEnvelope(map[string]any{"ok": true}))
	})

	_, cmd, cleanup := setupDeviceTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"device", "execute", testDeviceUUID,
		"--json", `{"command":"set_mode","params":{"mode":"cool"}}`,
	})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "set_mode", gotBody["command"])
	params, ok := gotBody["params"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "cool", params["mode"])
}

func TestDeviceExecute_InvalidParamFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("잘못된 파라미터 형식이면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupDeviceTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"device", "execute", testDeviceUUID, "cmd", "badparam"})
	require.Error(t, cmd.Execute())
}

func TestDeviceExecute_NoCommand(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("커맨드 미지정 시 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupDeviceTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"device", "execute", testDeviceUUID})
	require.Error(t, cmd.Execute())
}

// --- device metadata set ---

func TestDeviceMetadataSet(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(deviceEnvelope(gotBody))
	})

	buf, cmd, cleanup := setupDeviceTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"device", "metadata", "set", testDeviceUUID,
		"--name", "거실 에어컨",
		"--location", "1층 거실",
		"--group", "living",
		"--tag", "hvac",
		"--tag", "floor1",
		"--label", "vendor=lg",
	})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodPut, gotMethod)
	assert.Equal(t, "/api/v1/devices/"+testDeviceUUID+"/metadata", gotPath)
	assert.Equal(t, "거실 에어컨", gotBody["name"])
	assert.Equal(t, "1층 거실", gotBody["location"])
	assert.Equal(t, "living", gotBody["group"])

	tags, ok := gotBody["tags"].([]any)
	require.True(t, ok)
	assert.ElementsMatch(t, []any{"hvac", "floor1"}, tags)

	labels, ok := gotBody["labels"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "lg", labels["vendor"])

	assert.Contains(t, buf.String(), "설정되었습니다")
}

func TestDeviceMetadataSet_PartialOmitsUnsetFields(t *testing.T) {
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(deviceEnvelope(gotBody))
	})

	_, cmd, cleanup := setupDeviceTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"device", "metadata", "set", testDeviceUUID, "--name", "온도계"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "온도계", gotBody["name"])
	_, hasLocation := gotBody["location"]
	assert.False(t, hasLocation, "미지정 필드는 본문에서 제외되어야 합니다")
	_, hasTags := gotBody["tags"]
	assert.False(t, hasTags)
}

func TestDeviceMetadataSet_NoFlags(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("설정 플래그가 없으면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupDeviceTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"device", "metadata", "set", testDeviceUUID})
	require.Error(t, cmd.Execute())
}

func TestDeviceMetadataSet_InvalidLabel(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("잘못된 라벨 형식이면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupDeviceTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"device", "metadata", "set", testDeviceUUID, "--label", "badlabel"})
	require.Error(t, cmd.Execute())
}

// --- device metadata delete ---

func TestDeviceMetadataDelete_Confirmed(t *testing.T) {
	var gotPath, gotMethod string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	})

	buf, cmd, cleanup := setupDeviceTestWithConfirm(t, handler, func(string, io.Reader) bool { return true })
	defer cleanup()

	cmd.SetArgs([]string{"device", "metadata", "delete", testDeviceUUID})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodDelete, gotMethod)
	assert.Equal(t, "/api/v1/devices/"+testDeviceUUID+"/metadata", gotPath)
	assert.Contains(t, buf.String(), "삭제되었습니다")
}

func TestDeviceMetadataDelete_Cancelled(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("확인이 거부되면 서버를 호출하면 안 됩니다")
	})

	buf, cmd, cleanup := setupDeviceTestWithConfirm(t, handler, func(string, io.Reader) bool { return false })
	defer cleanup()

	cmd.SetArgs([]string{"device", "metadata", "delete", testDeviceUUID})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "취소되었습니다")
}

func TestDeviceMetadataDelete_YesSkipsConfirm(t *testing.T) {
	var called bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	// confirmFn 이 false 여도 --yes 면 삭제가 진행되어야 한다.
	_, cmd, cleanup := setupDeviceTestWithConfirm(t, handler, func(string, io.Reader) bool { return false })
	defer cleanup()

	cmd.SetArgs([]string{"device", "metadata", "delete", testDeviceUUID, "--yes"})
	require.NoError(t, cmd.Execute())
	assert.True(t, called, "--yes 플래그는 확인 없이 삭제를 진행해야 합니다")
}

// --- device history ---

func TestDeviceHistory(t *testing.T) {
	var gotPath string
	var gotQuery url.Values

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.Write(deviceEnvelope(map[string]any{
			"device_id": testDeviceUUID,
			"count":     float64(2),
			"entries": []map[string]any{
				{"timestamp": float64(1700000000000), "online": true, "last_seen": float64(1700000000000)},
				{"timestamp": float64(1700000001000), "online": false, "last_seen": float64(1700000001000)},
			},
		}))
	})

	buf, cmd, cleanup := setupDeviceTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"device", "history", testDeviceUUID, "--limit", "50"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, "/api/v1/devices/"+testDeviceUUID+"/history", gotPath)
	assert.Equal(t, "50", gotQuery.Get("limit"))

	out := buf.String()
	assert.Contains(t, out, testDeviceUUID)
	assert.Contains(t, out, "TIMESTAMP")
	assert.Contains(t, out, "1700000000000")
}

func TestDeviceHistory_NoLimitOmitsQuery(t *testing.T) {
	var gotQuery url.Values

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.Write(deviceEnvelope(map[string]any{
			"device_id": testDeviceUUID,
			"count":     float64(0),
			"entries":   []map[string]any{},
		}))
	})

	_, cmd, cleanup := setupDeviceTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"device", "history", testDeviceUUID})
	require.NoError(t, cmd.Execute())

	_, hasLimit := gotQuery["limit"]
	assert.False(t, hasLimit, "--limit 미지정 시 limit 쿼리가 없어야 합니다")
}

func TestDeviceHistory_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(deviceEnvelope(map[string]any{
			"device_id": testDeviceUUID,
			"count":     float64(1),
			"entries": []map[string]any{
				{"timestamp": float64(1700000000000), "online": true},
			},
		}))
	})

	buf, cmd, cleanup := setupDeviceTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"--format", "json", "device", "history", testDeviceUUID})
	require.NoError(t, cmd.Execute())

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	assert.Equal(t, testDeviceUUID, parsed["device_id"])
}

// --- buildDeviceGetPath 단위 테스트 ---

func TestBuildDeviceGetPath(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"uuid", []string{testDeviceUUID}, "/api/v1/devices/" + testDeviceUUID, false},
		{"slash ref", []string{"agent1/dev1"}, "/api/v1/devices/agent1/dev1", false},
		{"two args", []string{"agent1", "dev1"}, "/api/v1/devices/agent1/dev1", false},
		{"empty single", []string{""}, "", true},
		{"empty agent in two", []string{"", "dev1"}, "", true},
		{"trailing slash ref", []string{"agent1/"}, "", true},
		{"leading slash ref", []string{"/dev1"}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildDeviceGetPath(tt.args)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
