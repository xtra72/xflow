// migrate_tsdb_tags_test.go (SPEC-DEVICE-IDENTITY-001 Phase D § D-T19)
//
// xflowd v1.0 부터 `xflowd migrate tsdb-tags` 명령은 deprecated noop 으로
// 동작한다. 기존 Phase C2 의 CLI 통합 테스트들 (help / required-flags /
// invalid-target / dry-run / output-dir 충돌 / ping 실패 등) 은 모두 삭제되었으며,
// 본 파일은 deprecation 메시지 출력 + exit code 0 의 단일 시나리오만 검증한다.

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestMigrateTSDBCLI_DeprecatedNoop — Phase D § D-T19: tsdb-tags 명령은 모든
// 플래그 조합에서 deprecated 안내 메시지를 출력하고 즉시 exit code 0 으로 종료.
func TestMigrateTSDBCLI_DeprecatedNoop(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
	}{
		{"flags 없음", []string{"tsdb-tags"}},
		{"flags 일부 지정", []string{"tsdb-tags", "--influx-url", "http://x", "--bucket", "b"}},
		{"dry-run 플래그", []string{"tsdb-tags", "--dry-run"}},
		{"target=v2", []string{"tsdb-tags", "--target", "v2"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := newMigrateCmd()
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetArgs(tc.args)

			err := root.Execute()
			if err != nil {
				t.Fatalf("expected nil err (deprecated noop), got: %v\nstderr: %s",
					err, stderr.String())
			}

			out := stdout.String()
			if !strings.Contains(out, "DEPRECATED") {
				t.Errorf("expected DEPRECATED message in stdout, got: %q", out)
			}
			if !strings.Contains(out, "v1.0 환경에는 마이그레이션 대상 없음") {
				t.Errorf("expected v1.0 noop guidance in stdout, got: %q", out)
			}
			if !strings.Contains(out, "brownfield") {
				t.Errorf("expected brownfield guidance in stdout, got: %q", out)
			}
			if !strings.Contains(out, "internal/migrate/tsdbtags") {
				t.Errorf("expected internal-package preservation note in stdout, got: %q", out)
			}
		})
	}
}
