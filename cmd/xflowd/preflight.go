// @SPEC:SPEC-DEVICE-IDENTITY-001 Phase D § D-T5
// preflight.go — `xflowd preflight` 명령.
//
// xflowd v1.0 (Phase D) 부팅 사전 점검 명령. greenfield 환경 (외부 클라이언트
// 부재) 가정 하에 단순한 sanity check 를 수행하며, brownfield 마이그레이션
// 검증 (composite metadata 잔존 / yaml composite 참조) 은 부수적 가드로
// 포함한다.
//
// 점검 항목 (모두 통과 시 exit 0, 하나라도 실패 시 exit 1):
//
//   1. config 파일 yaml 형식 검증 — 파일이 존재하고 yaml parser 가 통과.
//      composite ("agent:local_id") 참조는 ParseDeviceRef 가 거부하므로
//      ErrInvalidDeviceReference 가 발생하면 yaml composite 잔존 신호.
//
//   2. device_ids.json 존재 + 로드 가능 — 영속 UUID repository 가 부팅 후
//      Device.UID() 호출에 응답 가능한지 확인. 파일 부재는 첫 부팅 시
//      예상되므로 경고 (WARN) 로만 표시하고 exit 0 유지.
//
//   3. device_metadata.json 의 키가 composite 형식 (agent:local_id) 부재.
//      Phase C1 migrate device-ids 도구가 완료된 상태인지 검증.
//      composite key 발견 시 actionable 에러 메시지로 즉시 안내.
//
// 본 명령은 read-only — 어떤 파일도 수정하지 않는다.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xtra/xflow/internal/config"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/storage"
)

// preflightFlags 는 preflight 서브명령의 플래그를 보관한다.
type preflightFlags struct {
	// configPath 는 검증할 config 파일 경로. 빈 문자열이면 기본 경로 사용.
	configPath string

	// dataDir 는 영속 메타데이터 / device_ids.json 의 부모 디렉토리.
	// 빈 문자열이면 storage 패키지의 기본값 사용.
	dataDir string
}

// newPreflightCmd 는 `xflowd preflight` 명령을 생성한다.
//
// 호출 형식:
//
//	xflowd preflight                                  # 기본 경로 점검
//	xflowd preflight --config /etc/xflow/config.yaml
//	xflowd preflight --data-dir /var/lib/xflow
//
// 성공 시 exit 0, 실패 시 exit 1 + 명확한 actionable 메시지.
func newPreflightCmd() *cobra.Command {
	flags := &preflightFlags{}
	cmd := &cobra.Command{
		Use:   "preflight",
		Short: "xflowd v1.0 부팅 사전 점검 (sanity check)",
		Long: `xflowd 부팅 전 환경이 v1.0 (Phase D) 요구사항을 만족하는지 점검합니다.

점검 항목:
  1. config 파일 yaml 형식 검증 (composite 참조 부재 확인)
  2. device_ids.json 존재 + 로드 가능 (UUID repository 작동 확인)
  3. device_metadata.json 의 composite key 부재 (Phase C1 마이그레이션 완료 확인)

성공 시 exit 0, 실패 시 exit 1 + 명확한 수정 안내. 본 명령은 read-only 입니다.

SPEC-DEVICE-IDENTITY-001 Phase D § D-T5.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPreflight(cmd.OutOrStdout(), cmd.ErrOrStderr(), flags)
		},
	}

	cmd.Flags().StringVar(&flags.configPath, "config", "", "config 파일 경로 (기본: ~/.xflow/config.yaml)")
	cmd.Flags().StringVar(&flags.dataDir, "data-dir", "", "영속 데이터 디렉토리 (기본: ~/.xflow/data)")

	return cmd
}

// preflightResult 는 단일 점검 항목의 결과를 표현한다.
type preflightResult struct {
	name    string
	passed  bool
	message string
}

// runPreflight 는 모든 점검 항목을 순차 실행하고 결과를 출력한다.
// 하나라도 실패하면 errors.New("preflight failed") 를 반환 (exit 1).
func runPreflight(stdout, stderr interface{ Write([]byte) (int, error) }, flags *preflightFlags) error {
	results := make([]preflightResult, 0, 3)

	// 1. config 파일 yaml + composite 검증.
	results = append(results, checkConfigYAML(flags.configPath))

	// 2. device_ids.json 검증.
	results = append(results, checkDeviceIDsRepository(flags.dataDir))

	// 3. device_metadata.json 의 composite key 검증.
	results = append(results, checkDeviceMetadataKeys(flags.dataDir))

	// 결과 출력.
	allPassed := true
	for _, r := range results {
		mark := "PASS"
		if !r.passed {
			mark = "FAIL"
			allPassed = false
		}
		fmt.Fprintf(stdout, "[%s] %s\n", mark, r.name)
		if r.message != "" {
			fmt.Fprintf(stdout, "       %s\n", r.message)
		}
	}

	if !allPassed {
		fmt.Fprintln(stderr, "\nPreflight FAILED. Address the issues above and re-run.")
		return fmt.Errorf("preflight failed")
	}

	fmt.Fprintln(stdout, "\nPreflight PASSED — xflowd v1.0 is ready to boot.")
	return nil
}

// checkConfigYAML 은 config 파일이 존재하고 yaml 형식이 정상인지, composite
// 참조가 없는지 검증한다.
//
// 검증 방법:
//   - configPath 가 빈 문자열이면 기본 경로 사용.
//   - config.Load() 호출 시 ErrInvalidDeviceReference 가 발생하면 composite
//     잔존 신호로 FAIL.
//   - 파일 부재 (예: 첫 부팅) 는 명령 옵션 미설정으로 간주, WARN 으로 PASS.
func checkConfigYAML(configPath string) preflightResult {
	res := preflightResult{name: "config yaml format / composite reference check"}

	// config.Load 는 yaml 파싱과 device-ref 검증을 모두 수행.
	var opts []config.LoadOption
	if configPath != "" {
		opts = append(opts, config.WithConfigFile(configPath))
	}
	cfg, err := config.Load(opts...)
	if err != nil {
		if isCompositeRefError(err) {
			res.message = fmt.Sprintf("config yaml contains legacy composite device reference: %v.\n"+
				"       Update yaml to use UUID or 'agent/name' format.", err)
			return res
		}
		// 파일 부재는 첫 부팅 시 정상 — PASS 로 처리하되 노트.
		if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file") {
			res.passed = true
			res.message = "config file not found (first boot — defaults will be used)"
			return res
		}
		res.message = fmt.Sprintf("config load failed: %v", err)
		return res
	}
	_ = cfg
	res.passed = true
	return res
}

// isCompositeRefError 는 에러가 yaml composite 참조에 의한 것인지 검사한다.
func isCompositeRefError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "invalid device reference") ||
		strings.Contains(msg, "composite")
}

// checkDeviceIDsRepository 는 device_ids.json (DeviceIDFileRepository 의 영속
// 파일) 이 로드 가능한지 검증한다.
//
// 파일 부재는 첫 부팅 시 정상 (PASS + WARN). 파일이 존재하지만 손상되어
// 로드 실패 시 FAIL.
func checkDeviceIDsRepository(dataDir string) preflightResult {
	res := preflightResult{name: "device_ids.json (UUID repository) check"}

	dir := dataDir
	if dir == "" {
		// storage 기본 경로 사용.
		home, err := os.UserHomeDir()
		if err != nil {
			res.message = fmt.Sprintf("cannot determine default data-dir: %v", err)
			return res
		}
		dir = filepath.Join(home, ".xflow", "data", "device_ids")
	} else {
		dir = filepath.Join(dir, "device_ids")
	}

	// 디렉토리가 없으면 첫 부팅 — PASS + WARN.
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			res.passed = true
			res.message = fmt.Sprintf("device_ids directory not found at %s (first boot — will be created)", dir)
			return res
		}
		res.message = fmt.Sprintf("stat device_ids directory failed: %v", err)
		return res
	}

	// NewDeviceIDFileRepository 는 파일을 로드한다 (없으면 빈 cache 로 시작).
	repo, err := storage.NewDeviceIDFileRepository(dir)
	if err != nil {
		res.message = fmt.Sprintf("device_ids repository load failed: %v.\n"+
			"       Inspect %s/device_ids.json for corruption.", err, dir)
		return res
	}
	defer repo.Close()

	mappings, err := repo.List(context.Background())
	if err != nil {
		res.message = fmt.Sprintf("device_ids List failed: %v", err)
		return res
	}
	res.passed = true
	res.message = fmt.Sprintf("device_ids repository OK (%d mappings loaded)", len(mappings))
	return res
}

// checkDeviceMetadataKeys 는 device_metadata.json 의 key 가 composite
// ("agent:local_id") 형식인지 검사한다. composite key 가 존재하면 Phase C1
// migrate device-ids 도구가 미완료 — FAIL + actionable 안내.
func checkDeviceMetadataKeys(dataDir string) preflightResult {
	res := preflightResult{name: "device_metadata.json composite key check"}

	dir := dataDir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			res.message = fmt.Sprintf("cannot determine default data-dir: %v", err)
			return res
		}
		dir = filepath.Join(home, ".xflow", "data", "device_metadata")
	} else {
		dir = filepath.Join(dir, "device_metadata")
	}

	metaFile := filepath.Join(dir, "device_metadata.json")
	if _, err := os.Stat(metaFile); err != nil {
		if os.IsNotExist(err) {
			res.passed = true
			res.message = fmt.Sprintf("metadata file not found at %s (first boot — no metadata yet)", metaFile)
			return res
		}
		res.message = fmt.Sprintf("stat metadata file failed: %v", err)
		return res
	}

	// metadata 파일 내용 파싱 (key 만 검사).
	data, err := os.ReadFile(metaFile)
	if err != nil {
		res.message = fmt.Sprintf("read metadata file failed: %v", err)
		return res
	}
	if len(data) == 0 {
		res.passed = true
		res.message = "metadata file is empty"
		return res
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		res.message = fmt.Sprintf("metadata file parse failed: %v", err)
		return res
	}

	var compositeKeys []string
	for key := range raw {
		// composite ("agent:local_id") 는 UUID 형식이 아니면서 콜론 포함.
		if device.ClassifyDeviceRef(key) == device.DeviceRefUnknown && strings.Contains(key, ":") {
			compositeKeys = append(compositeKeys, key)
		}
	}

	if len(compositeKeys) > 0 {
		// 최대 5개까지 표시.
		display := compositeKeys
		if len(display) > 5 {
			display = display[:5]
		}
		res.message = fmt.Sprintf("found %d composite keys (legacy) in %s.\n"+
			"       Examples: %v\n"+
			"       Run: xflowd migrate device-ids --metadata-dir %s (before xflowd v1.0)",
			len(compositeKeys), metaFile, display, dir)
		return res
	}

	res.passed = true
	res.message = fmt.Sprintf("metadata file OK (%d keys, all UUID)", len(raw))
	return res
}
