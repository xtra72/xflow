# Plan: Device UI 3 Improvements

## Context

Device 페이지에 3가지 UI 문제:
1. 오프라인 디바이스의 마지막 통신 시간이 "739686일 전"으로 표시됨 (Go zero time `0001-01-01T00:00:00Z` 미처리)
2. 속성 키가 `current_temp` 같은 snake_case 원본으로 표시됨 (한국어 라벨 필요)
3. 상태 속성이 flat grid 카드 형태 (리모컨 형태 UI로 변경)

## Changes

### 1. `web/src/pages/devices/DeviceListPage.tsx` - zero-time 버그 수정

`formatRelativeTime` 함수(L18-38)에 year < 2000 가드 추가:

```typescript
function formatRelativeTime(dateStr: string): string {
  if (!dateStr) return '-';
  const then = new Date(dateStr).getTime();
  if (isNaN(then)) return '-';
  // Go zero time ("0001-01-01T00:00:00Z") 처리
  if (new Date(dateStr).getUTCFullYear() < 2000) return '-';
  // ... 이하 동일
}
```

### 2. `web/src/lib/utils/deviceLabels.ts` - 속성 라벨 맵 (신규)

```typescript
export function getPropertyLabel(key: string, protocol?: string, type?: string): string
```

- NASA Indoor: `power`->`전원`, `mode`->`운전 모드`, `target_temp`->`설정 온도`, `current_temp`->`현재 온도`, `fan_speed`->`풍량`, `swing_vertical`->`상하 스윙`, `filter_alarm`->`필터 알람`, `error_code`->`에러 코드`
- Modbus: `host`->`호스트`, `port`->`포트`, `unit_id`->`유닛 ID`
- 폴백: 알 수 없는 키는 원본 그대로

### 3. `web/src/pages/devices/DeviceDetailPanel.tsx` - 리모컨 레이아웃

`StatePropertiesSection`을 분기 컴포넌트로 교체:
- `protocol === 'nasa' && type === 'indoor'` -> `NasaIndoorRemoteControl`
- 그 외 -> `GenericPropertiesGrid` (기존 grid + 라벨 적용)

**NasaIndoorRemoteControl 레이아웃:**

```
┌─────────────────────────────┐
│  [POWER ON/OFF]   에러코드   │  헤더
├─────────────────────────────┤
│        26°C                  │  현재 온도 (large)
│     현재 온도                │
│   설정 온도: 24°C            │
├─────────────────────────────┤
│  [냉방] [난방] [자동]        │  운전 모드 배지
│  [제습] [팬]                 │  (활성 모드 강조)
├─────────────────────────────┤
│  풍량: [약] [중] [강] [자동] │  팬 속도
├─────────────────────────────┤
│  [스윙: ON]  [필터: 정상]    │  상태 인디케이터
└─────────────────────────────┘
```

- lucide-react 아이콘 활용: Power, Snowflake, Flame, Wind, Droplets, Thermometer, Filter, ChevronsUpDown
- 모드별 색상: cool=blue, heat=orange, auto=green, dry=cyan, fan=purple
- `max-w-sm`, 다크모드 지원

## Files

| File | Action |
|------|--------|
| `web/src/lib/utils/deviceLabels.ts` | New - property label map |
| `web/src/pages/devices/DeviceListPage.tsx` | Fix - zero-time guard in formatRelativeTime |
| `web/src/pages/devices/DeviceDetailPanel.tsx` | Refactor - remote control layout + labels |

## Verification

1. `npm run build` - 컴파일 확인
2. 오프라인 디바이스 "마지막 통신" 열이 `-`로 표시되는지 확인
3. NASA 실내기 상세 패널이 리모컨 형태로 표시되는지 확인
4. Modbus/기타 디바이스는 기존 grid + 한국어 라벨로 표시되는지 확인
