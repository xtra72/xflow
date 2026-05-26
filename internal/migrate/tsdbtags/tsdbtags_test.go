// tsdbtags_test.go — Options 검증과 path resolver 의 단위 테스트.
package tsdbtags

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateOptions_Required(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		opts    Options
		wantErr string
	}{
		{
			name:    "url 누락",
			opts:    Options{InfluxToken: "tok", Bucket: "b", Target: TargetV2, Org: "o"},
			wantErr: "influx-url",
		},
		{
			name:    "token 누락",
			opts:    Options{InfluxURL: "http://x", Bucket: "b", Target: TargetV2, Org: "o"},
			wantErr: "influx-token",
		},
		{
			name:    "bucket 누락",
			opts:    Options{InfluxURL: "http://x", InfluxToken: "tok", Target: TargetV2, Org: "o"},
			wantErr: "bucket",
		},
		{
			name:    "v2 인데 org 누락",
			opts:    Options{InfluxURL: "http://x", InfluxToken: "tok", Bucket: "b", Target: TargetV2},
			wantErr: "org",
		},
		{
			name:    "잘못된 target",
			opts:    Options{InfluxURL: "http://x", InfluxToken: "tok", Bucket: "b", Target: "v9"},
			wantErr: "target",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateOptions(tc.opts)
			if err == nil {
				t.Fatalf("에러를 반환해야 함")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("에러 메시지에 %q 포함되어야 함: %v", tc.wantErr, err)
			}
		})
	}
}

func TestValidateOptions_Valid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		opts Options
	}{
		{
			name: "v2 with org",
			opts: Options{InfluxURL: "http://x", InfluxToken: "tok", Bucket: "b", Target: TargetV2, Org: "o"},
		},
		{
			name: "v3 without org",
			opts: Options{InfluxURL: "http://x", InfluxToken: "tok", Bucket: "b", Target: TargetV3},
		},
		{
			name: "v3 with org",
			opts: Options{InfluxURL: "http://x", InfluxToken: "tok", Bucket: "b", Target: TargetV3, Org: "o"},
		},
		{
			name: "auto without org",
			opts: Options{InfluxURL: "http://x", InfluxToken: "tok", Bucket: "b", Target: TargetAuto},
		},
		{
			name: "빈 target == auto",
			opts: Options{InfluxURL: "http://x", InfluxToken: "tok", Bucket: "b"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := validateOptions(tc.opts); err != nil {
				t.Errorf("정상 옵션이 거부됨: %v", err)
			}
		})
	}
}

func TestResolveOutputDir_Default(t *testing.T) {
	t.Parallel()

	// OutputDir 빈 문자열이면 timestamped 기본값.
	got := ResolveOutputDir(Options{})
	if !strings.HasPrefix(got, "tsdb-migrations-") {
		t.Errorf("기본 OutputDir prefix 가 'tsdb-migrations-' 아님: %s", got)
	}
}

func TestResolveOutputDir_Explicit(t *testing.T) {
	t.Parallel()

	got := ResolveOutputDir(Options{OutputDir: "/var/lib/xflow/migrations"})
	if got != "/var/lib/xflow/migrations" {
		t.Errorf("명시 OutputDir 가 유지되지 않음: %s", got)
	}
}

func TestResolveIDRepoPath_Default(t *testing.T) {
	t.Parallel()

	got := ResolveIDRepoPath(Options{}, "/home/op")
	want := filepath.Join("/home/op", ".xflow", "storage", "device_ids", "device_ids.json")
	if got != want {
		t.Errorf("기본 IDRepoPath:\n  got=%s\n  want=%s", got, want)
	}
}

func TestResolveIDRepoPath_Explicit(t *testing.T) {
	t.Parallel()

	got := ResolveIDRepoPath(Options{IDRepoPath: "/custom/path.json"}, "/home/op")
	if got != "/custom/path.json" {
		t.Errorf("명시 IDRepoPath 가 유지되지 않음: %s", got)
	}
}
