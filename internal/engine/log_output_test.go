package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
)

// ---------------------------------------------------------------------------
// TestParseLogOutput - log_output 문자열 파싱 테스트
// ---------------------------------------------------------------------------

func TestParseLogOutput(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		want      LogOutputTarget
		wantErr   bool
	}{
		{
			name: "stdout 키워드",
			raw:  "stdout",
			want: LogOutputTarget{UseStdout: true, FilePath: ""},
		},
		{
			name: "단일 파일 경로",
			raw:  "/var/log/xflow/node.log",
			want: LogOutputTarget{UseStdout: false, FilePath: "/var/log/xflow/node.log"},
		},
		{
			name: "stdout+파일 경로 조합",
			raw:  "stdout+/var/log/xflow/node.log",
			want: LogOutputTarget{UseStdout: true, FilePath: "/var/log/xflow/node.log"},
		},
		{
			name: "빈 문자열은 설정 없음",
			raw:  "",
			want: LogOutputTarget{UseStdout: false, FilePath: ""},
		},
		{
			name:    "stdout+ 뒤에 경로 없으면 에러",
			raw:     "stdout+",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLogOutput(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// TestResolveNodeLogOutput - 계층적 우선순위 해석 테스트
// ---------------------------------------------------------------------------

func TestResolveNodeLogOutput(t *testing.T) {
	tests := []struct {
		name          string
		nd            flow.NodeDef
		flowCfg       flow.FlowConfig
		serverDefault string
		wantTarget    LogOutputTarget
		wantExplicit  bool
	}{
		{
			name: "노드 config 설정 시 노드 값 사용",
			nd: flow.NodeDef{
				Config: map[string]any{"log_output": "stdout"},
			},
			flowCfg:       flow.FlowConfig{LogOutput: "/var/log/flow.log"},
			serverDefault: "/var/log/server.log",
			wantTarget:    LogOutputTarget{UseStdout: true},
			wantExplicit:  true,
		},
		{
			name:          "노드 미설정, 플로우 설정 시 플로우 값 사용",
			nd:            flow.NodeDef{},
			flowCfg:       flow.FlowConfig{LogOutput: "/var/log/flow.log"},
			serverDefault: "/var/log/server.log",
			wantTarget:    LogOutputTarget{FilePath: "/var/log/flow.log"},
			wantExplicit:  true,
		},
		{
			name:          "노드/플로우 미설정, 서버 기본값 사용",
			nd:            flow.NodeDef{},
			flowCfg:       flow.FlowConfig{},
			serverDefault: "/var/log/server.log",
			wantTarget:    LogOutputTarget{FilePath: "/var/log/server.log"},
			wantExplicit:  true,
		},
		{
			name:          "모두 미설정 시 explicit=false",
			nd:            flow.NodeDef{},
			flowCfg:       flow.FlowConfig{},
			serverDefault: "",
			wantTarget:    LogOutputTarget{},
			wantExplicit:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, explicit := resolveNodeLogOutput(tt.nd, tt.flowCfg, tt.serverDefault)
			require.Equal(t, tt.wantExplicit, explicit)
			require.Equal(t, tt.wantTarget, target)
		})
	}
}

// ---------------------------------------------------------------------------
// TestOpenLogFile - 로그 파일 생성 및 열기 테스트
// ---------------------------------------------------------------------------

func TestOpenLogFile(t *testing.T) {
	t.Run("기존 디렉토리에 파일 생성", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test.log")

		f, err := openLogFile(path)
		require.NoError(t, err)
		require.NotNil(t, f)
		defer f.Close()

		// 파일이 존재하는지 확인
		_, err = os.Stat(path)
		require.NoError(t, err)
	})

	t.Run("디렉토리 자동 생성(MkdirAll)", func(t *testing.T) {
		dir := t.TempDir()
		nestedPath := filepath.Join(dir, "nested", "deep", "test.log")

		f, err := openLogFile(nestedPath)
		require.NoError(t, err)
		require.NotNil(t, f)
		defer f.Close()

		// 중첩 디렉토리가 생성되었는지 확인
		_, err = os.Stat(filepath.Join(dir, "nested", "deep"))
		require.NoError(t, err)
	})

	t.Run("append 모드(기존 내용 보존)", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "existing.log")

		// 초기 내용 작성
		err := os.WriteFile(path, []byte("existing\n"), 0644)
		require.NoError(t, err)

		// openLogFile 로 열어서 추가 기록
		f, err := openLogFile(path)
		require.NoError(t, err)
		_, err = f.Write([]byte("appended\n"))
		require.NoError(t, err)
		f.Close()

		// 기존 내용 + 추가 내용 확인
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, "existing\nappended\n", string(content))
	})

	t.Run("읽기 전용 경로에서 에러 반환", func(t *testing.T) {
		// /dev/null/impossible 은 디렉토리 생성이 불가능한 경로
		_, err := openLogFile("/dev/null/impossible/test.log")
		require.Error(t, err)
	})
}
