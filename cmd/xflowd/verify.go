// verify.go 는 `xflowd verify` 서브커맨드를 제공한다.
//
// 설정을 로드·검증하고 즉시 종료한다(성공 0 / 실패 비0). 서버/에이전트/포트 바인딩은
// 띄우지 않으므로(하드웨어·DB·포트 충돌 없음), 원격 자가 업데이트의 pre-flight 스모크
// 테스트로 안전하게 사용된다 — 교체 전에 후보 바이너리가 기동 가능한지(아키텍처/링크/
// 설정 호환) 확인해, 깨진 빌드로의 교체를 다운타임 없이 차단한다.
package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/xtra/xflow/internal/config"
)

func newVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify",
		Short: "설정 로드/초기화를 검증하고 종료합니다 (원격 업데이트 pre-flight 스모크 테스트)",
		Long: "서버/에이전트/포트 바인딩 없이 설정만 로드·검증하고 종료합니다. 성공 시 exit 0.\n" +
			"원격 자가 업데이트가 교체 전에 후보 바이너리를 `verify` 로 실행해 기동 가능성을 확인합니다.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// 루트의 persistent --config 플래그를 상속받는다.
			configFile, _ := cmd.Flags().GetString("config")
			return runVerify(configFile)
		},
	}
}

// runVerify 는 설정을 로드해 검증한다. 로드 성공이면 nil(exit 0), 실패면 오류(비0).
func runVerify(configFile string) error {
	var loadOpts []config.LoadOption
	if configFile != "" {
		loadOpts = append(loadOpts, config.WithConfigFile(configFile))
	}
	if _, err := config.Load(loadOpts...); err != nil {
		return fmt.Errorf("설정 로딩/검증 실패: %w", err)
	}
	fmt.Println("ok")
	return nil
}
