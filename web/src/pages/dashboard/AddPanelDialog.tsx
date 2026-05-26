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
} from 'lucide-react';
import type { LucideIcon } from 'lucide-react';

import { useUIStore, type PanelType } from '@/stores/uiStore';
import { useDevices } from '@/hooks/useDevice';
import { cn } from '@/lib/utils/cn';
import { getDeviceDisplayName, getDeviceTypeLabel } from '@/lib/utils/deviceLabels';
import {
  listChartChannels,
  type ChartChannelSummary,
} from '@/services/api/charts';

// ---- 차트 패널 공통 ----

/** SPEC-CHART-001 REQ-M1-02 / REQ-M5-04: channel_name 정규식 */
const CHANNEL_NAME_REGEX = /^[a-zA-Z][a-zA-Z0-9_-]{0,63}$/;

/** 채널 이름 검증 에러 메시지 (사용자 대화 언어: 한국어) */
const CHANNEL_NAME_ERROR_MESSAGE =
  '유효한 채널 이름이 아닙니다. 영문자로 시작하고 영숫자/하이픈/밑줄만 허용됩니다 (최대 64자).';

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
  label: string;
  description: string;
  /** 디바이스 선택 스텝이 필요한 유형 */
  needsDevice?: boolean;
  /**
   * 차트 채널 선택 스텝을 건너뛰고 곧바로 추가할 prefilled config.
   * 다채널 비교 등 단일 채널 입력만으로 부족한 프리셋용.
   */
  presetConfig?: Record<string, unknown>;
}

/** 카테고리 정의 */
type Category = '데이터' | '차트' | '콘텐츠' | '제어';

const CATEGORIES: Category[] = ['데이터', '차트', '콘텐츠', '제어'];

/** 카테고리별 패널 옵션 */
const PANEL_OPTIONS_BY_CATEGORY: Record<Category, PanelOption[]> = {
  데이터: [
    { type: 'flows', icon: GitBranch, label: '플로우 현황', description: '플로우 목록과 실행 상태' },
    { type: 'agents', icon: Bot, label: '에이전트 현황', description: '에이전트 목록과 상태' },
    { type: 'resource', icon: Activity, label: '프로세스 리소스', description: 'CPU, 메모리 등 시스템 메트릭' },
    { type: 'devices', icon: HardDrive, label: '디바이스 목록', description: '등록된 디바이스 목록과 상태' },
    { type: 'device', icon: HardDrive, label: '디바이스 제어', description: '개별 디바이스 리모컨', needsDevice: true },
    { type: 'logs', icon: ScrollText, label: '로그', description: '실시간 로그 스트림' },
    { type: 'table', icon: Table, label: '테이블', description: '데이터 테이블 뷰' },
    { type: 'properties-grid', icon: LayoutGrid, label: '속성 그리드', description: '디바이스 속성을 항목별 그리드로 표시', needsDevice: true },
  ],
  차트: [
    { type: 'stat', icon: Hash, label: '통계', description: '단일 수치 통계 카드' },
    { type: 'gauge', icon: CircleDot, label: '게이지', description: '원형 게이지 차트' },
    { type: 'line-chart', icon: TrendingUp, label: '라인 차트', description: '시계열 라인 차트' },
    {
      type: 'line-chart',
      icon: TrendingUp,
      label: '다채널 비교',
      description: '여러 chart-emitter 채널을 한 라인 차트에서 동시 비교',
      presetConfig: { channels: [{ name: '' }, { name: '' }] },
    },
    { type: 'bar-chart', icon: BarChart2, label: '바 차트', description: '막대 차트' },
    { type: 'pie-chart', icon: PieChart, label: '파이 차트', description: '원형 비율 차트' },
  ],
  콘텐츠: [
    { type: 'text', icon: FileText, label: '텍스트', description: '마크다운 텍스트 블록' },
  ],
  제어: [
    { type: 'ac-control', icon: Thermometer, label: '에어컨 제어', description: '에어컨 온도/모드 제어', needsDevice: true },
    { type: 'hvac-control', icon: Wind, label: '공조기 제어', description: '공조기 통합 제어', needsDevice: true },
    { type: 'outdoor-control', icon: Cpu, label: '실외기 모니터링', description: '실외기/제어기 압축기 상태', needsDevice: true },
    { type: 'custom-control', icon: Settings, label: '커스텀 제어', description: '사용자 정의 제어', needsDevice: true },
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
  const [step, setStep] = useState<'type' | 'device' | 'chart-config'>('type');
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
        if (step === 'device' || step === 'chart-config') {
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
  const [activeCategory, setActiveCategory] = useState<Category>('데이터');
  const [searchQuery, setSearchQuery] = useState('');

  // 검색 결과 (검색어가 있으면 전체 카테고리에서 필터링)
  const filteredOptions = useMemo(() => {
    const query = searchQuery.trim().toLowerCase();
    if (!query) {
      return PANEL_OPTIONS_BY_CATEGORY[activeCategory];
    }
    // 검색 시 전체에서 필터링
    return ALL_PANEL_OPTIONS.filter(
      (opt) =>
        opt.label.toLowerCase().includes(query) ||
        opt.description.toLowerCase().includes(query),
    );
  }, [activeCategory, searchQuery]);

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
            패널 추가
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
            aria-label="닫기"
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
            placeholder="패널 검색..."
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
                {cat}
              </button>
            ))}
          </div>
        )}
      </div>

      {/* 패널 유형 카드 그리드 */}
      <div className="max-h-80 overflow-y-auto px-5 py-4">
        {filteredOptions.length === 0 ? (
          <p className="py-8 text-center text-sm text-(--color-text-muted)">
            검색 결과가 없습니다.
          </p>
        ) : (
          <div className="grid grid-cols-2 gap-3">
            {filteredOptions.map((option) => {
              const Icon = option.icon;
              return (
                <button
                  key={`${option.type}:${option.label}`}
                  type="button"
                  onClick={() => onSelect(option)}
                  className="flex items-start gap-3 rounded-lg border border-(--color-border-default) p-3 text-left transition-colors hover:border-blue-300 hover:bg-blue-50 dark:hover:border-blue-600 dark:hover:bg-blue-900/20"
                >
                  <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-(--color-bg-elevated)">
                    <Icon className="h-4 w-4 text-(--color-text-secondary)" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <p className="text-sm font-semibold text-(--color-text-primary)">
                      {option.label}
                    </p>
                    <p className="mt-0.5 text-xs leading-relaxed text-(--color-text-muted)">
                      {option.description}
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
            aria-label="뒤로"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <h2 className="text-lg font-semibold text-(--color-text-primary)">
            디바이스 선택
          </h2>
        </div>
        <button
          type="button"
          onClick={onClose}
          className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
          aria-label="닫기"
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
            등록된 디바이스가 없습니다.
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

/** 차트 패널 타입별 표시 라벨 */
const CHART_TYPE_LABEL: Record<PanelType, string> = {
  stat: '통계',
  'line-chart': '라인 차트',
  'bar-chart': '바 차트',
  'pie-chart': '파이 차트',
  table: '테이블',
  // 아래는 차트 외 타입이지만 Record 완전성을 위해 포함 (사용되지 않음)
  flows: '',
  agents: '',
  resource: '',
  devices: '',
  device: '',
  logs: '',
  gauge: '',
  text: '',
  'ac-control': '',
  'hvac-control': '',
  'custom-control': '',
  'outdoor-control': '',
  'properties-grid': '',
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
            aria-label="뒤로"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <h2
            id="add-panel-dialog-title"
            className="text-lg font-semibold text-(--color-text-primary)"
          >
            {CHART_TYPE_LABEL[panelType] || '차트'} 채널 선택
          </h2>
        </div>
        <button
          type="button"
          onClick={onClose}
          className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
          aria-label="닫기"
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
            채널 이름 <span className="text-red-500">*</span>
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
                ? '활성 채널 목록 불러오는 중...'
                : channels.length === 0
                  ? '활성 채널이 없습니다 (Custom 으로 수동 입력)'
                  : '채널을 선택하세요'}
            </option>
            {channels.map((ch) => (
              <option key={ch.name} value={ch.name}>
                {ch.name} — flow {ch.flow_id} ({ch.subscriber_count} subs)
              </option>
            ))}
            <option value={CUSTOM_CHANNEL_SENTINEL}>Custom... (직접 입력)</option>
          </select>
          {loadState === 'error' && (
            <p className="mt-1 text-xs text-amber-600 dark:text-amber-400">
              채널 목록 조회에 실패했습니다. 수동 입력을 사용하세요.
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
              수동 입력 채널 이름
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
              placeholder="예: room1_temp"
              autoFocus
              className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
            />
          </div>
        )}

        {/* 인라인 에러 (REQ-M5-04) */}
        {showError && (
          <p data-testid="chart-channel-error" className="text-xs text-red-500">
            {CHANNEL_NAME_ERROR_MESSAGE}
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
          이전
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
          저장
        </button>
      </div>
    </>
  );
}
