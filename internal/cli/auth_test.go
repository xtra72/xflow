package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturedRequest 는 mock 서버가 수신한 요청을 기록하는 구조이다.
type capturedRequest struct {
	method string
	path   string
	auth   string
	body   map[string]any
}

// setupAuthTest 는 mock 서버와 auth 커맨드, 임시 config 경로를 세팅하는 헬퍼이다.
// handler 는 요청을 기록한 뒤 응답을 작성한다.
// 반환된 configPath 는 임시 디렉토리의 config.yaml 경로이며 --config 플래그로 주입된다.
func setupAuthTest(t *testing.T, token string, handler func(*capturedRequest, http.ResponseWriter, *http.Request)) (*bytes.Buffer, *cobra.Command, string, func()) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured := &capturedRequest{
			method: r.Method,
			path:   r.URL.Path,
			auth:   r.Header.Get("Authorization"),
		}
		if r.Body != nil {
			b, _ := io.ReadAll(r.Body)
			if len(b) > 0 {
				_ = json.Unmarshal(b, &captured.body)
			}
		}
		handler(captured, w, r)
	}))

	client := NewClient(server.URL, token, 5*time.Second, false)
	clientPtr := &client

	confirmFn := func(prompt string, reader io.Reader) bool { return true }

	// 임시 config 경로 (테스트 격리)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")

	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("format", "table", "출력 형식")
	rootCmd.PersistentFlags().String("config", configPath, "설정 파일 경로")
	rootCmd.AddCommand(newAuthCmd(clientPtr, confirmFn))

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)

	return &buf, rootCmd, configPath, server.Close
}

// writeAuthEnvelope 는 성공 응답 엔벨로프를 작성하는 헬퍼이다.
func writeAuthEnvelope(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]any{"success": true, "data": data}
	_ = json.NewEncoder(w).Encode(resp)
}

// readConfigToken 은 config 파일에서 auth.token 값을 읽는 헬퍼이다.
func readConfigToken(t *testing.T, configPath string) string {
	t.Helper()
	v := viper.New()
	v.SetConfigFile(configPath)
	if err := v.ReadInConfig(); err != nil {
		return ""
	}
	return v.GetString("auth.token")
}

// stubSecret 은 authReadSecret 을 주어진 응답 시퀀스로 교체하고 복원 함수를 반환한다.
func stubSecret(t *testing.T, responses ...string) {
	t.Helper()
	orig := authReadSecret
	idx := 0
	authReadSecret = func(prompt string) (string, error) {
		if idx >= len(responses) {
			return "", nil
		}
		v := responses[idx]
		idx++
		return v, nil
	}
	t.Cleanup(func() { authReadSecret = orig })
}

func TestAuthLogin(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		secrets     []string
		respToken   string
		wantPath    string
		wantUser    string
		wantPass    string
		wantErr     bool
		save        bool // --save 지정 여부 (config 저장 기대치)
		wantSession string
		checkBody   bool
	}{
		{
			name:        "플래그로 로그인 성공 및 세션 메모리 적용",
			args:        []string{"auth", "login", "--username", "admin", "--password", "secret"},
			respToken:   "issued-access-token",
			wantPath:    "/api/v1/auth/login",
			wantUser:    "admin",
			wantPass:    "secret",
			wantSession: "issued-access-token",
			checkBody:   true,
		},
		{
			name:        "비밀번호 프롬프트로 로그인 (세션 메모리)",
			args:        []string{"auth", "login", "--username", "operator"},
			secrets:     []string{"prompt-pass"},
			respToken:   "tok-2",
			wantPath:    "/api/v1/auth/login",
			wantUser:    "operator",
			wantPass:    "prompt-pass",
			wantSession: "tok-2",
			checkBody:   true,
		},
		{
			name:        "--save 로그인은 config 에도 저장",
			args:        []string{"auth", "login", "--username", "admin", "--password", "secret", "--save"},
			respToken:   "saved-tok",
			wantPath:    "/api/v1/auth/login",
			wantUser:    "admin",
			wantPass:    "secret",
			save:        true,
			wantSession: "saved-tok",
			checkBody:   true,
		},
		{
			name:      "토큰 없는 응답은 에러",
			args:      []string{"auth", "login", "--username", "admin", "--password", "secret"},
			respToken: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 세션 메모리 토큰 격리
			sessionToken = ""
			defer func() { sessionToken = "" }()

			var captured *capturedRequest
			buf, rootCmd, configPath, closeFn := setupAuthTest(t, "", func(c *capturedRequest, w http.ResponseWriter, r *http.Request) {
				captured = c
				writeAuthEnvelope(w, map[string]any{
					"user": map[string]any{"username": "admin", "role": "admin"},
					"tokens": map[string]any{
						"access_token":  tt.respToken,
						"refresh_token": "refresh-xyz",
						"expires_at":    int64(1234567890),
						"token_type":    "Bearer",
					},
				})
			})
			defer closeFn()

			if len(tt.secrets) > 0 {
				stubSecret(t, tt.secrets...)
			}

			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			require.NotNil(t, captured)
			assert.Equal(t, http.MethodPost, captured.method)
			assert.Equal(t, tt.wantPath, captured.path)

			if tt.checkBody {
				assert.Equal(t, tt.wantUser, captured.body["username"])
				assert.Equal(t, tt.wantPass, captured.body["password"])
			}

			// 토큰이 세션 메모리에 반영되었는지 검증
			assert.Equal(t, tt.wantSession, sessionToken,
				"로그인 후 sessionToken 에 토큰이 보관되어야 합니다")

			// config 저장 여부는 --save 지정 시에만 기대한다.
			if tt.save {
				assert.Equal(t, tt.wantSession, readConfigToken(t, configPath),
					"--save 로그인은 config 에 토큰을 저장해야 합니다")
			} else {
				assert.Equal(t, "", readConfigToken(t, configPath),
					"--save 없는 로그인은 config 를 수정하면 안 됩니다")
			}

			// 비밀번호가 출력에 평문 노출되지 않았는지 검증
			assert.NotContains(t, buf.String(), tt.wantPass)
		})
	}
}

func TestAuthLogoutCallsEndpointAndClearsToken(t *testing.T) {
	var captured *capturedRequest

	// 먼저 config 에 토큰을 심어둔다.
	buf, rootCmd, configPath, closeFn := setupAuthTest(t, "existing-token", func(c *capturedRequest, w http.ResponseWriter, r *http.Request) {
		captured = c
		writeAuthEnvelope(w, map[string]any{"message": "로그아웃 성공"})
	})
	defer closeFn()

	require.NoError(t, setConfigValue(configPath, "auth.token", "existing-token", io.Discard))
	require.Equal(t, "existing-token", readConfigToken(t, configPath))

	rootCmd.SetArgs([]string{"auth", "logout"})
	require.NoError(t, rootCmd.Execute())

	require.NotNil(t, captured)
	assert.Equal(t, http.MethodPost, captured.method)
	assert.Equal(t, "/api/v1/auth/logout", captured.path)
	// 현재 클라이언트 토큰이 Authorization 헤더로 전송되었는지 확인
	assert.Equal(t, "Bearer existing-token", captured.auth)

	// 로컬 config 의 토큰이 제거되었는지 검증
	assert.Equal(t, "", readConfigToken(t, configPath))
	assert.Contains(t, buf.String(), "로그아웃")
}

func TestAuthLogoutRemovesTokenEvenOnServerError(t *testing.T) {
	buf, rootCmd, configPath, closeFn := setupAuthTest(t, "existing-token", func(c *capturedRequest, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   map[string]any{"code": "INTERNAL", "message": "서버 에러"},
		})
	})
	defer closeFn()

	require.NoError(t, setConfigValue(configPath, "auth.token", "existing-token", io.Discard))

	rootCmd.SetArgs([]string{"auth", "logout"})
	err := rootCmd.Execute()
	require.Error(t, err) // 서버 에러는 전파됨

	// 그래도 로컬 토큰은 제거되어야 한다.
	assert.Equal(t, "", readConfigToken(t, configPath))
	assert.Contains(t, buf.String(), "로컬 토큰이 제거되었습니다")
}

func TestAuthWhoamiCallsMe(t *testing.T) {
	var captured *capturedRequest
	buf, rootCmd, _, closeFn := setupAuthTest(t, "tok", func(c *capturedRequest, w http.ResponseWriter, r *http.Request) {
		captured = c
		writeAuthEnvelope(w, map[string]any{"username": "admin", "role": "admin"})
	})
	defer closeFn()

	rootCmd.SetArgs([]string{"auth", "whoami"})
	require.NoError(t, rootCmd.Execute())

	require.NotNil(t, captured)
	assert.Equal(t, http.MethodGet, captured.method)
	assert.Equal(t, "/api/v1/auth/me", captured.path)

	out := buf.String()
	assert.Contains(t, out, "admin")
}

func TestAuthWhoamiJSONFormat(t *testing.T) {
	buf, rootCmd, _, closeFn := setupAuthTest(t, "tok", func(c *capturedRequest, w http.ResponseWriter, r *http.Request) {
		writeAuthEnvelope(w, map[string]any{"username": "operator", "role": "viewer"})
	})
	defer closeFn()

	rootCmd.SetArgs([]string{"auth", "whoami", "--format", "json"})
	require.NoError(t, rootCmd.Execute())

	var got map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	assert.Equal(t, "operator", got["username"])
	assert.Equal(t, "viewer", got["role"])
}

func TestAuthPasswd(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		secrets []string
		wantCur string
		wantNew string
		wantErr bool
	}{
		{
			name:    "플래그로 비밀번호 변경",
			args:    []string{"auth", "passwd", "--current", "old", "--new", "newpass"},
			wantCur: "old",
			wantNew: "newpass",
		},
		{
			name:    "프롬프트로 비밀번호 변경 (확인 일치)",
			args:    []string{"auth", "passwd"},
			secrets: []string{"oldp", "newp", "newp"},
			wantCur: "oldp",
			wantNew: "newp",
		},
		{
			name:    "프롬프트 확인 불일치는 에러",
			args:    []string{"auth", "passwd"},
			secrets: []string{"oldp", "newp", "different"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured *capturedRequest
			buf, rootCmd, _, closeFn := setupAuthTest(t, "tok", func(c *capturedRequest, w http.ResponseWriter, r *http.Request) {
				captured = c
				writeAuthEnvelope(w, map[string]any{"message": "비밀번호가 변경되었습니다"})
			})
			defer closeFn()

			if len(tt.secrets) > 0 {
				stubSecret(t, tt.secrets...)
			}

			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, captured) // 검증 실패 시 서버 호출 없음
				return
			}
			require.NoError(t, err)

			require.NotNil(t, captured)
			assert.Equal(t, http.MethodPut, captured.method)
			assert.Equal(t, "/api/v1/auth/password", captured.path)
			assert.Equal(t, tt.wantCur, captured.body["current_password"])
			assert.Equal(t, tt.wantNew, captured.body["new_password"])

			// 비밀번호 평문 비노출 검증
			assert.NotContains(t, buf.String(), tt.wantNew)
		})
	}
}

func TestAuthRefresh(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		respToken   string
		wantErr     bool
		save        bool // --save 지정 여부 (config 저장 기대치)
		wantSession string
		wantBodyRT  string
	}{
		{
			name:        "리프레시 토큰으로 갱신 (세션 메모리)",
			args:        []string{"auth", "refresh", "--refresh-token", "rt-123"},
			respToken:   "new-access-token",
			wantSession: "new-access-token",
			wantBodyRT:  "rt-123",
		},
		{
			name:        "--save 갱신은 config 에도 저장",
			args:        []string{"auth", "refresh", "--refresh-token", "rt-123", "--save"},
			respToken:   "new-saved-token",
			save:        true,
			wantSession: "new-saved-token",
			wantBodyRT:  "rt-123",
		},
		{
			name:    "리프레시 토큰 미제공은 에러",
			args:    []string{"auth", "refresh"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 세션 메모리 토큰 격리
			sessionToken = ""
			defer func() { sessionToken = "" }()

			var captured *capturedRequest
			_, rootCmd, configPath, closeFn := setupAuthTest(t, "tok", func(c *capturedRequest, w http.ResponseWriter, r *http.Request) {
				captured = c
				writeAuthEnvelope(w, map[string]any{
					"access_token":  tt.respToken,
					"refresh_token": "rt-next",
					"expires_at":    int64(999),
					"token_type":    "Bearer",
				})
			})
			defer closeFn()

			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, captured)
				return
			}
			require.NoError(t, err)

			require.NotNil(t, captured)
			assert.Equal(t, http.MethodPost, captured.method)
			assert.Equal(t, "/api/v1/auth/refresh", captured.path)
			assert.Equal(t, tt.wantBodyRT, captured.body["refresh_token"])

			// 새 토큰이 세션 메모리에 반영되었는지 검증
			assert.Equal(t, tt.wantSession, sessionToken)

			// config 저장 여부는 --save 지정 시에만 기대한다.
			if tt.save {
				assert.Equal(t, tt.wantSession, readConfigToken(t, configPath))
			} else {
				assert.Equal(t, "", readConfigToken(t, configPath),
					"--save 없는 refresh 는 config 를 수정하면 안 됩니다")
			}
		})
	}
}

// TestAuthLoginDefaultDoesNotWriteConfig 는 --save 없는 로그인이
// 설정 파일에 토큰을 쓰지 않고 세션 메모리(sessionToken)와 라이브 클라이언트에만
// 토큰을 반영하는지 검증한다. (재현 테스트: 자동 디스크 저장 정책 제거)
func TestAuthLoginDefaultDoesNotWriteConfig(t *testing.T) {
	sessionToken = ""
	defer func() { sessionToken = "" }()

	var captured *capturedRequest
	buf, rootCmd, configPath, closeFn := setupAuthTest(t, "", func(c *capturedRequest, w http.ResponseWriter, r *http.Request) {
		captured = c
		writeAuthEnvelope(w, map[string]any{
			"user": map[string]any{"username": "admin", "role": "admin"},
			"tokens": map[string]any{
				"access_token":  "mem-token",
				"refresh_token": "refresh-xyz",
				"expires_at":    int64(1234567890),
				"token_type":    "Bearer",
			},
		})
	})
	defer closeFn()

	// 라이브 클라이언트 포인터를 검증하기 위해 명령에 연결된 client 를 추적한다.
	// setupAuthTest 는 clientPtr 를 내부에 보관하므로, 명령 실행 후
	// resolveToken 대신 sessionToken 과 client.token 을 직접 확인한다.
	rootCmd.SetArgs([]string{"auth", "login", "--username", "admin", "--password", "secret"})
	require.NoError(t, rootCmd.Execute())

	require.NotNil(t, captured)
	assert.Equal(t, "/api/v1/auth/login", captured.path)

	// 설정 파일에는 토큰이 기록되지 않아야 한다 (디스크 미저장 정책).
	assert.Equal(t, "", readConfigToken(t, configPath),
		"--save 없는 로그인은 config 에 토큰을 쓰면 안 됩니다")

	// 세션 메모리에는 토큰이 반영되어야 한다.
	assert.Equal(t, "mem-token", sessionToken,
		"로그인 후 sessionToken 에 토큰이 보관되어야 합니다")

	// 사용자 안내 메시지: 디스크 미저장 안내가 포함되어야 한다.
	assert.Contains(t, buf.String(), "재시작",
		"디스크 미저장(재로그인 필요) 안내 메시지가 있어야 합니다")
	assert.NotContains(t, buf.String(), "secret")
}

// TestAuthLoginSaveWritesConfig 는 --save 플래그가 주어지면
// 토큰이 설정 파일에 영구 저장되는지 검증한다.
func TestAuthLoginSaveWritesConfig(t *testing.T) {
	sessionToken = ""
	defer func() { sessionToken = "" }()

	buf, rootCmd, configPath, closeFn := setupAuthTest(t, "", func(c *capturedRequest, w http.ResponseWriter, r *http.Request) {
		writeAuthEnvelope(w, map[string]any{
			"user": map[string]any{"username": "admin", "role": "admin"},
			"tokens": map[string]any{
				"access_token":  "saved-token",
				"refresh_token": "refresh-xyz",
				"expires_at":    int64(1234567890),
				"token_type":    "Bearer",
			},
		})
	})
	defer closeFn()

	rootCmd.SetArgs([]string{"auth", "login", "--username", "admin", "--password", "secret", "--save"})
	require.NoError(t, rootCmd.Execute())

	// --save 시 설정 파일에 토큰이 저장되어야 한다.
	assert.Equal(t, "saved-token", readConfigToken(t, configPath),
		"--save 로그인은 config 에 토큰을 저장해야 합니다")
	assert.Equal(t, "saved-token", sessionToken,
		"--save 로그인도 sessionToken 에 토큰을 반영해야 합니다")
	assert.Contains(t, buf.String(), "설정에 저장")
}

// TestAuthLogoutClearsSessionToken 은 logout 이 세션 메모리 토큰을 비우는지 검증한다.
func TestAuthLogoutClearsSessionToken(t *testing.T) {
	sessionToken = "live-mem-token"
	defer func() { sessionToken = "" }()

	_, rootCmd, _, closeFn := setupAuthTest(t, "live-mem-token", func(c *capturedRequest, w http.ResponseWriter, r *http.Request) {
		writeAuthEnvelope(w, map[string]any{"message": "로그아웃 성공"})
	})
	defer closeFn()

	rootCmd.SetArgs([]string{"auth", "logout"})
	require.NoError(t, rootCmd.Execute())

	assert.Equal(t, "", sessionToken, "로그아웃 후 sessionToken 이 비워져야 합니다")
}

// TestAuthRefreshDefaultDoesNotWriteConfig 는 --save 없는 refresh 가
// 세션 메모리에만 새 토큰을 반영하고 config 에는 쓰지 않는지 검증한다.
func TestAuthRefreshDefaultDoesNotWriteConfig(t *testing.T) {
	sessionToken = ""
	defer func() { sessionToken = "" }()

	_, rootCmd, configPath, closeFn := setupAuthTest(t, "tok", func(c *capturedRequest, w http.ResponseWriter, r *http.Request) {
		writeAuthEnvelope(w, map[string]any{
			"access_token":  "refreshed-mem",
			"refresh_token": "rt-next",
			"expires_at":    int64(999),
			"token_type":    "Bearer",
		})
	})
	defer closeFn()

	rootCmd.SetArgs([]string{"auth", "refresh", "--refresh-token", "rt-123"})
	require.NoError(t, rootCmd.Execute())

	assert.Equal(t, "", readConfigToken(t, configPath),
		"--save 없는 refresh 는 config 에 토큰을 쓰면 안 됩니다")
	assert.Equal(t, "refreshed-mem", sessionToken,
		"refresh 후 sessionToken 에 새 토큰이 반영되어야 합니다")
}

// TestAuthRefreshSaveWritesConfig 는 --save refresh 가 config 에 저장하는지 검증한다.
func TestAuthRefreshSaveWritesConfig(t *testing.T) {
	sessionToken = ""
	defer func() { sessionToken = "" }()

	_, rootCmd, configPath, closeFn := setupAuthTest(t, "tok", func(c *capturedRequest, w http.ResponseWriter, r *http.Request) {
		writeAuthEnvelope(w, map[string]any{
			"access_token":  "refreshed-saved",
			"refresh_token": "rt-next",
			"expires_at":    int64(999),
			"token_type":    "Bearer",
		})
	})
	defer closeFn()

	rootCmd.SetArgs([]string{"auth", "refresh", "--refresh-token", "rt-123", "--save"})
	require.NoError(t, rootCmd.Execute())

	assert.Equal(t, "refreshed-saved", readConfigToken(t, configPath),
		"--save refresh 는 config 에 토큰을 저장해야 합니다")
	assert.Equal(t, "refreshed-saved", sessionToken)
}

func TestAuthCmdStructure(t *testing.T) {
	clientPtr := (*Client)(nil)
	cp := &clientPtr
	confirmFn := func(string, io.Reader) bool { return true }

	cmd := newAuthCmd(cp, confirmFn)
	assert.Equal(t, "auth", cmd.Name())

	want := map[string]bool{
		"login":   false,
		"logout":  false,
		"whoami":  false,
		"passwd":  false,
		"refresh": false,
	}
	for _, sub := range cmd.Commands() {
		want[sub.Name()] = true
	}
	for name, found := range want {
		assert.True(t, found, "서브커맨드 누락: %s", name)
	}
}

func TestPromptLine(t *testing.T) {
	reader := strings.NewReader("  hello-user  \n")
	got, err := promptLine(reader, "사용자명: ")
	require.NoError(t, err)
	assert.Equal(t, "hello-user", got)
}

func TestResolveConfigPathDefault(t *testing.T) {
	rootCmd := &cobra.Command{Use: "xflow"}
	rootCmd.PersistentFlags().String("config", "", "설정 파일 경로")
	sub := &cobra.Command{Use: "child"}
	rootCmd.AddCommand(sub)

	home, _ := os.UserHomeDir()
	got := resolveConfigPath(sub)
	assert.Equal(t, filepath.Join(home, ".xflow", "config.yaml"), got)
}
