// script_test.go — Flux / SQL / RUN.md 스크립트 생성기의 golden file 테스트.
//
// 테스트는 결정론 (동일 입력 → 동일 byte 출력) 과 출력 형식을 검증한다.
// testdata/expected/ 의 golden file 과 byte-perfect 비교한다.
//
// golden file 재생성 옵션: TSDBTAGS_REGEN=1 환경변수가 설정되면 expected/
// 파일을 덮어쓴다 (수동 검토 후 commit). production 테스트에는 사용 안 함.
package tsdbtags

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// goldenFixture 는 script 생성 테스트의 공통 입력이다.
func goldenFixture() (ClassifyResult, time.Time) {
	// 고정된 timestamp — 결정론적 출력.
	ts := time.Date(2026, 5, 26, 10, 30, 0, 0, time.UTC)
	result := ClassifyResult{
		Mapped: []MappedTagValue{
			{
				Composite:    "century:7",
				UUID:         "d8e7f99b-8074-4e6f-c051-af6fbd4e5f60",
				Measurements: []string{"outdoor_temp"},
				TagKey:       "device_id",
			},
			{
				Composite:    "lgcnp:81",
				UUID:         "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
				Measurements: []string{"indoor_temp", "outdoor_temp"},
				TagKey:       "device_id",
			},
			{
				Composite:    "lgcnp:82",
				UUID:         "b6c5d779-6852-4c4d-ae3f-8f4d9b2c3d4e",
				Measurements: []string{"indoor_temp"},
				TagKey:       "device_id",
			},
		},
		Orphan:      []string{"samsung:0.0.16"},
		UUIDAlready: []string{"a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"},
	}
	return result, ts
}

func TestGenerateFluxScript_Golden(t *testing.T) {
	t.Parallel()

	result, ts := goldenFixture()
	var buf bytes.Buffer
	if err := GenerateFluxScript(&buf, "xflow", "acme", result, ts); err != nil {
		t.Fatalf("GenerateFluxScript: %v", err)
	}

	goldenPath := filepath.Join("testdata", "expected", "migration-v2.flux")
	compareGolden(t, goldenPath, buf.Bytes())
}

func TestGenerateSQLScript_Golden(t *testing.T) {
	t.Parallel()

	result, ts := goldenFixture()
	var buf bytes.Buffer
	if err := GenerateSQLScript(&buf, "xflow", result, ts); err != nil {
		t.Fatalf("GenerateSQLScript: %v", err)
	}

	goldenPath := filepath.Join("testdata", "expected", "migration-v3.sql")
	compareGolden(t, goldenPath, buf.Bytes())
}

func TestGenerateReadme_V2Golden(t *testing.T) {
	t.Parallel()

	_, ts := goldenFixture()
	var buf bytes.Buffer
	includes := "- migration-v2.flux\n- RUN.md\n"
	if err := GenerateReadme(&buf, TargetV2, "xflow", 3, ts, includes); err != nil {
		t.Fatalf("GenerateReadme v2: %v", err)
	}
	goldenPath := filepath.Join("testdata", "expected", "RUN-v2.md")
	compareGolden(t, goldenPath, buf.Bytes())
}

func TestGenerateReadme_V3Golden(t *testing.T) {
	t.Parallel()

	_, ts := goldenFixture()
	var buf bytes.Buffer
	includes := "- migration-v3.sql\n- RUN.md\n"
	if err := GenerateReadme(&buf, TargetV3, "xflow", 3, ts, includes); err != nil {
		t.Fatalf("GenerateReadme v3: %v", err)
	}
	goldenPath := filepath.Join("testdata", "expected", "RUN-v3.md")
	compareGolden(t, goldenPath, buf.Bytes())
}

func TestGenerateFluxScript_Deterministic(t *testing.T) {
	t.Parallel()

	result, ts := goldenFixture()
	var buf1, buf2 bytes.Buffer
	_ = GenerateFluxScript(&buf1, "xflow", "acme", result, ts)
	_ = GenerateFluxScript(&buf2, "xflow", "acme", result, ts)
	if !bytes.Equal(buf1.Bytes(), buf2.Bytes()) {
		t.Errorf("동일 입력에서 다른 출력:\n  buf1=%q\n  buf2=%q", buf1.String(), buf2.String())
	}
}

func TestGenerateReadme_UnknownTarget(t *testing.T) {
	t.Parallel()

	_, ts := goldenFixture()
	var buf bytes.Buffer
	err := GenerateReadme(&buf, Target("v9"), "xflow", 3, ts, "")
	if err == nil {
		t.Errorf("알 수 없는 target 에서 에러를 반환해야 함")
	}
}

// compareGolden 은 generated 의 byte 를 golden file 과 비교한다.
//
// TSDBTAGS_REGEN=1 환경변수가 설정되면 golden file 을 덮어쓴다 (개발자가
// 의도적으로 fixture 를 갱신할 때 사용).
func compareGolden(t *testing.T, path string, generated []byte) {
	t.Helper()

	if os.Getenv("TSDBTAGS_REGEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("regen mkdir: %v", err)
		}
		if err := os.WriteFile(path, generated, 0o644); err != nil {
			t.Fatalf("regen write: %v", err)
		}
		t.Logf("[REGEN] golden file 갱신: %s", path)
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden file (%s) 읽기 실패: %v (TSDBTAGS_REGEN=1 로 재생성 가능)", path, err)
	}
	if !bytes.Equal(generated, want) {
		// 차이 진단을 돕기 위해 처음 다른 라인을 표시.
		genLines := strings.Split(string(generated), "\n")
		wantLines := strings.Split(string(want), "\n")
		minLen := len(genLines)
		if len(wantLines) < minLen {
			minLen = len(wantLines)
		}
		for i := 0; i < minLen; i++ {
			if genLines[i] != wantLines[i] {
				t.Errorf("golden file (%s) line %d 불일치:\n  got:  %q\n  want: %q",
					path, i+1, genLines[i], wantLines[i])
				return
			}
		}
		t.Errorf("golden file (%s) 라인 수 불일치: got=%d want=%d",
			path, len(genLines), len(wantLines))
	}
}
