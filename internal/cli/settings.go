package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// newSettingsCmd 는 서버 전역(앱 전체 공유) KV 설정 커맨드 그룹을 생성한다.
//
// 주의: 이 명령은 "서버측 전역 설정"(GET/PUT /api/v1/settings/{key})을 다룬다.
// 로컬 클라이언트 설정(서버 주소, 토큰, 출력 형식 등)을 다루는 `xflow config` 와는
// 완전히 별개이다. 전역 설정은 모든 클라이언트가 공유하며 서버 DB 에 영속화된다.
//
// 서브커맨드:
//
//	get <key>          GET  /api/v1/settings/{key}  → 저장된 value 조회
//	set <key> <value>  PUT  /api/v1/settings/{key}  → value 저장(UPSERT)
func newSettingsCmd(client **Client) *cobra.Command {
	settingsCmd := &cobra.Command{
		Use:   "settings",
		Short: "서버 전역 설정 관리 명령어",
		Long: "서버측 전역(앱 전체 공유) key-value 설정을 조회/저장합니다.\n\n" +
			"이 설정은 서버에 영속화되어 모든 클라이언트가 공유합니다.\n" +
			"로컬 클라이언트 설정(서버 주소, 토큰, 출력 형식)을 다루는 'xflow config' 와는 별개입니다.",
	}

	settingsCmd.AddCommand(newSettingsGetCmd(client))
	settingsCmd.AddCommand(newSettingsSetCmd(client))

	return settingsCmd
}

// settingsValue 는 settings API 응답 데이터(`{key, value}`)를 파싱하기 위한 구조이다.
//
// Value 는 서버가 불투명 JSON 으로 저장한 원본을 그대로 받기 위해 json.RawMessage 로
// 둔다(이중 인코딩 방지 — 받은 그대로 재파싱/출력 가능).
type settingsValue struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

// newSettingsGetCmd 는 settings get <key> 서브커맨드를 생성한다.
// GET /api/v1/settings/{key} 로 전역 설정 value 를 조회한다.
func newSettingsGetCmd(client **Client) *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "서버 전역 설정 값 조회",
		Long: "서버 전역 설정에서 key 에 해당하는 value(JSON)를 조회합니다.\n" +
			"로컬 클라이언트 설정이 아닌 서버측 전역 설정입니다('xflow config' 와 별개).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]

			var result settingsValue
			if err := (*client).Get("/api/v1/settings/"+key, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			// value 는 불투명 JSON 이므로, 출력 형식에 맞춰 파싱 후 렌더링한다.
			// 파싱 실패 시(이론상 발생 안 함) 원본 문자열을 그대로 보여준다.
			var decoded any
			if err := json.Unmarshal(result.Value, &decoded); err != nil {
				fmt.Fprintln(w, string(result.Value))
				return nil
			}
			return PrintResult(w, format, decoded, nil, nil)
		},
	}
}

// newSettingsSetCmd 는 settings set <key> <value> 서브커맨드를 생성한다.
// PUT /api/v1/settings/{key} 로 전역 설정 value 를 저장(UPSERT)한다.
//
// value 처리:
//   - 기본: <value> 인자를 JSON 문자열로 인코딩하여 저장한다(예: hello → "hello").
//   - --json: <value> 인자를 이미 유효한 JSON(객체/배열/숫자 등)으로 간주해 원본 그대로
//     전달한다(예: '{"a":1}', '[1,2]', '42', 'true').
func newSettingsSetCmd(client **Client) *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "서버 전역 설정 값 저장 (UPSERT)",
		Long: "서버 전역 설정에 key=value 를 저장합니다(이미 있으면 덮어쓰기).\n" +
			"로컬 클라이언트 설정이 아닌 서버측 전역 설정입니다('xflow config' 와 별개).\n\n" +
			"기본적으로 value 는 JSON 문자열로 저장됩니다(hello → \"hello\").\n" +
			"--json 플래그를 사용하면 value 를 이미 유효한 JSON 으로 간주해 그대로 전달합니다\n" +
			"(예: --json '{\"columns\":[\"id\",\"name\"]}').",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			rawValue := args[1]

			// body 는 서버가 불투명 JSON value 로 저장하는 "value 자체"이다
			// (서버 핸들러는 body 를 그대로 받아 json.Valid 로만 검증한다 — {"value": ...}
			// 래핑이 아니다).
			var body any
			if asJSON {
				// 사용자가 넘긴 문자열이 유효한 JSON 인지 먼저 검증한다.
				if !json.Valid([]byte(rawValue)) {
					return ErrInvalidInput("--json 값이 유효한 JSON 이 아닙니다")
				}
				// 원본 JSON 을 그대로 전달(이중 인코딩 방지).
				body = json.RawMessage(rawValue)
			} else {
				// 평범한 문자열은 JSON 문자열로 인코딩하여 전달(Client.Put 이 직렬화).
				body = rawValue
			}

			var result settingsValue
			if err := (*client).Put("/api/v1/settings/"+key, body, &result); err != nil {
				return err
			}

			format := getFormat(cmd)
			w := cmd.OutOrStdout()

			if format == "table" || format == "text" {
				fmt.Fprintf(w, "전역 설정 '%s' 가 저장되었습니다.\n", key)
				return nil
			}

			// json/yaml 형식은 저장된 value 를 파싱하여 출력한다.
			var decoded any
			if err := json.Unmarshal(result.Value, &decoded); err != nil {
				return PrintResult(w, format, string(result.Value), nil, nil)
			}
			return PrintResult(w, format, decoded, nil, nil)
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "value 를 이미 유효한 JSON 으로 간주하여 원본 그대로 전달")

	return cmd
}
