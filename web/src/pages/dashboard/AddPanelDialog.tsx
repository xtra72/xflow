// 패널 추가 다이얼로그.
// 카테고리 탭 기반으로 패널 유형을 선택하여 활성 대시보드에 추가한다.
// 'device', 'ac-control', 'hvac-control', 'custom-control' 유형 선택 시
// 디바이스 목록을 표시하여 개별 디바이스를 선택한다.

import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  GitBranch,
  Bot,
  Activity,
  HardDrive,
  ScrollText,
  Hash,
  TrendingUp,
  BarChart2,
  PieChart,
  FileText,
  Cpu,
  Thermometer,
  Wind,
  Settings,
  X,
  ArrowLeft,
  Wifi,
  WifiOff,
  Search,
  CircleDot,
  Table,
  LayoutGrid,
  Route,
  MapPin,
  Fan,
} from 'lucide-react';
import type { LucideIcon } from 'lucide-react';

import { useUIStore, type PanelType } from '@/stores/uiStore';
import { useDevices } from '@/hooks/useDevice';
import { useAgents } from '@/hooks/useAgent';
import { useStations, useAirpurifierDevices } from '@/hooks/useStation';
import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';
import { getDeviceDisplayName, getDeviceTypeLabel } from '@/lib/utils/deviceLabels';
import {
  listChartChannels,
  type ChartChannelSummary,
} from '@/services/api/charts';

// ---- 차트 패널 공통 ----

/** SPEC-CHART-001 REQ-M1-02 / REQ-M5-04: channel_name 정규식 */
const CHANNEL_NAME_REGEX = /^[a-zA-Z][a-zA-Z0-9_-]{0,63}$/;

/** 채널 이름 검증 에러 메시지 키 (렌더 시 t() 로 변환) */
const CHANNEL_NAME_ERROR_KEY = 'dashboard.chart.channelNameError';

/** 차트 계열 패널 타입 집합 */
const CHART_PANEL_TYPES: ReadonlySet<PanelType> = new Set<PanelType>([
  'stat',
  'line-chart',
  'bar-chart',
  'pie-chart',
  'table',
]);

function isChartPanelType(type: PanelType): boolean {
  return CHART_PANEL_TYPES.has(type);
}

// ---- 패널 유형 정의 ----

/** 패널 유형 옵션 */
interface PanelOption {
  type: PanelType;
  icon: LucideIcon;
  /** 라벨 i18n 키 (렌더 시 t() 로 변환) */
  labelKey: string;
  /** 설명 i18n 키 (렌더 시 t() 로 변환) */
  descriptionKey: string;
  /** 디바이스 선택 스텝이 필요한 유형 */
  needsDevice?: boolean;
  /** 설비(에이전트 + 라인/역사/기기) 선택 스텝이 필요한 유형 (SPEC-FACILITY-DASHBOARD-001 M5) */
  needsFacility?: boolean;
  /**
   * 차트 채널 선택 스텝을 건너뛰고 곧바로 추가할 prefilled config.
   * 다채널 비교 등 단일 채널 입력만으로 부족한 프리셋용.
   */
  presetConfig?: Record<string, unknown>;
}

/** 카테고리 정의 (안정적인 식별자, 표시 라벨은 t() 로 변환) */
type Category = 'data' | 'chart' | 'content' | 'control';

const CATEGORIES: Category[] = ['data', 'chart', 'content', 'control'];

/** 카테고리 표시 라벨 키 (기존 dashboard.panelCategories 재사용) */
const CATEGORY_LABEL_KEY: Record<Category, string> = {
  data: 'dashboard.panelCategories.data',
  chart: 'dashboard.panelCategories.chart',
  content: 'dashboard.panelCategories.content',
  control: 'dashboard.panelCategories.control',
};

/** 카테고리별 패널 옵션 */
const PANEL_OPTIONS_BY_CATEGORY: Record<Category, PanelOption[]> = {
  data: [
    { type: 'flows', icon: GitBranch, labelKey: 'dashboard.panelTypes.flows', descriptionKey: 'dashboard.addPanel.descriptions.flows' },
    { type: 'agents', icon: Bot, labelKey: 'dashboard.panelTypes.agents', descriptionKey: 'dashboard.addPanel.descriptions.agents' },
    { type: 'resource', icon: Activity, labelKey: 'dashboard.panelTypes.resource', descriptionKey: 'dashboard.addPanel.descriptions.resource' },
    { type: 'devices', icon: HardDrive, labelKey: 'dashboard.panelTypes.devices', descriptionKey: 'dashboard.addPanel.descriptions.devices' },
    { type: 'device', icon: HardDrive, labelKey: 'dashboard.panelTypes.device', descriptionKey: 'dashboard.addPanel.descriptions.device', needsDevice: true },
    { type: 'logs', icon: ScrollText, labelKey: 'dashboard.panelTypes.logs', descriptionKey: 'dashboard.addPanel.descriptions.logs' },
    { type: 'table', icon: Table, labelKey: 'dashboard.panelTypes.table', descriptionKey: 'dashboard.addPanel.descriptions.table' },
    { type: 'properties-grid', icon: LayoutGrid, labelKey: 'dashboard.addPanel.labels.propertiesGrid', descriptionKey: 'dashboard.addPanel.descriptions.propertiesGrid', needsDevice: true },
  ],
  chart: [
    { type: 'stat', icon: Hash, labelKey: 'dashboard.panelTypes.stat', descriptionKey: 'dashboard.addPanel.descriptions.stat' },
    { type: 'gauge', icon: CircleDot, labelKey: 'dashboard.panelTypes.gauge', descriptionKey: 'dashboard.addPanel.descriptions.gauge' },
    { type: 'line-chart', icon: TrendingUp, labelKey: 'dashboard.panelTypes.lineChart', descriptionKey: 'dashboard.addPanel.descriptions.lineChart' },
    {
      type: 'line-chart',
      icon: TrendingUp,
      labelKey: 'dashboard.addPanel.labels.multiChannel',
      descriptionKey: 'dashboard.addPanel.descriptions.multiChannel',
      presetConfig: { channels: [{ name: '' }, { name: '' }] },
    },
    { type: 'bar-chart', icon: BarChart2, labelKey: 'dashboard.panelTypes.barChart', descriptionKey: 'dashboard.addPanel.descriptions.barChart' },
    { type: 'pie-chart', icon: PieChart, labelKey: 'dashboard.panelTypes.pieChart', descriptionKey: 'dashboard.addPanel.descriptions.pieChart' },
  ],
  content: [
    { type: 'text', icon: FileText, labelKey: 'dashboard.panelTypes.text', descriptionKey: 'dashboard.addPanel.descriptions.text' },
  ],
  control: [
    { type: 'ac-control', icon: Thermometer, labelKey: 'dashboard.panelTypes.acControl', descriptionKey: 'dashboard.addPanel.descriptions.acControl', needsDevice: true },
    { type: 'hvac-control', icon: Wind, labelKey: 'dashboard.panelTypes.hvacControl', descriptionKey: 'dashboard.addPanel.descriptions.hvacControl', needsDevice: true },
    { type: 'outdoor-control', icon: Cpu, labelKey: 'dashboard.addPanel.labels.outdoorControl', descriptionKey: 'dashboard.addPanel.descriptions.outdoorControl', needsDevice: true },
    { type: 'custom-control', icon: Settings, labelKey: 'dashboard.panelTypes.customControl', descriptionKey: 'dashboard.addPanel.descriptions.customControl', needsDevice: true },
    // SPEC-FACILITY-DASHBOARD-001 M5: 설비 패널 3종 (에이전트 + 라인/역사/기기 선택).
    { type: 'facility-line', icon: Route, labelKey: 'dashboard.panelTypes.facilityLine', descriptionKey: 'dashboard.addPanel.descriptions.facilityLine', needsFacility: true },
    { type: 'facility-station', icon: MapPin, labelKey: 'dashboard.panelTypes.facilityStation', descriptionKey: 'dashboard.addPanel.descriptions.facilityStation', needsFacility: true },
    { type: 'facility-device', icon: Fan, labelKey: 'dashboard.panelTypes.facilityDevice', descriptionKey: 'dashboard.addPanel.descriptions.facilityDevice', needsFacility: true },
  ],
};

/** 전체 패널 옵션 (검색용) */
const ALL_PANEL_OPTIONS: PanelOption[] = Object.values(PANEL_OPTIONS_BY_CATEGORY).flat();

// ---- 컴포넌트 ----

interface AddPanelDialogProps {
  open: boolean;
  onClose: () => void;
}

/** 패널 추가 모달 */
export default function AddPanelDialog({ open, onClose }: AddPanelDialogProps) {
  const addPanel = useUIStore((s) => s.addPanel);
  const addPanelWithConfig = useUIStore((s) => s.addPanelWithConfig);
  const [step, setStep] = useState<'type' | 'device' | 'chart-config' | 'facility'>('type');
  const [selectedType, setSelectedType] = useState<PanelType | null>(null);

  // 다이얼로그 닫힐 때 상태 초기화
  useEffect(() => {
    if (!open) {
      setStep('type');
      setSelectedType(null);
    }
  }, [open]);

  // ESC 키로 닫기
  useEffect(() => {
    if (!open) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (step === 'device' || step === 'chart-config' || step === 'facility') {
          setStep('type');
          setSelectedType(null);
        } else {
          onClose();
        }
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [open, step, onClose]);

  // 배경 클릭 시 닫기
  const handleBackdropClick = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (e.target === e.currentTarget) onClose();
    },
    [onClose],
  );

  // 패널 유형 선택 처리
  const handleSelect = (option: PanelOption) => {
    if (option.needsDevice) {
      setSelectedType(option.type);
      setStep('device');
      return;
    }
    if (option.needsFacility) {
      setSelectedType(option.type);
      setStep('facility');
      return;
    }
    if (option.presetConfig) {
      // 프리셋이 채널 정보를 미리 주므로 chart-config 스텝 건너뜀
      addPanelWithConfig(option.type, option.presetConfig);
      onClose();
      return;
    }
    if (isChartPanelType(option.type)) {
      setSelectedType(option.type);
      setStep('chart-config');
      return;
    }
    addPanel(option.type);
    onClose();
  };

  // 디바이스 선택 처리
  const handleDeviceSelect = (deviceId: string, deviceName: string) => {
    if (!selectedType) return;
    addPanelWithConfig(selectedType, { deviceId }, deviceName);
    onClose();
  };

  // 설비 대상 선택 완료 처리 (에이전트 + 라인/역사/기기).
  const handleFacilityConfirm = (config: Record<string, unknown>, title?: string) => {
    if (!selectedType) return;
    addPanelWithConfig(selectedType, config, title);
    onClose();
  };

  // 차트 설정 완료 처리
  const handleChartConfirm = (channelName: string) => {
    if (!selectedType) return;
    // panelDefaultSize + createDefaultPanel 이 이미 channel_name: '' 을 주므로
    // addPanelWithConfig 로 channel_name 을 덮어쓴다.
    addPanelWithConfig(selectedType, { channel_name: channelName });
    onClose();
  };

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={handleBackdropClick}
      role="dialog"
      aria-modal="true"
      aria-labelledby="add-panel-dialog-title"
    >
      <div className="mx-4 w-full max-w-lg rounded-lg bg-(--color-bg-surface) shadow-xl">
        {step === 'type' && <TypeStep onSelect={handleSelect} onClose={onClose} />}
        {step === 'device' && (
          <DeviceStep
            onSelect={handleDeviceSelect}
            onBack={() => {
              setStep('type');
              setSelectedType(null);
            }}
            onClose={onClose}
          />
        )}
        {step === 'chart-config' && selectedType && (
          <ChartConfigStep
            panelType={selectedType}
            onConfirm={handleChartConfirm}
            onBack={() => {
              setStep('type');
              setSelectedType(null);
            }}
            onClose={onClose}
          />
        )}
        {step === 'facility' && selectedType && (
          <FacilityStep
            panelType={selectedType}
            onConfirm={handleFacilityConfirm}
            onBack={() => {
              setStep('type');
              setSelectedType(null);
            }}
            onClose={onClose}
          />
        )}
      </div>
    </div>
  );
}

// ---- Step 1: 카테고리 기반 패널 유형 선택 ----

function TypeStep({
  onSelect,
  onClose,
}: {
  onSelect: (option: PanelOption) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [activeCategory, setActiveCategory] = useState<Category>('data');
  const [searchQuery, setSearchQuery] = useState('');

  // 검색 결과 (검색어가 있으면 전체 카테고리에서 필터링)
  const filteredOptions = useMemo(() => {
    const query = searchQuery.trim().toLowerCase();
    if (!query) {
      return PANEL_OPTIONS_BY_CATEGORY[activeCategory];
    }
    // 검색 시 전체에서 필터링 (번역된 라벨/설명 기준)
    return ALL_PANEL_OPTIONS.filter(
      (opt) =>
        t(opt.labelKey).toLowerCase().includes(query) ||
        t(opt.descriptionKey).toLowerCase().includes(query),
    );
  }, [activeCategory, searchQuery, t]);

  const isSearching = searchQuery.trim().length > 0;

  return (
    <>
      {/* 헤더 */}
      <div className="border-b border-(--color-border-default) px-5 pt-4 pb-3">
        <div className="flex items-center justify-between">
          <h2
            id="add-panel-dialog-title"
            className="text-lg font-semibold text-(--color-text-primary)"
          >
            {t('dashboard.addPanel.title')}
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
            aria-label={t('dashboard.addPanel.closeAria')}
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* 검색 입력 */}
        <div className="relative mt-3">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-(--color-text-muted)" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder={t('dashboard.addPanel.searchPlaceholder')}
            className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) py-2 pr-3 pl-9 text-sm text-(--color-text-primary) placeholder:text-(--color-text-muted) focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400"
          />
        </div>

        {/* 카테고리 탭 (검색 중이 아닐 때만 표시) */}
        {!isSearching && (
          <div className="mt-3 flex gap-1">
            {CATEGORIES.map((cat) => (
              <button
                key={cat}
                type="button"
                onClick={() => setActiveCategory(cat)}
                className={cn(
                  'rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
                  activeCategory === cat
                    ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300'
                    : 'text-(--color-text-secondary) hover:bg-(--color-bg-elevated) hover:text-(--color-text-primary)',
                )}
              >
                {t(CATEGORY_LABEL_KEY[cat])}
              </button>
            ))}
          </div>
        )}
      </div>

      {/* 패널 유형 카드 그리드 */}
      <div className="max-h-80 overflow-y-auto px-5 py-4">
        {filteredOptions.length === 0 ? (
          <p className="py-8 text-center text-sm text-(--color-text-muted)">
            {t('dashboard.addPanel.noResults')}
          </p>
        ) : (
          <div className="grid grid-cols-2 gap-3">
            {filteredOptions.map((option) => {
              const Icon = option.icon;
              return (
                <button
                  key={`${option.type}:${option.labelKey}`}
                  type="button"
                  onClick={() => onSelect(option)}
                  className="flex items-start gap-3 rounded-lg border border-(--color-border-default) p-3 text-left transition-colors hover:border-blue-300 hover:bg-blue-50 dark:hover:border-blue-600 dark:hover:bg-blue-900/20"
                >
                  <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-(--color-bg-elevated)">
                    <Icon className="h-4 w-4 text-(--color-text-secondary)" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <p className="text-sm font-semibold text-(--color-text-primary)">
                      {t(option.labelKey)}
                    </p>
                    <p className="mt-0.5 text-xs leading-relaxed text-(--color-text-muted)">
                      {t(option.descriptionKey)}
                    </p>
                  </div>
                </button>
              );
            })}
          </div>
        )}
      </div>
    </>
  );
}

// ---- Step 2: 디바이스 선택 ----

function DeviceStep({
  onSelect,
  onBack,
  onClose,
}: {
  onSelect: (deviceId: string, deviceName: string) => void;
  onBack: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const { data: devicesData, isLoading } = useDevices();
  const devices = devicesData?.data ?? [];

  return (
    <>
      <div className="flex items-center justify-between border-b border-(--color-border-default) px-5 py-4">
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={onBack}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
            aria-label={t('dashboard.addPanel.backAria')}
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <h2 className="text-lg font-semibold text-(--color-text-primary)">
            {t('dashboard.addPanel.selectDevice')}
          </h2>
        </div>
        <button
          type="button"
          onClick={onClose}
          className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
          aria-label={t('dashboard.addPanel.closeAria')}
        >
          <X className="h-5 w-5" />
        </button>
      </div>
      <div className="max-h-80 overflow-y-auto px-5 py-4">
        {isLoading ? (
          <div className="flex items-center justify-center py-8">
            <div className="h-6 w-6 animate-spin rounded-full border-2 border-(--color-border-strong) border-t-blue-600" />
          </div>
        ) : devices.length === 0 ? (
          <p className="py-8 text-center text-sm text-(--color-text-muted)">
            {t('dashboard.addPanel.noDevices')}
          </p>
        ) : (
          <div className="space-y-2">
            {devices.map((device) => (
              <button
                key={device.uid ?? device.id}
                type="button"
                onClick={() => onSelect(device.id, getDeviceDisplayName(device))}
                className="flex w-full items-center gap-3 rounded-lg border border-(--color-border-default) p-3 text-left transition-colors hover:border-blue-300 hover:bg-blue-50 dark:hover:border-blue-600 dark:hover:bg-blue-900/20"
              >
                <span
                  className={cn(
                    'h-2.5 w-2.5 shrink-0 rounded-full',
                    device.online ? 'bg-green-500' : 'bg-gray-400',
                  )}
                />
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium text-(--color-text-primary)">
                    {getDeviceDisplayName(device)}
                  </p>
                  <p className="text-xs text-(--color-text-muted)">
                    {getDeviceTypeLabel(device.type)} &middot; {device.protocol.toUpperCase()}
                  </p>
                </div>
                {device.online ? (
                  <Wifi className="h-4 w-4 shrink-0 text-green-500" />
                ) : (
                  <WifiOff className="h-4 w-4 shrink-0 text-gray-400" />
                )}
              </button>
            ))}
          </div>
        )}
      </div>
    </>
  );
}

// ---- Step 3: 차트 채널 이름 설정 (SPEC-CHART-001 REQ-M5-01/02/04) ----

/** "Custom..." 수동 입력 표시용 sentinel */
const CUSTOM_CHANNEL_SENTINEL = '__custom__';

/** 차트 패널 타입별 표시 라벨 키 (기존 dashboard.panelTypes 재사용) */
const CHART_TYPE_LABEL_KEY: Partial<Record<PanelType, string>> = {
  stat: 'dashboard.panelTypes.stat',
  'line-chart': 'dashboard.panelTypes.lineChart',
  'bar-chart': 'dashboard.panelTypes.barChart',
  'pie-chart': 'dashboard.panelTypes.pieChart',
  table: 'dashboard.panelTypes.table',
};

function ChartConfigStep({
  panelType,
  onConfirm,
  onBack,
  onClose,
}: {
  panelType: PanelType;
  onConfirm: (channelName: string) => void;
  onBack: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  // 드롭다운 선택 상태: 채널 이름 OR '__custom__' OR '' (초기)
  const [selectedOption, setSelectedOption] = useState<string>('');
  // Custom 모드일 때 수동 입력 값
  const [customName, setCustomName] = useState<string>('');
  const [channels, setChannels] = useState<ChartChannelSummary[]>([]);
  const [loadState, setLoadState] = useState<'idle' | 'loading' | 'error'>('loading');
  const [loadError, setLoadError] = useState<string | null>(null);

  // 활성 채널 목록 조회 (마운트 시 1회)
  useEffect(() => {
    let cancelled = false;
    setLoadState('loading');
    setLoadError(null);
    listChartChannels()
      .then((result) => {
        if (cancelled) return;
        setChannels(result);
        setLoadState('idle');
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        const msg = err instanceof Error ? err.message : String(err);
        setLoadError(msg);
        setLoadState('error');
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // 실제로 사용할 채널 이름 계산
  const effectiveName =
    selectedOption === CUSTOM_CHANNEL_SENTINEL ? customName : selectedOption;

  // 검증 (REQ-M5-04)
  const trimmed = effectiveName.trim();
  const isEmpty = trimmed.length === 0;
  const isValidFormat = !isEmpty && CHANNEL_NAME_REGEX.test(trimmed);
  const showError = !isEmpty && !isValidFormat;
  const canSave = isValidFormat;

  const handleConfirm = () => {
    if (!canSave) return;
    onConfirm(trimmed);
  };

  return (
    <>
      {/* 헤더 */}
      <div className="flex items-center justify-between border-b border-(--color-border-default) px-5 py-4">
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={onBack}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
            aria-label={t('dashboard.addPanel.backAria')}
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <h2
            id="add-panel-dialog-title"
            className="text-lg font-semibold text-(--color-text-primary)"
          >
            {(CHART_TYPE_LABEL_KEY[panelType] ? t(CHART_TYPE_LABEL_KEY[panelType]!) : t('dashboard.addPanel.chartFallback'))} {t('dashboard.addPanel.channelSuffix')}
          </h2>
        </div>
        <button
          type="button"
          onClick={onClose}
          className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
          aria-label={t('dashboard.addPanel.closeAria')}
        >
          <X className="h-5 w-5" />
        </button>
      </div>

      {/* 본문 */}
      <div className="space-y-4 px-5 py-4">
        {/* 드롭다운: 활성 채널 목록 + Custom */}
        <div>
          <label
            htmlFor="chart-channel-select"
            className="mb-1.5 block text-xs font-medium text-(--color-text-muted)"
          >
            {t('dashboard.addPanel.channelNameLabel')} <span className="text-red-500">*</span>
          </label>
          <select
            id="chart-channel-select"
            data-testid="chart-channel-select"
            value={selectedOption}
            onChange={(e) => {
              setSelectedOption(e.target.value);
              if (e.target.value !== CUSTOM_CHANNEL_SENTINEL) {
                setCustomName('');
              }
            }}
            disabled={loadState === 'loading'}
            className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 disabled:opacity-60"
          >
            <option value="">
              {loadState === 'loading'
                ? t('dashboard.addPanel.loadingChannels')
                : channels.length === 0
                  ? t('dashboard.addPanel.noActiveChannelsCustom')
                  : t('dashboard.addPanel.selectChannel')}
            </option>
            {channels.map((ch) => (
              <option key={ch.name} value={ch.name}>
                {t('dashboard.addPanel.channelOption')
                  .replace('{name}', ch.name)
                  .replace('{flow}', String(ch.flow_id))
                  .replace('{count}', String(ch.subscriber_count))}
              </option>
            ))}
            <option value={CUSTOM_CHANNEL_SENTINEL}>{t('dashboard.addPanel.customOption')}</option>
          </select>
          {loadState === 'error' && (
            <p className="mt-1 text-xs text-amber-600 dark:text-amber-400">
              {t('dashboard.addPanel.loadErrorCustom')}
              {loadError ? ` (${loadError})` : ''}
            </p>
          )}
        </div>

        {/* Custom 모드: 수동 입력 필드 */}
        {selectedOption === CUSTOM_CHANNEL_SENTINEL && (
          <div>
            <label
              htmlFor="chart-channel-custom"
              className="mb-1.5 block text-xs font-medium text-(--color-text-muted)"
            >
              {t('dashboard.addPanel.customLabel')}
            </label>
            <input
              id="chart-channel-custom"
              data-testid="chart-channel-custom-input"
              type="text"
              value={customName}
              onChange={(e) => setCustomName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && canSave) handleConfirm();
              }}
              placeholder={t('dashboard.addPanel.customPlaceholder')}
              autoFocus
              className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
            />
          </div>
        )}

        {/* 인라인 에러 (REQ-M5-04) */}
        {showError && (
          <p data-testid="chart-channel-error" className="text-xs text-red-500">
            {t(CHANNEL_NAME_ERROR_KEY)}
          </p>
        )}
      </div>

      {/* 푸터 */}
      <div className="flex justify-end gap-2 border-t border-(--color-border-default) px-5 py-3">
        <button
          type="button"
          onClick={onBack}
          className="rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-4 py-1.5 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-border-default)"
        >
          {t('dashboard.addPanel.previous')}
        </button>
        <button
          type="button"
          data-testid="chart-channel-save"
          onClick={handleConfirm}
          disabled={!canSave}
          className={cn(
            'rounded-md px-4 py-1.5 text-sm font-medium transition-colors',
            canSave
              ? 'bg-blue-600 text-white hover:bg-blue-700'
              : 'cursor-not-allowed bg-gray-300 text-gray-500 dark:bg-gray-700 dark:text-gray-500',
          )}
        >
          {t('dashboard.addPanel.save')}
        </button>
      </div>
    </>
  );
}

// ---- Step: 설비 대상 선택 (SPEC-FACILITY-DASHBOARD-001 M5) ----

/** 설비 패널 타입 → config 대상 키. */
const FACILITY_TARGET_KEY: Record<string, 'deviceId' | 'station' | 'line'> = {
  'facility-device': 'deviceId',
  'facility-station': 'station',
  'facility-line': 'line',
};

/** 대상 셀렉트 라벨/placeholder i18n 키(패널 타입별). */
const FACILITY_TARGET_I18N: Record<string, { label: string; placeholder: string }> = {
  'facility-device': { label: 'dashboard.settings.device', placeholder: 'dashboard.settings.selectDevice' },
  'facility-station': { label: 'dashboard.settings.station', placeholder: 'dashboard.settings.selectStation' },
  'facility-line': { label: 'dashboard.settings.line', placeholder: 'dashboard.settings.selectLine' },
};

/** 대상 옵션 한 건(값 + 표시 라벨). */
interface FacilityTargetOption {
  value: string;
  label: string;
}

/**
 * 설비 대상 선택 스텝. 에이전트(airpurifier)를 먼저 고르고, 패널 타입에 따라
 * 라인/역사/기기 대상을 고른다. 대상 조회는 기존 useStations / useAirpurifierDevices 를
 * 재사용한다(UB-001, 재구현 없음). 완료 시 addPanelWithConfig 로 { agentId, <대상키> } 를 전달한다.
 */
function FacilityStep({
  panelType,
  onConfirm,
  onBack,
  onClose,
}: {
  panelType: PanelType;
  onConfirm: (config: Record<string, unknown>, title?: string) => void;
  onBack: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [agentId, setAgentId] = useState('');
  const [target, setTarget] = useState('');

  const { data: agentsResult } = useAgents();
  const airAgents = useMemo(
    () => (agentsResult?.data ?? []).filter((a) => a.type === 'airpurifier'),
    [agentsResult],
  );

  const { data: stations, isLoading: stationsLoading } = useStations(agentId);
  const { data: devices, isLoading: devicesLoading } = useAirpurifierDevices(agentId);

  const targetKey = FACILITY_TARGET_KEY[panelType] ?? 'deviceId';
  const targetI18n = FACILITY_TARGET_I18N[panelType] ?? FACILITY_TARGET_I18N['facility-device']!;

  // 패널 타입별 대상 옵션 계산.
  const targetOptions: FacilityTargetOption[] = useMemo(() => {
    if (panelType === 'facility-device') {
      return (devices ?? []).map((d) => ({ value: d.device_id, label: d.name || d.device_id }));
    }
    if (panelType === 'facility-station') {
      return (stations ?? []).map((s) => ({ value: s.station, label: s.display_name || s.station }));
    }
    // facility-line: 로스터의 distinct line 값.
    const lines = Array.from(new Set((stations ?? []).map((s) => s.line).filter(Boolean)));
    return lines.map((l) => ({ value: l, label: l }));
  }, [panelType, devices, stations]);

  const targetLoading = panelType === 'facility-device' ? devicesLoading : stationsLoading;
  const canSave = agentId.length > 0 && target.length > 0;

  const handleConfirm = () => {
    if (!canSave) return;
    const selected = targetOptions.find((o) => o.value === target);
    onConfirm({ agentId, [targetKey]: target }, selected?.label);
  };

  return (
    <>
      {/* 헤더 */}
      <div className="flex items-center justify-between border-b border-(--color-border-default) px-5 py-4">
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={onBack}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
            aria-label={t('dashboard.addPanel.backAria')}
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <h2 className="text-lg font-semibold text-(--color-text-primary)">
            {t(targetI18n.label)}
          </h2>
        </div>
        <button
          type="button"
          onClick={onClose}
          className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
          aria-label={t('dashboard.addPanel.closeAria')}
        >
          <X className="h-5 w-5" />
        </button>
      </div>

      {/* 본문 */}
      <div className="space-y-4 px-5 py-4">
        {/* 에이전트 선택 */}
        <div>
          <label
            htmlFor="facility-agent-select"
            className="mb-1.5 block text-xs font-medium text-(--color-text-muted)"
          >
            {t('dashboard.settings.agent')} <span className="text-red-500">*</span>
          </label>
          <select
            id="facility-agent-select"
            data-testid="facility-agent-select"
            value={agentId}
            onChange={(e) => {
              setAgentId(e.target.value);
              setTarget('');
            }}
            className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
          >
            <option value="">{t('dashboard.settings.selectAgent')}</option>
            {airAgents.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          </select>
        </div>

        {/* 대상(라인/역사/기기) 선택 */}
        <div>
          <label
            htmlFor="facility-target-select"
            className="mb-1.5 block text-xs font-medium text-(--color-text-muted)"
          >
            {t(targetI18n.label)} <span className="text-red-500">*</span>
          </label>
          <select
            id="facility-target-select"
            data-testid="facility-target-select"
            value={target}
            onChange={(e) => setTarget(e.target.value)}
            disabled={!agentId || targetLoading}
            className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 disabled:opacity-60"
          >
            <option value="">{t(targetI18n.placeholder)}</option>
            {targetOptions.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </div>
      </div>

      {/* 푸터 */}
      <div className="flex justify-end gap-2 border-t border-(--color-border-default) px-5 py-3">
        <button
          type="button"
          onClick={onBack}
          className="rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-4 py-1.5 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-border-default)"
        >
          {t('dashboard.addPanel.previous')}
        </button>
        <button
          type="button"
          data-testid="facility-save"
          onClick={handleConfirm}
          disabled={!canSave}
          className={cn(
            'rounded-md px-4 py-1.5 text-sm font-medium transition-colors',
            canSave
              ? 'bg-blue-600 text-white hover:bg-blue-700'
              : 'cursor-not-allowed bg-gray-300 text-gray-500 dark:bg-gray-700 dark:text-gray-500',
          )}
        >
          {t('dashboard.addPanel.save')}
        </button>
      </div>
    </>
  );
}
