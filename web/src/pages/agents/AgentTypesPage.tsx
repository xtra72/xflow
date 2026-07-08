// 에이전트 타입 브라우저 페이지.
// 등록된 에이전트 타입을 카드 그리드로 표시하며 검색을 지원한다.
// 카드 클릭 시 상세 패널(설정 필드, 예제)이 확장된다.

import { Fragment, useMemo, useState } from 'react';
import { ChevronDown, ChevronRight, Search } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';
import { AGENT_TYPES } from '@/config/agentSchemas';

import { AGENT_TYPE_META } from './agentTypeMeta';
import AgentTypeDetailPanel from './AgentTypeDetailPanel';

/** 그리드 열 수 (lg 기준) */
const GRID_COLS = 3;

/** 에이전트 타입별 카테고리 */
const AGENT_CATEGORY: Record<string, string> = {
  'mqtt-client': 'protocol',
  'thingplus-gateway': 'protocol',
  'modbus-tcp': 'protocol',
  'modbus-tcp-server': 'protocol',
  http: 'protocol',
  'http-sender': 'protocol',
  serial: 'protocol',
  'tcp-server': 'protocol',
  'tcp-client': 'protocol',
  'samsung_hvacr01': 'device',
  lgap: 'device',
  lg_hvacr02: 'device',
  lg_hvacr01: 'device',
  'century_hvacr01': 'device',
  influxdb: 'storage',
  store: 'storage',
  logger: 'utility',
};

/** 카테고리 라벨 i18n 키 */
const CATEGORY_LABEL_KEYS: Record<string, string> = {
  전체: 'agents.types.catAll',
  protocol: 'agents.types.catProtocol',
  device: 'agents.types.catDevice',
  storage: 'agents.types.catStorage',
  utility: 'agents.types.catUtility',
};

/** 카테고리별 배지 스타일 */
const CATEGORY_BADGE_STYLES: Record<string, string> = {
  protocol: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
  device: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400',
  storage: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
  utility: 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300',
};

/**
 * 에이전트 타입 브라우저 페이지.
 * 검색, 카테고리 탭, 카드 그리드로 구성된다.
 */
export default function AgentTypesPage() {
  const { t } = useTranslation();
  const [search, setSearch] = useState('');
  const [selectedCategory, setSelectedCategory] = useState('전체');
  const [expandedType, setExpandedType] = useState<string | null>(null);

  const categories = ['전체', 'protocol', 'device', 'storage', 'utility'];

  // 검색어 및 카테고리로 필터링
  const filteredAgents = useMemo(() => {
    const query = search.toLowerCase().trim();
    return AGENT_TYPES.filter((agent) => {
      // 카테고리 필터
      if (selectedCategory !== '전체' && AGENT_CATEGORY[agent.value] !== selectedCategory) {
        return false;
      }
      // 검색어 필터
      if (query) {
        const meta = AGENT_TYPE_META[agent.value];
        return (
          agent.value.toLowerCase().includes(query) ||
          agent.label.toLowerCase().includes(query) ||
          (meta?.description ?? '').toLowerCase().includes(query)
        );
      }
      return true;
    });
  }, [search, selectedCategory]);

  /** 카드 클릭 시 상세 패널 토글 */
  const toggleExpand = (type: string) => {
    setExpandedType((prev) => (prev === type ? null : type));
  };

  return (
    <div className="space-y-6">
      {/* 페이지 헤더 */}
      <div>
        <h2 className="text-2xl font-bold text-(--color-text-primary)">
          {t('agents.types.title')}
        </h2>
        <p className="mt-1 text-sm text-(--color-text-muted)">
          {t('agents.types.subtitle')}
        </p>
      </div>

      {/* 검색 입력 */}
      <div className="relative max-w-md">
        <Search
          className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400 dark:text-gray-500"
          aria-hidden="true"
        />
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={t('agents.types.searchPlaceholder')}
          className={cn(
            'block w-full rounded-md border border-(--color-border-strong) py-2 pl-10 pr-3 text-sm',
            'placeholder:text-gray-400',
            'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500',
            'bg-(--color-bg-surface) text-(--color-text-primary)',
          )}
        />
      </div>

      {/* 카테고리 탭 */}
      <div className="flex gap-1 border-b border-(--color-border-default)">
        {categories.map((cat) => (
          <button
            key={cat}
            type="button"
            onClick={() => setSelectedCategory(cat)}
            className={cn(
              'px-4 py-2 text-sm font-medium transition-colors',
              selectedCategory === cat
                ? 'border-b-2 border-blue-500 text-blue-600 dark:text-blue-400'
                : 'text-(--color-text-muted) hover:text-(--color-text-secondary)',
            )}
          >
            {t(CATEGORY_LABEL_KEYS[cat] ?? 'agents.types.catUtility')}
          </button>
        ))}
      </div>

      {/* 카드 그리드 */}
      {filteredAgents.length > 0 ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {filteredAgents.map((agent, index) => {
            const isExpanded = expandedType === agent.value;
            const category = AGENT_CATEGORY[agent.value] ?? 'utility';
            const meta = AGENT_TYPE_META[agent.value];
            const isRowEnd =
              (index + 1) % GRID_COLS === 0 || index === filteredAgents.length - 1;
            const showPanel =
              isExpanded ||
              (expandedType &&
                isRowEnd &&
                isExpandedInRow(filteredAgents, expandedType, index));

            return (
              <Fragment key={agent.value}>
                {/* 에이전트 카드 */}
                <div
                  onClick={() => toggleExpand(agent.value)}
                  className={cn(
                    'rounded-lg border border-(--color-border-default)',
                    'bg-(--color-bg-surface) p-4',
                    'hover:shadow-md transition-shadow cursor-pointer',
                    isExpanded && 'ring-2 ring-blue-500 dark:ring-blue-400',
                  )}
                >
                  <div className="flex items-center justify-between gap-2">
                    <div className="flex items-center gap-2 min-w-0">
                      <span className="shrink-0 text-(--color-text-muted)">
                        {isExpanded ? (
                          <ChevronDown className="h-4 w-4" />
                        ) : (
                          <ChevronRight className="h-4 w-4" />
                        )}
                      </span>
                      <h3 className="text-sm font-bold text-(--color-text-primary) truncate">
                        {agent.label}
                      </h3>
                    </div>
                    <span
                      className={cn(
                        'inline-flex shrink-0 rounded-full px-2 py-0.5 text-xs font-medium',
                        CATEGORY_BADGE_STYLES[category] ?? CATEGORY_BADGE_STYLES.utility,
                      )}
                    >
                      {t(CATEGORY_LABEL_KEYS[category] ?? 'agents.types.catUtility')}
                    </span>
                  </div>
                  <p className="text-xs font-mono text-(--color-text-muted) mt-1">
                    {agent.value}
                  </p>
                  {meta && (
                    <p className="text-sm text-(--color-text-muted) mt-2 line-clamp-2">
                      {meta.description}
                    </p>
                  )}
                </div>

                {/* 상세 패널 (행의 마지막 카드 뒤) */}
                {showPanel && expandedType && (
                  <div className="col-span-1 md:col-span-2 lg:col-span-3">
                    <AgentTypeDetailPanel agentType={expandedType} />
                  </div>
                )}
              </Fragment>
            );
          })}
        </div>
      ) : (
        <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-12 text-center">
          <p className="text-sm text-(--color-text-muted)">
            {t('agents.types.noResults')}
          </p>
        </div>
      )}
    </div>
  );
}

/**
 * 확장된 에이전트가 현재 인덱스와 같은 행에 있는지 확인.
 */
function isExpandedInRow(
  agents: readonly { value: string }[],
  expandedType: string,
  currentIndex: number,
): boolean {
  const rowStart = currentIndex - (currentIndex % GRID_COLS);
  const rowEnd = Math.min(rowStart + GRID_COLS - 1, agents.length - 1);
  for (let i = rowStart; i <= rowEnd; i++) {
    if (agents[i]!.value === expandedType) return true;
  }
  return false;
}
