package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testTokenID = "tok-01"

// --- remote token create ---

func TestRemoteTokenCreate(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"id":         testTokenID,
			"token":      "secret-raw-token-value",
			"label":      "edge-fleet",
			"expires_at": float64(1700000000000),
			"max_uses":   float64(5),
		}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{
		"remote", "token", "create",
		"--label", "edge-fleet", "--expires-in", "24h", "--max-uses", "5",
	})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/api/v1/remote/enrollment-tokens", gotPath)
	assert.Equal(t, "edge-fleet", gotBody["label"])
	assert.Equal(t, "24h", gotBody["expires_in"])
	assert.Equal(t, float64(5), gotBody["max_uses"])

	out := buf.String()
	// 원본 토큰은 1회만 노출되며 명확히 출력되어야 한다.
	assert.Contains(t, out, "secret-raw-token-value")
	assert.Contains(t, out, testTokenID)
	// 1회 노출 경고 문구를 포함해야 한다.
	assert.Contains(t, out, "다시 표시되지 않습니다")
}

func TestRemoteTokenCreate_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{
			"id":    testTokenID,
			"token": "secret-raw-token-value",
		}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"--format", "json", "remote", "token", "create"})
	require.NoError(t, cmd.Execute())

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	assert.Equal(t, testTokenID, parsed["id"])
	assert.Equal(t, "secret-raw-token-value", parsed["token"])
}

func TestRemoteTokenCreate_NoFlagsOmitsFields(t *testing.T) {
	var gotBody map[string]any

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(map[string]any{"id": testTokenID, "token": "raw"}))
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "token", "create"})
	require.NoError(t, cmd.Execute())

	for _, k := range []string{"label", "expires_in", "max_uses"} {
		_, ok := gotBody[k]
		assert.False(t, ok, "플래그 미지정 시 %q 필드는 생략되어야 합니다", k)
	}
}

// --- remote token list ---

func sampleTokens() []map[string]any {
	return []map[string]any{
		{
			"id":         testTokenID,
			"label":      "edge-fleet",
			"created_at": float64(1700000000000),
			"expires_at": float64(1700000100000),
			"max_uses":   float64(5),
			"uses":       float64(2),
			"revoked":    false,
		},
		{
			"id":         "tok-02",
			"created_at": float64(1700000001000),
			"uses":       float64(0),
			"revoked":    true,
		},
	}
}

func TestRemoteTokenList(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/remote/enrollment-tokens", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(sampleTokens()))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "token", "list"})
	require.NoError(t, cmd.Execute())

	out := buf.String()
	for _, h := range []string{"ID", "LABEL", "CREATED_AT", "EXPIRES_AT", "USES", "MAX_USES", "REVOKED"} {
		assert.Contains(t, out, h)
	}
	assert.Contains(t, out, testTokenID)
	assert.Contains(t, out, "edge-fleet")
	// created_at 은 epoch ms 정수 문자열로 표시(과학표기 금지).
	assert.Contains(t, out, "1700000000000")
	// revoked bool 은 yes/no 로 렌더링.
	assert.Contains(t, out, "yes")
	assert.Contains(t, out, "no")
}

func TestRemoteTokenList_JSONFormat(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope(sampleTokens()))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"--format", "json", "remote", "token", "list"})
	require.NoError(t, cmd.Execute())

	var parsed []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
	require.Len(t, parsed, 2)
	assert.Equal(t, testTokenID, parsed[0]["id"])
}

func TestRemoteTokenList_NullableFields(t *testing.T) {
	// expires_at/max_uses 가 없는(null) 항목도 안전하게 렌더링되어야 한다.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(remoteEnvelope([]map[string]any{
			{
				"id":         "tok-03",
				"created_at": float64(1700000002000),
				"uses":       float64(0),
				"revoked":    false,
			},
		}))
	})

	buf, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "token", "list"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "tok-03")
}

// --- remote token revoke ---

func TestRemoteTokenRevoke_Confirmed(t *testing.T) {
	var gotPath, gotMethod string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	})

	buf, cmd, cleanup := setupRemoteTestWithConfirm(t, handler, func(string, io.Reader) bool { return true })
	defer cleanup()

	cmd.SetArgs([]string{"remote", "token", "revoke", testTokenID})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, http.MethodDelete, gotMethod)
	assert.Equal(t, "/api/v1/remote/enrollment-tokens/"+testTokenID, gotPath)
	assert.Contains(t, buf.String(), "폐기")
}

func TestRemoteTokenRevoke_Cancelled(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("확인이 거부되면 서버를 호출하면 안 됩니다")
	})

	buf, cmd, cleanup := setupRemoteTestWithConfirm(t, handler, func(string, io.Reader) bool { return false })
	defer cleanup()

	cmd.SetArgs([]string{"remote", "token", "revoke", testTokenID})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "취소되었습니다")
}

func TestRemoteTokenRevoke_YesSkipsConfirm(t *testing.T) {
	var called bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	_, cmd, cleanup := setupRemoteTestWithConfirm(t, handler, func(string, io.Reader) bool { return false })
	defer cleanup()

	cmd.SetArgs([]string{"remote", "token", "revoke", testTokenID, "--yes"})
	require.NoError(t, cmd.Execute())
	assert.True(t, called, "--yes 플래그는 확인 없이 폐기를 진행해야 합니다")
}

func TestRemoteTokenRevoke_EmptyID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("빈 토큰 ID 면 서버를 호출하면 안 됩니다")
	})

	_, cmd, cleanup := setupRemoteTest(t, handler)
	defer cleanup()

	cmd.SetArgs([]string{"remote", "token", "revoke", "  ", "--yes"})
	require.Error(t, cmd.Execute())
}

// --- buildTokenCreateBody 단위 테스트 ---

func TestBuildTokenCreateBody(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantKeys  []string
		wantNoKey []string
	}{
		{
			name:      "all flags",
			args:      []string{"--label", "x", "--expires-in", "24h", "--max-uses", "5"},
			wantKeys:  []string{"label", "expires_in", "max_uses"},
			wantNoKey: nil,
		},
		{
			name:      "label only",
			args:      []string{"--label", "x"},
			wantKeys:  []string{"label"},
			wantNoKey: []string{"expires_in", "max_uses"},
		},
		{
			name:      "no flags yields empty body",
			args:      nil,
			wantKeys:  nil,
			wantNoKey: []string{"label", "expires_in", "max_uses"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRemoteTokenCreateCmdForTest()
			require.NoError(t, cmd.ParseFlags(tt.args))

			body := buildTokenCreateBodyFromCmd(cmd)

			for _, k := range tt.wantKeys {
				_, ok := body[k]
				assert.True(t, ok, "키 %q 가 본문에 있어야 합니다", k)
			}
			for _, k := range tt.wantNoKey {
				_, ok := body[k]
				assert.False(t, ok, "키 %q 는 본문에 없어야 합니다", k)
			}
		})
	}
}
