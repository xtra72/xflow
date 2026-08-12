// 디바이스 속성 키를 한국어 라벨로 변환하는 유틸리티.

import type { DeviceInfo } from '@/types/device';

/**
 * 디바이스 표시명을 결정한다.
 *
 * SPEC-DEVICE-IDENTITY-001 Phase D (M11):
 * fallback 순서: metadata.name → name → agent_name (composite 또는 UUID 노출 방지).
 * Phase D (xflowd v1.0) 부터 `device.id` 는 UUID 를 반환하므로 사용자에게
 * 직접 노출하면 raw UUID 가 화면에 표시되는 문제가 발생한다. `agent_name`
 * 을 최종 fallback 으로 사용하면 사람이 읽을 수 있는 라벨이 보장된다.
 */
export function getDeviceDisplayName(device: DeviceInfo): string {
  if (device.metadata?.name) return device.metadata.name;
  if (device.name) return device.name;
  if (device.agent_name) return device.agent_name;
  return device.uid ?? device.id;
}

const SAMSUNG_NASA_INDOOR_LABELS: Record<string, string> = {
  power: '전원',
  mode: '운전 모드',
  target_temperature: '설정 온도',
  current_temperature: '현재 온도',
  current_humidity: '현재 습도', // NASA V1.1 지표 세트 #10 (0x4038)
  fan_speed: '풍량',
  swing_vertical: '상하 스윙',
  filter_alarm: '필터 알람',
  error_code: '에러 코드',
};

const SAMSUNG_NASA_OUTDOOR_LABELS: Record<string, string> = {
  power: '전원',
  current_temperature: '현재 온도',
  error_code: '에러 코드',
  // 실외기(ODU) 텔레메트리 — internal/agent/samsung/outdoor.go outdoorFieldRegistry 와 대응.
  // 온도(°C) 계열
  outdoor_temperature: '실외 온도',
  compressor_discharge_temperature: '압축기 토출 온도',
  out_sensor_pipein3: '파이프 입구 온도 3',
  out_sensor_pipein4: '파이프 입구 온도 4',
  out_sensor_pipein5: '파이프 입구 온도 5',
  out_sensor_pipeout1: '파이프 출구 온도 1',
  out_sensor_pipeout2: '파이프 출구 온도 2',
  out_sensor_pipeout3: '파이프 출구 온도 3',
  out_sensor_pipeout4: '파이프 출구 온도 4',
  out_sensor_pipeout5: '파이프 출구 온도 5',
  // 운전 상태(ENUM)
  out_operation_odu_mode: '실외기 운전 상태',
  out_operation_heatcool: '냉난방',
  out_load_comp1: '압축기1',
  out_load_comp2: '압축기2',
  out_load_comp3: '압축기3',
  out_load_4way: '4-way 밸브',
  out_deice_step: '제상 단계',
  // 압축기 주파수(raw Hz)
  out_control_order_cfreq_comp2: '압축기2 지령 주파수',
  out_control_target_cfreq_comp2: '압축기2 목표 주파수',
  // 전기/전력(raw)
  out_sensor_ct1: '실외기 전류(CT1)',
  out_phase_current: '상 전류',
  out_sensor_voltage: '공급 전압',
  wattmeter_1min_sum: '순시 소비전력',
  wattmeter_all_unit_accum: '누적 전력량',
};

const MODBUS_LABELS: Record<string, string> = {
  host: '호스트',
  port: '포트',
  unit_id: '유닛 ID',
  connected: '연결 상태',
  temperature: '온도',
  humidity: '습도',
};

const LGAP_LABELS: Record<string, string> = {
  power: '전원',
  mode: '운전 모드',
  target_temperature: '설정 온도',
  current_temperature: '현재 온도',
  fan_speed: '풍량',
  swing_auto: '스윙 자동',
  locked: '잠금',
  plasma: '플라즈마',
  pipe_in_temperature: '입구 온도',
  pipe_out_temperature: '출구 온도',
  zone_load: '존 부하',
  zone_power: '존 전원',
  error_code: '에러 코드',
};

const LG_ICP02_LABELS: Record<string, string> = {
  power: '전원',
  mode: '운전 모드',
  target_temperature: '설정 온도',
  current_temperature: '현재 온도',
  fan_speed: '풍량',
  fan_motor_hz: '팬 모터 (Hz)',
  valve_open: '밸브 개도',
  pipe_temperature1_c: '배관 온도 1',
  pipe_temperature2_c: '배관 온도 2',
  compressor_cap: '압축기 용량',
  compressor_hz: '압축기 (Hz)',
  compressor_run: '압축기 가동',
  outdoor_active: '실외기 가동',
  heat_demand: '난방 요구',
  refrigerant_on: '냉매 순환',
  op_mode: '운전 상태',
};

const LG_ICP01_LABELS: Record<string, string> = {
  power: '전원',
  mode: '운전 모드',
  fan_speed: '풍량',
  target_temperature: '설정 온도',
  current_temperature: '현재 온도',
  inlet_temperature: '흡입 온도',
  outlet_temperature: '토출 온도',
  outdoor_temperature: '외기 온도',
  compressor_suction_temperature: '압축기 흡입 온도',
  compressor_discharge_temperature: '압축기 토출 온도',
  condenser_temperature_a: '응축기 온도 A',
  condenser_temperature_b: '응축기 온도 B',
  avg_temperature: '운전 평균 온도',
};

const COMMON_LABELS: Record<string, string> = {
  power: '전원',
  status: '상태',
  error_code: '에러 코드',
  online: '온라인',
  temperature: '온도',
  humidity: '습도',
  current_temperature: '현재 온도',
  target_temperature: '설정 온도',
};

/** 디바이스 타입을 한글 표시명으로 변환. v0.18.3: HVACR.IDU/HVACR.ODU 신규 + 'indoor'/'outdoor' 레거시 호환. */
export function getDeviceTypeLabel(type: string): string {
  switch (type) {
    case 'HVACR.IDU':
    case 'indoor': // legacy
      return '실내기';
    case 'HVACR.ODU':
    case 'outdoor': // legacy
      return '실외기';
    case 'sensor':
      return '센서';
    case 'controller':
      return '컨트롤러';
    case 'gateway':
      return '게이트웨이';
    default:
      return type;
  }
}

/** 명령 이름 → 한국어 라벨 */
const COMMAND_LABELS: Record<string, string> = {
  target_temperature: '온도 설정',
  set_mode: '운전 모드',
  set_power: '전원',
  set_fan_speed: '풍량',
  write_register: '레지스터 쓰기',
  write_coil: '코일 쓰기',
};

/** 파라미터 이름 → 한국어 라벨 */
const PARAM_LABELS: Record<string, string> = {
  target_temperature: '설정 온도',
  mode: '모드',
  power: '전원',
  fan_speed: '풍량',
  address: '주소',
  value: '값',
};

/** 열거형 값 → 한국어 라벨 */
const ENUM_LABELS: Record<string, string> = {
  auto: '자동',
  cool: '냉방',
  cooling: '냉방',
  dry: '제습',
  dehumidify: '제습',
  fan: '송풍',
  heat: '난방',
  heating: '난방',
  low: '약',
  medium: '중',
  high: '강',
  quiet: '미풍',
  turbo: '터보',
};

/** snake_case 키를 Title Case로 변환. 예: target_temperature → Set Temperature */
export function humanizeKey(key: string): string {
  return key
    .split('_')
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(' ');
}

/** 명령 이름을 한국어 라벨로 변환. */
export function getCommandLabel(name: string): string {
  return COMMAND_LABELS[name] ?? humanizeKey(name);
}

/** 파라미터 이름을 한국어 라벨로 변환. */
export function getParamLabel(name: string): string {
  return PARAM_LABELS[name] ?? humanizeKey(name);
}

/** 열거형 값을 한국어 라벨로 변환. */
export function getEnumLabel(value: string): string {
  return ENUM_LABELS[value] ?? value;
}

/** 속성 키를 한국어 라벨로 변환. 알 수 없는 키는 Title Case로 변환. */
export function getPropertyLabel(key: string, protocol?: string, type?: string): string {
  if (protocol === 'samsung_nasa' && (type === 'HVACR.IDU' || type === 'indoor')) {
    const label = SAMSUNG_NASA_INDOOR_LABELS[key];
    if (label) return label;
  }
  if (protocol === 'samsung_nasa') {
    const label = SAMSUNG_NASA_OUTDOOR_LABELS[key];
    if (label) return label;
  }
  if (protocol === 'modbus') {
    const label = MODBUS_LABELS[key];
    if (label) return label;
  }
  if (protocol === 'lgap') {
    const label = LGAP_LABELS[key];
    if (label) return label;
  }
  if (protocol === 'lg_icp02') {
    const label = LG_ICP02_LABELS[key];
    if (label) return label;
  }
  if (protocol === 'lg_icp01') {
    const label = LG_ICP01_LABELS[key];
    if (label) return label;
  }
  return COMMON_LABELS[key] ?? humanizeKey(key);
}

/** 속성 표시 우선순위. 목록에 없는 키는 맨 뒤에 원래 순서대로 표시. */
const PROPERTY_ORDER: string[] = [
  // 제어 순서: 전원 → 운전 모드 → 온도 → 풍량 → 고정 설치
  'power',
  'mode',
  'target_temperature',
  'current_temperature',
  'fan_speed',
  // 고정 설치/상태
  'swing_vertical',
  'swing_auto',
  'locked',
  'plasma',
  'filter_alarm',
  // 센서/배관
  'fan_motor_hz',
  'valve_open',
  'pipe_temperature1_c',
  'pipe_temperature2_c',
  'pipe_in_temperature',
  'pipe_out_temperature',
  'zone_load',
  'zone_power',
  // 컨트롤러/실외기
  'compressor_cap',
  'compressor_hz',
  'compressor_run',
  'outdoor_active',
  'heat_demand',
  'refrigerant_on',
  'op_mode',
  // Samsung 실외기(ODU) 텔레메트리 — outdoorFieldRegistry 순서.
  'out_operation_odu_mode',
  'out_operation_heatcool',
  'out_load_comp1',
  'out_load_comp2',
  'out_load_comp3',
  'out_load_4way',
  'out_deice_step',
  'outdoor_temperature',
  'compressor_discharge_temperature',
  'out_sensor_pipein3',
  'out_sensor_pipein4',
  'out_sensor_pipein5',
  'out_sensor_pipeout1',
  'out_sensor_pipeout2',
  'out_sensor_pipeout3',
  'out_sensor_pipeout4',
  'out_sensor_pipeout5',
  'out_control_order_cfreq_comp2',
  'out_control_target_cfreq_comp2',
  'out_sensor_ct1',
  'out_phase_current',
  'out_sensor_voltage',
  'wattmeter_1min_sum',
  'wattmeter_all_unit_accum',
  // 에러
  'error_code',
];

/** 속성 엔트리를 표시 우선순위에 따라 정렬한다. */
/** 속성값을 사람이 읽을 수 있는 문자열로 포맷. */
// v0.7.5+ hvac 통일 ID (int) → 영문 enum 키. mode / fan_speed 가 백엔드에서
// int 로 emit 되므로 한국어 라벨 변환 전에 enum 키로 정규화한다.
const MODE_ID_TO_ENUM: Record<number, string> = {
  0: 'auto', 1: 'cool', 2: 'heat', 3: 'dry', 4: 'fan',
};
const FAN_SPEED_ID_TO_ENUM: Record<number, string> = {
  0: 'auto', 1: 'auto', 2: 'quiet', 3: 'low', 4: 'medium', 5: 'high', 6: 'turbo',
};

// 전원 OFF 시 값이 신뢰할 수 없는(자동/0°C 등으로 정규화된 기본값) 운전 계열 속성 키.
// power=false 이면 이 필드들은 실제 값이 아니므로 '-' 로 표시한다. 진단 필드
// (filter_alarm/error_code)와 전원(power) 자체는 전원과 무관한 상태라 그대로 표시한다.
const POWER_OFF_UNRELIABLE_KEYS = new Set<string>([
  'mode',
  'target_temperature',
  'current_temperature',
  'fan_speed',
  'swing_vertical',
]);

export function formatPropertyValue(
  key: string,
  value: unknown,
  opts?: { powerOff?: boolean },
): string {
  // 전원 OFF 시 운전 계열 값은 정규화된 기본값이라 실제 값이 아니므로 '-' 로 표시.
  if (opts?.powerOff && POWER_OFF_UNRELIABLE_KEYS.has(key)) return '-';
  if (value === null || value === undefined) return '-';
  if (typeof value === 'number') {
    // mode / fan_speed 는 hvac 통일 ID (int) — enum 키로 변환 후 라벨링.
    if (key === 'mode') {
      const enumKey = MODE_ID_TO_ENUM[value];
      if (enumKey) return ENUM_LABELS[enumKey] ?? enumKey;
    }
    if (key === 'fan_speed') {
      const enumKey = FAN_SPEED_ID_TO_ENUM[value];
      if (enumKey) return ENUM_LABELS[enumKey] ?? enumKey;
    }
    const lowerKey = key.toLowerCase();
    if (lowerKey.includes('temp')) return `${value}\u00B0C`;
    return String(value);
  }
  if (typeof value === 'boolean') return value ? 'ON' : 'OFF';
  // 중첩 객체/배열은 String() 이 "[object Object]" 를 만들므로 compact JSON 으로 폴백한다.
  // (예: chirpstack 의 measurements 처럼 예상치 못한 중첩 값이 들어오는 경우)
  // 측정치는 expandMeasurementEntries 로 개별 항목으로 펼치는 것이 우선이며, 이 분기는
  // 그 경로를 타지 않는 모든 위치(이력 테이블 셀 등)를 위한 방어적 폴백이다.
  if (typeof value === 'object') return stringifyUnknownObject(value);
  const str = String(value);
  // 운전 모드, 풍량 등 enum 값을 한국어로 변환
  return ENUM_LABELS[str] ?? str;
}

/** 객체 폴백 표시 최대 길이. 초과분은 말줄임한다(그리드 셀/표 셀 레이아웃 보호). */
const OBJECT_FALLBACK_MAX_LEN = 80;

/**
 * 알 수 없는 객체/배열을 compact JSON 문자열로 변환한다.
 * 순환 참조 등으로 직렬화가 실패하면 타입 표기로 폴백한다(예외를 던지지 않는다).
 */
function stringifyUnknownObject(value: object): string {
  let json: string;
  try {
    json = JSON.stringify(value);
  } catch {
    return Array.isArray(value) ? '[…]' : '{…}';
  }
  // JSON.stringify 는 undefined/함수 등에서 undefined 를 반환할 수 있다.
  if (json === undefined) return '-';
  if (json.length <= OBJECT_FALLBACK_MAX_LEN) return json;
  return `${json.slice(0, OBJECT_FALLBACK_MAX_LEN)}…`;
}

/** 속성 그리드에 표시할 단일 항목. measurements 는 측정치별로 펼쳐진다. */
export interface PropertyEntry {
  /** React key 용 고유 식별자 (measurements 전개 항목은 "measurements.<이름>"). */
  id: string;
  /** 라벨/포맷 조회에 사용할 키 (측정치 이름 또는 원본 속성 키). */
  key: string;
  value: unknown;
  /** 측정치별 갱신 시각(epoch ms). 값이 없거나 형식이 다르면 undefined. */
  timeMs?: number;
}

/** measurements 하위 값이 {value, time_ms} 형태인지 판별한다. */
function isMeasurementRecord(v: unknown): v is { value: unknown; time_ms?: unknown } {
  return typeof v === 'object' && v !== null && !Array.isArray(v) && 'value' in v;
}

/**
 * 속성 엔트리 목록에서 `measurements` 를 측정치별 개별 항목으로 펼친다.
 *
 * chirpstack 디바이스 로스터는 측정치를 다음 형태로 내보낸다:
 *   measurements: { temperature: { value: 29.8, time_ms: 1786491121129 }, ... }
 * 이를 하나의 불투명한 카드가 아니라 측정치별 카드로 렌더하기 위한 순수 변환이며,
 * DeviceDetailPanel(GenericPropertiesGrid)과 대시보드 PropertiesGridPanel 이 공유한다.
 *
 * 예외 상황 처리:
 *   - measurements 없음 → 원본 그대로
 *   - measurements 가 빈 객체 → 항목 0개 (빈 카드/[object Object] 를 만들지 않음)
 *   - measurements 가 객체가 아님(문자열 등) → 원본 항목 그대로 유지
 *   - 하위 값이 {value, time_ms} 가 아닌 단순 스칼라 → 값만 사용하고 시각은 생략
 */
export function expandMeasurementEntries(entries: [string, unknown][]): PropertyEntry[] {
  const out: PropertyEntry[] = [];
  for (const [key, value] of entries) {
    if (key !== 'measurements' || typeof value !== 'object' || value === null || Array.isArray(value)) {
      out.push({ id: key, key, value });
      continue;
    }
    for (const [name, raw] of Object.entries(value as Record<string, unknown>)) {
      if (isMeasurementRecord(raw)) {
        out.push({
          id: `measurements.${name}`,
          key: name,
          value: raw.value,
          timeMs: typeof raw.time_ms === 'number' ? raw.time_ms : undefined,
        });
      } else {
        // 구형 데이터/타 프로바이더: 시각 없이 값만 있는 형태.
        out.push({ id: `measurements.${name}`, key: name, value: raw });
      }
    }
  }
  return out;
}

export function sortProperties<T>(entries: [string, T][]): [string, T][] {
  return entries.slice().sort((a, b) => {
    const ia = PROPERTY_ORDER.indexOf(a[0]);
    const ib = PROPERTY_ORDER.indexOf(b[0]);
    // 목록에 없는 키는 뒤로
    const oa = ia === -1 ? PROPERTY_ORDER.length : ia;
    const ob = ib === -1 ? PROPERTY_ORDER.length : ib;
    return oa - ob;
  });
}

/** 커맨드 표시 우선순위: 전원 → 운전 모드 → 온도 → 풍량 → 고정 설치 */
const COMMAND_ORDER: string[] = [
  'set_power',
  'set_mode',
  'target_temperature',
  'set_fan_speed',
  'set_swing',
  'set_lock',
  'set_plasma',
];

/** 커맨드를 표시 우선순위에 따라 정렬한다. */
export function sortCommands<T extends { name: string }>(commands: T[]): T[] {
  return commands.slice().sort((a, b) => {
    const ia = COMMAND_ORDER.indexOf(a.name);
    const ib = COMMAND_ORDER.indexOf(b.name);
    const oa = ia === -1 ? COMMAND_ORDER.length : ia;
    const ob = ib === -1 ? COMMAND_ORDER.length : ib;
    return oa - ob;
  });
}
