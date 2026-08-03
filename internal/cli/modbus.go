package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// registerAreaAliases 는 레지스터 영역의 별칭을 정규 이름으로 매핑한다.
var registerAreaAliases = map[string]string{
	"holding":           "holding_registers",
	"hr":                "holding_registers",
	"input":             "input_registers",
	"ir":                "input_registers",
	"coils":             "coils",
	"c":                 "coils",
	"discrete":          "discrete_inputs",
	"di":                "discrete_inputs",
	"holding_registers": "holding_registers",
	"input_registers":   "input_registers",
	"discrete_inputs":   "discrete_inputs",
}

// writableAreas 는 쓰기 가능한 레지스터 영역을 정의한다.
var writableAreas = map[string]bool{
	"holding_registers": true,
	"coils":             true,
}

const (
	modbusAgentTypeClient = "client"
	modbusAgentTypeServer = "server"
	modbusClientType      = "modbus-client"
	modbusServerType      = "modbus-gateway"
)

// resolveModbusAgentType 는 에이전트 ID로 MODBUS 에이전트 타입을 조회한다.
// "client" 또는 "server" 를 반환하며, MODBUS 타입이 아니면 에러를 반환한다.
func resolveModbusAgentType(client *Client, agentID string) (string, error) {
	var agent map[string]any
	if err := client.Get("/api/v1/agents/"+agentID, &agent); err != nil {
		return "", fmt.Errorf("에이전트 조회 실패: %w", err)
	}
	agentType, _ := agent["type"].(string)
	switch agentType {
	case modbusClientType:
		return modbusAgentTypeClient, nil
	case modbusServerType:
		return modbusAgentTypeServer, nil
	default:
		return "", fmt.Errorf("에이전트 '%s'은(는) MODBUS 타입이 아닙니다 (type: %s)", agentID, agentType)
	}
}

// normalizeRegisterArea 는 레지스터 영역 별칭을 정규 이름으로 변환한다.
func normalizeRegisterArea(alias string) (string, error) {
	if area, ok := registerAreaAliases[alias]; ok {
		return area, nil
	}
	return "", fmt.Errorf("알 수 없는 레지스터 영역: %q (사용 가능: holding|hr, input|ir, coils|c, discrete|di)", alias)
}

// newModbusCmd 는 modbus 서브커맨드를 생성한다.
// read, write, status, map 하위 커맨드를 포함한다.
func newModbusCmd(client **Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "modbus",
		Short: "MODBUS 에이전트 제어",
		Long:  "MODBUS 에이전트의 레지스터를 읽기/쓰기하고 상태를 조회합니다.",
	}

	cmd.AddCommand(newModbusReadCmd(client))
	cmd.AddCommand(newModbusWriteCmd(client))
	cmd.AddCommand(newModbusStatusCmd(client))
	cmd.AddCommand(newModbusMapCmd(client))

	return cmd
}

// newModbusReadCmd 는 modbus read 서브커맨드를 생성한다.
func newModbusReadCmd(client **Client) *cobra.Command {
	var (
		name     string
		register string
		address  int
		quantity int
		dataType string
		device   string
		force    bool
		unitID   int
	)

	cmd := &cobra.Command{
		Use:   "read [agent]",
		Short: "레지스터 읽기",
		Long: `MODBUS 에이전트에서 레지스터를 읽습니다.

예시:
  xflow modbus read my-server -r holding -a 0 -q 10
  xflow modbus read my-server -r hr -a 0 -q 5 -u 2
  xflow modbus read my-client -r hr -a 0 -q 5 -d 1
  xflow modbus read my-server -r input -a 100 -t float32`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// 에이전트 ID/이름 결정
			idOrName, err := resolveEntityArg(args, name)
			if err != nil {
				return err
			}

			id, err := resolveAgentID(*client, idOrName)
			if err != nil {
				return err
			}

			// 에이전트 타입 판별
			agentRole, err := resolveModbusAgentType(*client, id)
			if err != nil {
				return err
			}

			// 레지스터 영역 정규화
			area, err := normalizeRegisterArea(register)
			if err != nil {
				return err
			}

			// 커맨드 및 파라미터 구성
			body, err := buildReadCommand(agentRole, area, address, quantity, dataType, device, force, unitID)
			if err != nil {
				return err
			}

			// API 호출
			var result map[string]any
			path := fmt.Sprintf("/api/v1/agents/%s/exec", id)
			if err := (*client).Post(path, body, &result); err != nil {
				return err
			}

			// 출력
			return formatModbusResult(cmd, result)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "에이전트 이름으로 지정")
	cmd.Flags().StringVarP(&register, "register", "r", "holding", "레지스터 영역 (holding|hr, input|ir, coils|c, discrete|di)")
	cmd.Flags().IntVarP(&address, "address", "a", 0, "시작 주소")
	cmd.Flags().IntVarP(&quantity, "quantity", "q", 1, "레지스터 수")
	cmd.Flags().StringVarP(&dataType, "type", "t", "", "데이터 타입 (uint16, int16, uint32, int32, float32, float64)")
	cmd.Flags().StringVarP(&device, "device", "d", "", "디바이스 ID (Client 에이전트 전용, 필수)")
	cmd.Flags().BoolVar(&force, "force", false, "직접 읽기 강제 (Client 에이전트 전용)")
	cmd.Flags().IntVarP(&unitID, "unit-id", "u", 0, "유닛 ID (Server 에이전트 멀티 디바이스, 1-247)")

	return cmd
}

// buildReadCommand 는 read 커맨드의 요청 본문을 구성한다.
func buildReadCommand(agentRole, area string, address, quantity int, dataType, device string, force bool, unitID int) (map[string]any, error) {
	if agentRole == modbusAgentTypeClient {
		if device == "" {
			return nil, ErrInvalidInput("Client 에이전트는 --device (-d) 플래그가 필수입니다")
		}

		params := map[string]any{
			"device_id": device,
			"register":  area,
			"address":   address,
			"quantity":  quantity,
		}

		if force {
			params["force"] = true
		}

		if dataType != "" {
			params["data_type"] = dataType
			params["byte_order"] = "big_endian"
		}

		return map[string]any{
			"command": "read_registers",
			"params":  params,
		}, nil
	}

	// Server 에이전트
	if dataType != "" && (area == "holding_registers" || area == "input_registers") {
		params := map[string]any{
			"area":       area,
			"address":    address,
			"data_type":  dataType,
			"byte_order": "big_endian",
		}
		if unitID > 0 {
			params["unit_id"] = unitID
		}
		return map[string]any{
			"command": "get_register_typed",
			"params":  params,
		}, nil
	}

	// 영역별 커맨드 매핑
	cmdName := serverReadCommand(area)

	params := map[string]any{
		"address":  address,
		"quantity": quantity,
	}
	if unitID > 0 {
		params["unit_id"] = unitID
	}
	return map[string]any{
		"command": cmdName,
		"params":  params,
	}, nil
}

// serverReadCommand 는 Server 에이전트의 레지스터 영역에 대한 읽기 커맨드 이름을 반환한다.
func serverReadCommand(area string) string {
	switch area {
	case "holding_registers":
		return "get_holding_registers"
	case "input_registers":
		return "get_input_registers"
	case "coils":
		return "get_coils"
	case "discrete_inputs":
		return "get_discrete_inputs"
	default:
		return "get_holding_registers"
	}
}

// newModbusWriteCmd 는 modbus write 서브커맨드를 생성한다.
func newModbusWriteCmd(client **Client) *cobra.Command {
	var (
		name     string
		register string
		address  int
		values   string
		dataType string
		device   string
		unitID   int
	)

	cmd := &cobra.Command{
		Use:   "write [agent]",
		Short: "레지스터 쓰기",
		Long: `MODBUS 에이전트에 레지스터 값을 씁니다.

예시:
  xflow modbus write my-server -r holding -a 0 -v 100
  xflow modbus write my-server -r holding -a 0 -v 100 -u 2
  xflow modbus write my-server -r coils -a 0 -v true,false,true
  xflow modbus write my-client -r hr -a 10 -v 100,200,300 -d 1`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// 에이전트 ID/이름 결정
			idOrName, err := resolveEntityArg(args, name)
			if err != nil {
				return err
			}

			id, err := resolveAgentID(*client, idOrName)
			if err != nil {
				return err
			}

			// 에이전트 타입 판별
			agentRole, err := resolveModbusAgentType(*client, id)
			if err != nil {
				return err
			}

			// 레지스터 영역 정규화
			area, err := normalizeRegisterArea(register)
			if err != nil {
				return err
			}

			// 쓰기 가능 여부 확인
			if !writableAreas[area] {
				return ErrInvalidInput(fmt.Sprintf("레지스터 영역 '%s'는 쓰기를 지원하지 않습니다", area))
			}

			// 값 필수 확인
			if values == "" {
				return ErrInvalidInput("--values (-v) 플래그는 필수입니다")
			}

			// 커맨드 및 파라미터 구성
			body, err := buildWriteCommand(agentRole, area, address, values, device, unitID)
			if err != nil {
				return err
			}

			// API 호출
			var result map[string]any
			path := fmt.Sprintf("/api/v1/agents/%s/exec", id)
			if err := (*client).Post(path, body, &result); err != nil {
				return err
			}

			// 출력
			return formatModbusResult(cmd, result)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "에이전트 이름으로 지정")
	cmd.Flags().StringVarP(&register, "register", "r", "holding", "레지스터 영역 (holding|hr, coils|c)")
	cmd.Flags().IntVarP(&address, "address", "a", 0, "시작 주소")
	cmd.Flags().StringVarP(&values, "values", "v", "", "쓸 값 (쉼표 구분, 필수)")
	cmd.Flags().StringVarP(&dataType, "type", "t", "", "데이터 타입")
	cmd.Flags().StringVarP(&device, "device", "d", "", "디바이스 ID (Client 에이전트 전용, 필수)")
	cmd.Flags().IntVarP(&unitID, "unit-id", "u", 0, "유닛 ID (Server 에이전트 멀티 디바이스, 1-247)")

	return cmd
}

// buildWriteCommand 는 write 커맨드의 요청 본문을 구성한다.
func buildWriteCommand(agentRole, area string, address int, rawValues, device string, unitID int) (map[string]any, error) {
	if agentRole == modbusAgentTypeClient && device == "" {
		return nil, ErrInvalidInput("Client 에이전트는 --device (-d) 플래그가 필수입니다")
	}

	// 값 파싱
	parsed := parseWriteValues(rawValues)
	isSingle := len(parsed) == 1

	if agentRole == modbusAgentTypeClient {
		return buildClientWriteCommand(area, address, device, parsed, isSingle)
	}
	return buildServerWriteCommand(area, address, parsed, isSingle, unitID)
}

// parseWriteValues 는 쉼표로 구분된 값 문자열을 파싱한다.
func parseWriteValues(raw string) []any {
	parts := strings.Split(raw, ",")
	result := make([]any, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, parseParamValue(p))
		}
	}
	return result
}

// buildServerWriteCommand 는 Server 에이전트의 쓰기 커맨드를 구성한다.
func buildServerWriteCommand(area string, address int, parsed []any, isSingle bool, unitID int) (map[string]any, error) {
	switch area {
	case "holding_registers":
		if isSingle {
			params := map[string]any{
				"address": address,
				"value":   parsed[0],
			}
			if unitID > 0 {
				params["unit_id"] = unitID
			}
			return map[string]any{
				"command": "set_register",
				"params":  params,
			}, nil
		}
		params := map[string]any{
			"address": address,
			"values":  parsed,
		}
		if unitID > 0 {
			params["unit_id"] = unitID
		}
		return map[string]any{
			"command": "set_registers",
			"params":  params,
		}, nil

	case "coils":
		boolValues, err := toBoolSlice(parsed)
		if err != nil {
			return nil, err
		}
		if isSingle {
			params := map[string]any{
				"address": address,
				"value":   boolValues[0],
			}
			if unitID > 0 {
				params["unit_id"] = unitID
			}
			return map[string]any{
				"command": "set_coil",
				"params":  params,
			}, nil
		}
		params := map[string]any{
			"address": address,
			"values":  boolValues,
		}
		if unitID > 0 {
			params["unit_id"] = unitID
		}
		return map[string]any{
			"command": "set_coils",
			"params":  params,
		}, nil

	default:
		return nil, ErrInvalidInput(fmt.Sprintf("레지스터 영역 '%s'는 쓰기를 지원하지 않습니다", area))
	}
}

// buildClientWriteCommand 는 Client 에이전트의 쓰기 커맨드를 구성한다.
func buildClientWriteCommand(area string, address int, device string, parsed []any, isSingle bool) (map[string]any, error) {
	switch area {
	case "holding_registers":
		if isSingle {
			return map[string]any{
				"command": "write_register",
				"params": map[string]any{
					"device_id": device,
					"address":   address,
					"value":     parsed[0],
				},
			}, nil
		}
		return map[string]any{
			"command": "write_registers",
			"params": map[string]any{
				"device_id": device,
				"address":   address,
				"values":    parsed,
			},
		}, nil

	case "coils":
		boolValues, err := toBoolSlice(parsed)
		if err != nil {
			return nil, err
		}
		if isSingle {
			return map[string]any{
				"command": "write_coil",
				"params": map[string]any{
					"device_id": device,
					"address":   address,
					"value":     boolValues[0],
				},
			}, nil
		}
		return map[string]any{
			"command": "write_coils",
			"params": map[string]any{
				"device_id": device,
				"address":   address,
				"values":    boolValues,
			},
		}, nil

	default:
		return nil, ErrInvalidInput(fmt.Sprintf("레지스터 영역 '%s'는 쓰기를 지원하지 않습니다", area))
	}
}

// toBoolSlice 는 any 슬라이스를 bool 슬라이스로 변환한다.
func toBoolSlice(values []any) ([]bool, error) {
	result := make([]bool, len(values))
	for i, v := range values {
		switch b := v.(type) {
		case bool:
			result[i] = b
		default:
			return nil, ErrInvalidInput(fmt.Sprintf("coils 영역에는 bool 값이 필요합니다 (인덱스 %d: %v)", i, v))
		}
	}
	return result, nil
}

// newModbusStatusCmd 는 modbus status 서브커맨드를 생성한다.
func newModbusStatusCmd(client **Client) *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "status [agent]",
		Short: "에이전트 상태 조회",
		Long: `MODBUS 에이전트의 상태를 조회합니다.

예시:
  xflow modbus status my-server
  xflow modbus status my-client --name my-client`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// 에이전트 ID/이름 결정
			idOrName, err := resolveEntityArg(args, name)
			if err != nil {
				return err
			}

			id, err := resolveAgentID(*client, idOrName)
			if err != nil {
				return err
			}

			// 에이전트 타입 확인
			_, err = resolveModbusAgentType(*client, id)
			if err != nil {
				return err
			}

			// get_status 커맨드 실행
			body := map[string]any{
				"command": "get_status",
			}

			var result map[string]any
			path := fmt.Sprintf("/api/v1/agents/%s/exec", id)
			if err := (*client).Post(path, body, &result); err != nil {
				return err
			}

			// 출력
			return formatModbusResult(cmd, result)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "에이전트 이름으로 지정")

	return cmd
}

// newModbusMapCmd 는 modbus map 서브커맨드를 생성한다.
// Server 에이전트에서만 사용할 수 있다.
func newModbusMapCmd(client **Client) *cobra.Command {
	var (
		name   string
		unitID int
	)

	cmd := &cobra.Command{
		Use:   "map [agent]",
		Short: "레지스터 맵 조회 (Server 전용)",
		Long: `MODBUS Server 에이전트의 레지스터 맵을 조회합니다.
Client 에이전트에서는 사용할 수 없습니다.

예시:
  xflow modbus map my-server
  xflow modbus map my-server -u 2`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// 에이전트 ID/이름 결정
			idOrName, err := resolveEntityArg(args, name)
			if err != nil {
				return err
			}

			id, err := resolveAgentID(*client, idOrName)
			if err != nil {
				return err
			}

			// 에이전트 타입 확인 — Server 만 허용
			agentRole, err := resolveModbusAgentType(*client, id)
			if err != nil {
				return err
			}
			if agentRole != modbusAgentTypeServer {
				return ErrInvalidInput("`map` 명령은 Server 에이전트에서만 사용할 수 있습니다")
			}

			// get_map 커맨드 실행
			body := map[string]any{
				"command": "get_map",
			}
			if unitID > 0 {
				body["params"] = map[string]any{"unit_id": unitID}
			}

			var result map[string]any
			path := fmt.Sprintf("/api/v1/agents/%s/exec", id)
			if err := (*client).Post(path, body, &result); err != nil {
				return err
			}

			// 출력
			return formatModbusResult(cmd, result)
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "에이전트 이름으로 지정")
	cmd.Flags().IntVarP(&unitID, "unit-id", "u", 0, "유닛 ID (멀티 디바이스, 1-247)")

	return cmd
}

// formatModbusResult 는 MODBUS 커맨드 결과를 포맷에 맞게 출력한다.
func formatModbusResult(cmd *cobra.Command, result map[string]any) error {
	format := getFormat(cmd)
	w := cmd.OutOrStdout()

	if format == "table" {
		df := NewDetailFormatter(
			[]string{"ok", "command", "address", "quantity", "values", "value", "typed_values", "register_defs"},
			nil,
			map[string]bool{"typed_values": true, "register_defs": true, "values": false},
		)
		return df.Format(result, w)
	}
	return PrintResult(w, format, result, nil, nil)
}
