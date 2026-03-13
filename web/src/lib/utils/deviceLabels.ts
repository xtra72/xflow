// 디바이스 속성 키를 한국어 라벨로 변환하는 유틸리티.

const NASA_INDOOR_LABELS: Record<string, string> = {
  power: '전원',
  mode: '운전 모드',
  target_temp: '설정 온도',
  current_temp: '현재 온도',
  fan_speed: '풍량',
  swing_vertical: '상하 스윙',
  filter_alarm: '필터 알람',
  error_code: '에러 코드',
};

const NASA_OUTDOOR_LABELS: Record<string, string> = {
  power: '전원',
  current_temp: '현재 온도',
  error_code: '에러 코드',
};

const MODBUS_LABELS: Record<string, string> = {
  host: '호스트',
  port: '포트',
  unit_id: '유닛 ID',
  connected: '연결 상태',
  temperature: '온도',
  humidity: '습도',
};

const COMMON_LABELS: Record<string, string> = {
  power: '전원',
  status: '상태',
  error_code: '에러 코드',
  online: '온라인',
  temperature: '온도',
  humidity: '습도',
  current_temp: '현재 온도',
  target_temp: '설정 온도',
};

/** 디바이스 타입을 한글 표시명으로 변환. */
export function getDeviceTypeLabel(type: string): string {
  switch (type) {
    case 'indoor':
      return '실내기';
    case 'outdoor':
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

/** 속성 키를 한국어 라벨로 변환. 알 수 없는 키는 원본 그대로 반환. */
export function getPropertyLabel(key: string, protocol?: string, type?: string): string {
  if (protocol === 'nasa' && type === 'indoor') {
    const label = NASA_INDOOR_LABELS[key];
    if (label) return label;
  }
  if (protocol === 'nasa') {
    const label = NASA_OUTDOOR_LABELS[key];
    if (label) return label;
  }
  if (protocol === 'modbus') {
    const label = MODBUS_LABELS[key];
    if (label) return label;
  }
  return COMMON_LABELS[key] ?? key;
}
