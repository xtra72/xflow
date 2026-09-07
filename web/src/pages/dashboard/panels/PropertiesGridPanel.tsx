// 디바이스 속성 그리드 패널.
// 디바이스의 속성을 설정 가능한 컬럼 수와 항목 선택으로 표시한다.

import { useEffect, useState } from 'react';
import { Activity, HardDrive, Moon } from 'lucide-react';

import { useDeviceDetailTarget } from '@/hooks/useDetailTargets';
import { useDeviceRealtime } from '@/hooks/useDevice';
import { useTranslation } from '@/lib/i18n';
import { isRemoteTarget } from '@/lib/remote/target';
import { useTargetContext } from '@/lib/remote/TargetContext';
import { cn } from '@/lib/utils/cn';
import {
  buildDisplayEntries,
  formatPropertyValue,
  getDeviceTypeLabel,
  getPropertyLabel,
  isDerivedPropertyKey,
  needsReception,
} from '@/lib/utils/deviceLabels';

import {
  applyPropertyOverride,
  groupEntries,
  isStaleValue,
  PROPERTY_GROUP_AREA_KEY,
  PROPERTY_GROUP_GRID,
  PROPERTY_GROUP_GRID_KEY,
  PROPERTY_GROUP_LABEL_KEYS,
  PROPERTY_TILE_SIZE,
  readPropertiesGridStyle,
  readPropertyOverride,
  reorderVisible,
  resolveTileLabel,
  resolveCardLayout,
  selectEntries,
  type CardArea,
  type CardAreas,
  type CardGrid,
  type PropertiesGridStyle,
} from './propertiesGridStyle';
import { formatEpochMs, formatRelativeEpochMs } from '@/lib/utils/format';
import { placeTiles, readTileGrid, type TileArea } from './tileLayout';
import { readTileDesign, readTileItems, resolveTileDesign } from './tileSelection';
import { usePanelEditing } from '../panelEditContext';
import { usePanelTitleStyle, usePanelTitleVisible } from '../panelChromeContext';

interface PropertiesGridPanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
}

export default function PropertiesGridPanel({
  panelId: _panelId,
  title,
  config,
  onConfigChange,
  onTitleChange: _onTitleChange,
}: PropertiesGridPanelProps) {
  const showTitle = usePanelTitleVisible();
  const titleStyle = usePanelTitleStyle();
  const { t } = useTranslation();
  const deviceId = config.deviceId as string | undefined;
  // 배치·글자 모양은 순수 모듈이 해석한다(열 수 한계, 알 수 없는 배치 폴백 포함).
  const design = readPropertiesGridStyle(config);
  const visibleProperties = (config.visibleProperties as string[] | undefined) ?? [];
  const panelColor = config.panelColor as string | undefined;
  const accentElements = config.accentElements as Record<string, string | boolean> | undefined;

  // 원격 대시보드 target(SPEC-REMOTE-001 M10, REQ-L04): 원격이면 device.state(그룹 J)로
  // 그 노드 디바이스의 속성을 읽는다(REQ-L03). 로컬은 기존 useDeviceRealtime 그대로.
  // 편집 중(설정 미리보기)에만 카드를 끌어 옮길 수 있다. 대시보드에서는 패널 자체를
  // 끄는 동작과 부딪히므로 켜지 않는다.
  const editing = usePanelEditing();
  const [dragKey, setDragKey] = useState<string | null>(null);

  // 카드 밖에서 손을 떼도 끌기가 끝나야 한다 — 카드 위 mouseup 만 듣고 있으면 커서가
  // 패널 밖으로 나간 채 놓였을 때 끌기 상태가 남아 다음 클릭이 카드를 옮긴다.
  useEffect(() => {
    if (!dragKey) return;
    const end = (): void => setDragKey(null);
    window.addEventListener('mouseup', end);
    return () => window.removeEventListener('mouseup', end);
  }, [dragKey]);

  const target = useTargetContext();
  const remote = isRemoteTarget(target);
  const localDevice = useDeviceRealtime(remote ? '' : deviceId ?? '');
  const remoteDevice = useDeviceDetailTarget(target, deviceId ?? '');
  const device = remote ? remoteDevice.data : localDevice.data;
  const isLoading = remote ? remoteDevice.isLoading : localDevice.isLoading;

  const acColor = (group: string): string | undefined => {
    if (!accentElements) return panelColor;
    const val = accentElements[group];
    if (val === false) return undefined;
    if (typeof val === 'string') return val;
    return panelColor;
  };

  if (!deviceId) {
    return (
      <div className="flex min-h-0 flex-1 flex-col items-center justify-center rounded-lg bg-(--color-bg-surface) p-4 shadow">
        <HardDrive className="mb-2 h-6 w-6 text-(--color-text-muted)" />
        <p className="text-xs text-(--color-text-muted)">{t('dashboard.panel.deviceNotConfigured')}</p>
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className="flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-4 shadow">
        {showTitle && (
          <div className="mb-2 flex shrink-0 items-center gap-2">
            <div className="h-2 w-2 rounded-full bg-gray-300" />
            <span className="truncate text-sm font-medium text-(--color-text-primary)" style={titleStyle}>{title}</span>
          </div>
        )}
        <div className="flex flex-1 items-center justify-center">
          <div
            className="h-5 w-5 animate-spin rounded-full border-2 border-(--color-border-strong) border-t-blue-600"
            style={panelColor ? { borderTopColor: panelColor } : undefined}
          />
        </div>
      </div>
    );
  }

  if (!device) {
    return (
      <div className="flex min-h-0 flex-1 flex-col items-center justify-center rounded-lg bg-(--color-bg-surface) p-4 shadow">
        <HardDrive className="mb-2 h-6 w-6 text-(--color-text-muted)" />
        <p className="text-xs text-(--color-text-muted)">{t('dashboard.panel.deviceNotFound')}</p>
      </div>
    );
  }

  // 값이 하나도 없어도 여기서 되돌아가지 않는다. 고정 설치 디바이스는 첫 통신 전까지
  // 보고하는 값이 없는데, 그때 화면을 통째로 비우면 고정해 둔 것이 무엇이었는지조차
  // 알 수 없다. 고른 항목의 카드는 그대로 그리고 값만 '-' 로 둔다.
  const properties = device.state?.properties ?? {};

  // 전원 OFF 시 운전 계열 속성은 정규화된 기본값이라 실제 값이 아니므로 '-' 로 표시.
  const powerOff = properties['power'] === false;

  // 표시할 속성 필터링 (visibleProperties가 비어있으면 전체 표시).
  // 필터는 원본 속성 키 기준이므로 'measurements' 를 선택하면 측정치 전체가 표시된다.
  //
  // gateways 는 여기서 제외한다. 이 패널은 사용자가 컬럼 수와 표시 항목을 고르는
  // "key/value 카드 N열" 그리드라, (디바이스, 게이트웨이) 쌍 여러 건짜리 표를 끼워 넣으면
  // 사용자가 지정한 레이아웃이 깨진다. 제외하지 않으면 객체 폴백으로 JSON 덩어리가
  // 표시되므로 제외 자체는 필수다. 링크별 상세는 디바이스 상세 패널의 게이트웨이
  // 섹션과 에이전트 게이트웨이 탭에서 본다.
  // 카드 항목 생성은 buildDisplayEntries 한 곳이 한다 — 설정 화면의 "표시 항목"도
  // 같은 함수를 쓰므로 고를 수 있는 것과 그려지는 것이 갈라지지 않는다.
  //
  // 표시 항목 필터는 **펼친 뒤** 건다. 종전에는 펼치기 전 컨테이너 키(`measurements`)로
  // 걸러, 설정에서 "온도"를 골라도 아무 일이 없고 컨테이너를 골라야 측정치가 전부
  // 나왔다.
  const allEntries = buildDisplayEntries({
    properties,
    id: device.id,
    metadata: device.metadata,
  });
  const showBadges = config.showBadges !== false;
  const badgeItems = readTileItems(config.badgeItems, DEVICE_BADGES, DEVICE_BADGE_DEFAULT);
  // 배지의 "갱신" 은 가장 최근 값을 말한다 — 항목마다 다른 시각을 하나로 줄이는 방법은
  // 최댓값뿐이다(가장 오래된 것을 쓰면 방금 온 값이 있어도 오래된 것으로 보인다).
  const lastUpdatedMs = allEntries.reduce<number | undefined>(
    (max, e) => (e.timeMs !== undefined && (max === undefined || e.timeMs > max) ? e.timeMs : max),
    undefined,
  );

  /** 배지 하나가 낼 글자. */
  const badgeText = (badge: DeviceBadge): string => {
    switch (badge) {
      case 'updated':
        return `${t('dashboard.settings.propertiesGridOpt.badgeUpdated')} ${
          lastUpdatedMs !== undefined
            ? formatRelativeEpochMs(lastUpdatedMs)
            : t('dashboard.panel.awaitingData')
        }`;
      case 'kind':
        return getDeviceTypeLabel(device.type);
      case 'protocol':
        return device.protocol.toUpperCase();
      case 'online':
        return device.online ? t('devices.detail.online') : t('devices.detail.offline');
    }
  };

  // 고른 항목을 고른 **순서대로** 배열한다(selectEntries 주석 참조).
  // 값이 아직 없는 항목도 자리를 잡아 둔다. 다만 파생 항목(`meta.*` · `gw.*`)은 모두
  // 열거할 수 있으므로, 목록에 없다면 지금은 존재하지 않는 키다 — 지난 설정에 남은
  // 이름으로 빈 카드를 만들어 낼 이유가 없다.
  const entries = selectEntries(allEntries, visibleProperties, (key) =>
    isDerivedPropertyKey(key) ? undefined : { id: key, key, value: undefined },
  );

  return (
    <div className="flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-4 shadow">
      {/* 헤더 */}
      {showTitle && (
      <div className="mb-3 flex shrink-0 items-center justify-between">
        <div className="flex items-center gap-2">
          <span
            className="truncate text-sm font-medium text-(--color-text-primary)"
            style={{ ...(acColor('labels') ? { color: acColor('labels')! } : undefined), ...titleStyle }}
          >
            {title}
          </span>
        </div>
        <span className={cn(
          'inline-flex items-center gap-1 rounded-full px-2 py-1',
          device.online
            ? 'bg-blue-50 text-blue-500 dark:bg-blue-900/30 dark:text-blue-400'
            : 'bg-(--color-bg-sunken) text-(--color-text-muted)',
        )}>
          {device.online
            ? <span title={t('dashboard.acPanel.operating')}><Activity className="h-3.5 w-3.5" aria-label={t('dashboard.acPanel.operating')} /></span>
            : <span title={t('dashboard.acPanel.standby')}><Moon className="h-3.5 w-3.5" aria-label={t('dashboard.acPanel.standby')} /></span>}
        </span>
      </div>
      )}

      {/*
        배지 — 타이틀 옆에 글자로 붙어 있던 프로토콜을 여기로 옮겼다. 무엇을 낼지 고를 수
        있고 항목마다 모양을 정한다. 종전에는 종류와 프로토콜이 한 배지에 붙어 있어 하나만
        내는 것이 불가능했다.
      */}
      {showBadges && badgeItems.length > 0 && (
        <div className="mb-3 flex shrink-0 flex-wrap gap-2" data-testid="properties-grid-badges">
          {badgeItems.map((badge) => {
            const design = resolveTileDesign(readTileDesign(config.badgeStyles, badge), undefined);
            return (
              <span
                key={badge}
                data-testid={`properties-grid-badge-${badge}`}
                className={cn(
                  'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] text-(--color-text-secondary)',
                  !design.hasOwnBackground && 'bg-(--color-bg-sunken)',
                )}
                style={{ ...design.box, ...design.valueStyle }}
                title={
                  badge === 'updated' && lastUpdatedMs !== undefined
                    ? formatEpochMs(lastUpdatedMs)
                    : undefined
                }
              >
                {badgeText(badge)}
              </span>
            );
          })}
        </div>
      )}

      {/* 그룹별 속성 격자 */}
      <div className="min-h-0 flex-1 space-y-4 overflow-y-auto">
        {entries.length === 0 && (
          <div className="flex flex-1 items-center justify-center">
            <p className="text-xs text-(--color-text-muted)">{t('dashboard.panel.noProperties')}</p>
          </div>
        )}
        {groupEntries(entries).map(({ group, entries: groupItems }) => {
          const grid = readTileGrid(config[PROPERTY_GROUP_GRID_KEY[group]], PROPERTY_GROUP_GRID[group]);
          const areas = placeTiles(
            groupItems.map((e) => e.key),
            config[PROPERTY_GROUP_AREA_KEY[group]] as Record<string, Partial<TileArea>> | undefined,
            grid,
            PROPERTY_TILE_SIZE,
          );
          return (
            <div key={group} data-testid={`properties-grid-group-${group}`}>
              <p className="mb-1.5 text-xs font-medium text-(--color-text-muted)">
                {t(PROPERTY_GROUP_LABEL_KEYS[group])}
              </p>
              <div
                data-testid={`properties-grid-${group}-grid`}
                className="grid gap-3"
                style={{
                  gridTemplateColumns: `repeat(${grid.cols}, minmax(0, 1fr))`,
                  gridTemplateRows: `repeat(${grid.rows}, minmax(0, auto))`,
                }}
              >
                {groupItems.map(({ id, key, value, timeMs }) => {
                  const override = readPropertyOverride(config, key);
                  return (
                    <PropertyCard
                      key={id}
                      propertyKey={key}
                      label={resolveTileLabel(
                        override,
                        getPropertyLabel(key, device.protocol, device.type),
                      )}
                      value={formatPropertyValue(key, value, {
                        powerOff,
                        unit: override.unit,
                      })}
                      timeMs={timeMs}
                      design={design}
                      layout={resolveCardLayout(design, override)}
                      overrideStyle={applyPropertyOverride(design, override, value)}
                      borderColor={acColor('borders')}
                      // 항목별 배경이 공통 배경을 이긴다 — 좁은 쪽이 이기는 것이
                      // 글자 설정과 같은 규칙이다.
                      background={override.bg ?? (config.tileBg as string | undefined)}
                      area={areas[key]}
                      stale={isStaleValue(timeMs, device.metadata?.stale_after_sec, Date.now())}
                      dragging={dragKey === key}
                      onDragStart={editing && onConfigChange ? () => setDragKey(key) : undefined}
                      onDragOver={
                        editing && onConfigChange && dragKey && dragKey !== key
                          ? () => {
                              onConfigChange({
                                visibleProperties: reorderVisible(
                                  visibleProperties,
                                  entries.map((e) => e.key),
                                  dragKey,
                                  key,
                                ),
                              });
                            }
                          : undefined
                      }
                      onDragEnd={editing ? () => setDragKey(null) : undefined}
                    />
                  );
                })}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

/** 이 패널이 배지로 낼 수 있는 정보. */
const DEVICE_BADGES = ['updated', 'kind', 'protocol', 'online'] as const;
type DeviceBadge = (typeof DEVICE_BADGES)[number];

/**
 * 처음부터 켜 두는 배지.
 *
 * 연결 상태는 헤더 아이콘이 이미 말하므로 기본에서 뺀다 — 같은 것을 두 번 말하면
 * 어느 쪽이 정본인지 흐려진다. 필요하면 켜서 글자로도 볼 수 있다.
 */
const DEVICE_BADGE_DEFAULT: DeviceBadge[] = ['updated', 'kind', 'protocol'];

const DEVICE_BADGE_LABEL_KEYS: Record<DeviceBadge, string> = {
  updated: 'dashboard.settings.propertiesGridOpt.badgeUpdated',
  kind: 'dashboard.settings.propertiesGridOpt.badgeKind',
  protocol: 'dashboard.settings.propertiesGridOpt.badgeProtocol',
  online: 'dashboard.settings.propertiesGridOpt.badgeOnline',
};

export { DEVICE_BADGES, DEVICE_BADGE_DEFAULT, DEVICE_BADGE_LABEL_KEYS };
export type { DeviceBadge };

/** 시작 열 → 가로 정렬. 오른쪽 끝 칸은 오른쪽으로 붙는 게 자연스럽다. */
function areaAlign(area: CardArea, cols: number): string {
  // 한 줄 전체를 쓰면 왼쪽 정렬이 읽기 좋다 — 가운데로 몰면 값과 항목명이 어긋난다.
  if (area.colSpan >= cols) return 'text-left';
  if (area.col === 1) return 'text-left';
  return area.col + area.colSpan - 1 >= cols ? 'text-right' : 'text-center';
}

/**
 * 속성 카드 하나 — 항목명 · 값 · 마지막 갱신 시각.
 *
 * 카드를 행×열(기본 3×3)로 나누고 세 조각을 각자 고른 **영역**(시작 행·열 + 칸 수)에
 * 놓는다. 칸 하나만 쓰던 종전에는 "값을 위쪽 한 줄 전체" 같은 배치를 만들 수 없어
 * 값이 길면 칸 밖으로 넘쳤다. 빈 행은 높이를 차지하지 않아 카드가 커지지 않는다.
 */
function PropertyCard({
  propertyKey,
  label,
  value,
  timeMs,
  design,
  layout,
  overrideStyle,
  borderColor,
  background,
  area,
  stale,
  dragging,
  onDragStart,
  onDragOver,
  onDragEnd,
}: {
  /** 원본 항목 키 — 통신으로 채워지는 항목인지 가리는 데 쓴다. */
  propertyKey: string;
  label: string;
  value: React.ReactNode;
  timeMs: number | undefined;
  design: PropertiesGridStyle;
  /** 이 카드가 쓸 분할과 조각 배치(항목별 설정이 있으면 그것, 없으면 전체 설정). */
  layout: { grid: CardGrid; areas: CardAreas };
  /** 카드 전체 설정에 항목별 설정을 덮은 결과. */
  overrideStyle: Pick<PropertiesGridStyle, 'labelStyle' | 'valueStyle' | 'timeStyle'>;
  borderColor: string | undefined;
  /** 타일 배경색. 정하지 않으면 패널 기본 배경이 그대로 산다. */
  background: string | undefined;
  /** 격자 안 자리. */
  area: TileArea | undefined;
  /** 갱신 시간 제한을 넘긴 값. 지우지 않고 흐리게 두어 마지막 값이 무엇이었는지는 남긴다. */
  stale: boolean;
  dragging: boolean;
  /** 편집 중일 때만 넘어온다 — 없으면 끌 수 없다. */
  onDragStart?: () => void;
  onDragOver?: () => void;
  onDragEnd?: () => void;
}) {
  const { t } = useTranslation();
  // 시각 자리는 값이 없어도 **항상** 잡는다. 없을 때만 빼면 그 카드만 한 줄 짧아져
  // 격자 안에서 카드 높이가 들쭉날쭉해진다.
  const showTime = design.showUpdatedAt;
  const { rows, cols } = layout.grid;

  /** 조각 하나를 제 영역에 놓는다. gridRow/Column 으로 DOM 순서와 무관하게 자리가 정해진다. */
  const cell = (element: 'label' | 'value' | 'time', node: React.ReactNode) => {
    const area = layout.areas[element];
    return (
      <div
        key={element}
        data-testid={`property-${element}`}
        data-area={`${area.row},${area.col},${area.rowSpan},${area.colSpan}`}
        className={cn('min-w-0', areaAlign(area, cols))}
        style={{
          gridRow: `${area.row} / span ${area.rowSpan}`,
          gridColumn: `${area.col} / span ${area.colSpan}`,
        }}
      >
        {node}
      </div>
    );
  };

  return (
    <div
      className={cn(
        'rounded-lg border border-(--color-border-default) px-3 py-2',
        !background && 'bg-(--color-bg-surface)',
        onDragStart && 'cursor-grab select-none',
        stale && 'opacity-50',
        dragging && 'cursor-grabbing opacity-60 ring-2 ring-blue-500',
      )}
      style={{
        ...(background ? { backgroundColor: background } : undefined),
        ...(borderColor ? { borderColor: `${borderColor}30` } : undefined),
        ...(area
          ? { gridColumn: `${area.x} / span ${area.w}`, gridRow: `${area.y} / span ${area.h}` }
          : undefined),
      }}
      data-testid="property-card"
      onMouseDown={onDragStart ? (e) => { if (e.button === 0) onDragStart(); } : undefined}
      onMouseEnter={onDragOver}
      onMouseUp={onDragEnd}
    >
      {/* 분할 격자 — 빈 행은 auto 높이라 0 으로 접힌다. */}
      <div
        className="grid gap-x-2 gap-y-0.5"
        style={{
          gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`,
          gridTemplateRows: `repeat(${rows}, auto)`,
        }}
      >
        {cell(
          'label',
          <p className="truncate text-xs text-(--color-text-muted)" style={overrideStyle.labelStyle}>
            {label}
          </p>,
        )}
        {cell(
          'value',
          <p className="text-sm font-medium text-(--color-text-primary)" style={overrideStyle.valueStyle}>
            {value}
          </p>,
        )}
        {showTime &&
          cell(
            'time',
            // 값과 경쟁하지 않도록 작고 흐리게. 정확한 시각은 title(hover)로 준다.
            <p
              className="text-[10px] text-(--color-text-muted)"
              style={overrideStyle.timeStyle}
              title={timeMs !== undefined ? formatEpochMs(timeMs) : undefined}
            >
              {timeMs !== undefined ? (
                formatRelativeEpochMs(timeMs)
              ) : needsReception(propertyKey) ? (
                // 아직 한 번도 안 온 값. '-' 만 있으면 값이 없는 것인지 시각이 없는
                // 것인지 읽히지 않는다.
                <span data-testid="property-awaiting">{t('dashboard.panel.awaitingData')}</span>
              ) : (
                // 통신과 무관한 항목 — 비워 두되 자리는 남긴다(줄바꿈 없는 공백).
                '\u00A0'
              )}
              {/* 흐리기만 하면 원래 그런 모양인지 오래된 것인지 구분되지 않는다. */}
              {stale && (
                <span
                  data-testid="property-stale"
                  className="ml-1 rounded-sm bg-(--color-bg-sunken) px-1 text-(--color-text-muted)"
                >
                  {t('dashboard.panel.stale')}
                </span>
              )}
            </p>,
          )}
      </div>
    </div>
  );
}
