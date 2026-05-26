// print_test.go — Plan.Print / Result.Print + accessor 보조 함수 단위 테스트.
//
// 통합 테스트에서 자연스럽게 호출되지 않는 형식 분기들을 보강한다.
package tsdbtags

import (
	"bytes"
	"strings"
	"testing"
)

func TestPlan_Accessors_OnEmpty(t *testing.T) {
	t.Parallel()

	p := &Plan{result: ClassifyResult{}, target: TargetV2}
	if p.HasAmbiguous() {
		t.Errorf("빈 plan 은 HasAmbiguous=false 여야 함")
	}
	if p.result.HasMapped() {
		t.Errorf("빈 result 는 HasMapped=false 여야 함")
	}
}

func TestPlan_HasAmbiguous_True(t *testing.T) {
	t.Parallel()

	p := &Plan{result: ClassifyResult{
		Ambiguous: []AmbiguousTagValue{{Composite: "x:1", UUID: "u"}},
	}}
	if !p.HasAmbiguous() {
		t.Errorf("Ambiguous 가 있는데 HasAmbiguous=false")
	}
	if !p.result.HasAmbiguous() {
		t.Errorf("result.HasAmbiguous=false")
	}
}

func TestClassifyResult_HasMapped(t *testing.T) {
	t.Parallel()

	r := ClassifyResult{Mapped: []MappedTagValue{{Composite: "a:1", UUID: "u"}}}
	if !r.HasMapped() {
		t.Errorf("HasMapped=false")
	}
}

func TestPlan_Print_AllSections(t *testing.T) {
	t.Parallel()

	p := &Plan{
		result: ClassifyResult{
			Mapped: []MappedTagValue{
				{Composite: "lgcnp:81", UUID: "a58", Measurements: []string{"m1", "m2"}, TagKey: "device_id"},
			},
			Ambiguous: []AmbiguousTagValue{
				{Composite: "lgcnp:99a", UUID: "ee", Conflicts: []string{"lgcnp:99b"}},
			},
			Orphan:      []string{"unknown:1"},
			UUIDAlready: []string{"d8e7f99b-8074-4e6f-c051-af6fbd4e5f60"},
		},
		scanned: []ScannedTagValue{{Value: "x"}, {Value: "y"}},
		target:  TargetV2,
	}
	var buf bytes.Buffer
	if err := p.Print(&buf); err != nil {
		t.Fatalf("Print: %v", err)
	}
	body := buf.String()
	for _, want := range []string{
		"mapped=1", "ambiguous=1", "orphan=1", "uuid-already=1", "scanned=2", "target=v2",
		"[mapped]", "lgcnp:81", "m1, m2",
		"[ambiguous]", "lgcnp:99a", "lgcnp:99b",
		"[orphan]", "unknown:1",
		"[uuid-already] 1 entries",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Plan.Print 출력에 %q 없음:\n%s", want, body)
		}
	}
}

func TestResult_Print_DryRun(t *testing.T) {
	t.Parallel()

	r := &Result{DryRun: true, MappedCount: 5}
	var buf bytes.Buffer
	if err := r.Print(&buf); err != nil {
		t.Fatalf("Print: %v", err)
	}
	if !strings.Contains(buf.String(), "dry-run") {
		t.Errorf("dry-run 출력 없음: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "mapped=5") {
		t.Errorf("mapped=5 출력 없음: %s", buf.String())
	}
}

func TestResult_Print_NoMapped(t *testing.T) {
	t.Parallel()

	r := &Result{MappedCount: 0}
	var buf bytes.Buffer
	if err := r.Print(&buf); err != nil {
		t.Fatalf("Print: %v", err)
	}
	if !strings.Contains(buf.String(), "no mapped entries") {
		t.Errorf("no mapped 메시지 없음: %s", buf.String())
	}
}

func TestResult_Print_Full(t *testing.T) {
	t.Parallel()

	r := &Result{
		MappedCount: 3,
		OutputDir:   "/tmp/out",
		ScriptFile:  "/tmp/out/migration-v2.flux",
		ReadmeFile:  "/tmp/out/RUN.md",
	}
	var buf bytes.Buffer
	if err := r.Print(&buf); err != nil {
		t.Fatalf("Print: %v", err)
	}
	for _, want := range []string{"mapped=3", "/tmp/out", "migration-v2.flux", "RUN.md"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("Result.Print 에 %q 없음: %s", want, buf.String())
		}
	}
}
