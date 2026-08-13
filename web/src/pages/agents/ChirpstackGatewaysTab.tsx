// ChirpStack 게이트웨이 탭 (SPEC-CHIRPSTACK-003 M4, AC-8 / AC-8b).
//
// 하나의 업링크는 여러 게이트웨이가 동시에 수신하므로 (게이트웨이, 디바이스) 는 쌍이며
// RSSI / SNR / 채널은 그 쌍에 귀속된다. 따라서 같은 디바이스가 여러 게이트웨이 아래에
// 서로 다른 링크 품질로 반복 등장하는 것이 정상이다 — 게이트웨이 간 중복 제거 금지.
//
// 표기 주의(REQ-M4-04): `channel` 은 수신 게이트웨이의 concentrator IF 채널 인덱스로
// **게이트웨이 로컬 하드웨어 값**이다. 주파수가 아니며 게이트웨이 간 비교할 수 없다.
// 실제 RF 주파수는 프레임 레벨 `frequency_hz` 이고 별도 컬럼으로 분리해 표시한다.
//
// 확장 정책: 다중 확장(Record<string, boolean>). 커버리지를 비교하려는 사용자는 두 개
// 이상의 게이트웨이를 동시에 펼쳐 같은 디바이스의 링크 품질을 나란히 봐야 하므로,
// 단일 확장(DeviceListPage 관용구)이 아니라 XsfmStationsTab 의 다중 확장을 따른다.

import { useMemo, useState } from 'react';
import { ChevronDown, ChevronRight, RadioTower } from 'lucide-react';

import SortableHeader, { type SortState } from '@/components/common/SortableHeader';
import {
  GATEWAYS_POLL_INTERVAL_MS,
  useGateways,
  type ChirpstackGateway,
} from '@/hooks/useChirpstackGateway';
import { useTranslation } from '@/lib/i18n';
import { isRemoteTarget } from '@/lib/remote/target';
import { useTargetContext } from '@/lib/remote/TargetContext';
import { cn } from '@/lib/utils/cn';
// RF 값 포맷터는 디바이스 상세의 게이트웨이 섹션과 공유한다(표기 불일치 방지).
import {
  formatBandwidthHz,
  formatEpochMs,
  formatFrequencyHz,
  formatRelativeEpochMs,
  formatSnr,
} from '@/lib/utils/format';

import TablePagination from './TablePagination';

/** 게이트웨이 행 헤더 컬럼 수(확장 아이콘 + ID + 디바이스 수 + 마지막 수신). */
const GATEWAY_COLSPAN = 4;

export default function ChirpstackGatewaysTab({ agentId }: { agentId: string }) {
  const { t } = useTranslation();
  // 원격 타깃: 게이트웨이 로스터는 exec 기반이라 원격 READ 프록시 매핑이 없다.
  // 훅은 무조건 호출하되(hooks 규칙) 빈 agentId 로 fetch 를 스킵하고 안내만 표시한다.
  const detailTarget = useTargetContext();
  const detailRemote = isRemoteTarget(detailTarget);

  const { data: gateways = [], isLoading } = useGateways(
    detailRemote ? '' : agentId,
    GATEWAYS_POLL_INTERVAL_MS,
  );

  // 다중 확장(게이트웨이 간 링크 품질 비교를 위해 동시 확장 허용).
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const [sort, setSort] = useState<SortState>({ field: 'gateway_id', direction: 'asc' });
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  const sortedGateways = useMemo(() => {
    const dir = sort.direction === 'asc' ? 1 : -1;
    return [...gateways].sort((a, b) => {
      switch (sort.field) {
        case 'device_count':
          return (a.device_count - b.device_count) * dir;
        case 'last_seen_ms':
          return (a.last_seen_ms - b.last_seen_ms) * dir;
        default:
          return a.gateway_id.localeCompare(b.gateway_id) * dir;
      }
    });
  }, [gateways, sort]);

  const totalPages = Math.max(1, Math.ceil(sortedGateways.length / pageSize));
  const safePage = Math.min(page, totalPages);
  const startIndex = (safePage - 1) * pageSize;
  const pagedGateways = sortedGateways.slice(startIndex, startIndex + pageSize);

  function toggleExpand(gatewayId: string) {
    setExpanded((prev) => ({ ...prev, [gatewayId]: !prev[gatewayId] }));
  }

  function handleSort(field: string) {
    setSort((s) =>
      s.field === field
        ? { field, direction: s.direction === 'asc' ? 'desc' : 'asc' }
        : { field, direction: 'asc' },
    );
    setPage(1);
  }

  if (detailRemote) {
    return (
      <div className="flex flex-col items-center gap-2 py-12 text-(--color-text-muted)">
        <RadioTower className="h-8 w-8 opacity-40" aria-hidden="true" />
        <p className="text-sm">{t('agents.detail.gateways.remoteUnavailable')}</p>
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className="py-12 text-center text-sm text-(--color-text-muted)">
        {t('agents.detail.gateways.loading')}
      </div>
    );
  }

  // 빈 상태: 로스터가 업링크 파생이라 불완전하다는 사실을 반드시 알린다(AC-8b).
  if (gateways.length === 0) {
    return (
      <div className="flex flex-col items-center gap-3 px-6 py-12 text-center">
        <RadioTower className="h-8 w-8 text-(--color-text-muted) opacity-40" aria-hidden="true" />
        <p className="text-sm text-(--color-text-muted)">
          {t('agents.detail.gateways.noGateways')}
        </p>
        <p className="max-w-xl text-xs leading-relaxed text-(--color-text-muted)">
          {t('agents.detail.gateways.rosterLimitation')}
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-3 p-4">
      {/* 요약 + 로스터 한계 힌트(채워져 있을 때도 불완전할 수 있음을 상시 고지) */}
      <div className="space-y-1.5">
        <div className="flex items-center gap-2">
          <RadioTower className="h-4 w-4 text-(--color-text-muted)" aria-hidden="true" />
          <span className="text-sm font-medium text-(--color-text-primary)">
            {t('agents.detail.gateways.count').replace('{count}', String(gateways.length))}
          </span>
        </div>
      </div>

      <TablePagination
        page={safePage}
        pageSize={pageSize}
        totalItems={sortedGateways.length}
        onPageChange={setPage}
        onPageSizeChange={(size) => {
          setPageSize(size);
          setPage(1);
        }}
      />

      <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-(--color-border-default) bg-(--color-bg-elevated)">
              <th className="w-8 px-3 py-2" aria-hidden="true" />
              <SortableHeader
                label={t('agents.detail.gateways.gatewayId')}
                field="gateway_id"
                currentSort={sort}
                onSort={handleSort}
                className="px-3 py-2"
              />
              <SortableHeader
                label={t('agents.detail.gateways.deviceCount')}
                field="device_count"
                currentSort={sort}
                onSort={handleSort}
                className="px-3 py-2"
              />
              <SortableHeader
                label={t('agents.detail.gateways.lastSeen')}
                field="last_seen_ms"
                currentSort={sort}
                onSort={handleSort}
                className="px-3 py-2"
              />
            </tr>
          </thead>
          <tbody>
            {pagedGateways.map((g) => (
              <GatewayRow
                key={g.gateway_id}
                gateway={g}
                isOpen={expanded[g.gateway_id] ?? false}
                onToggle={() => toggleExpand(g.gateway_id)}
                t={t}
              />
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

// ---- 게이트웨이 행 + 확장된 디바이스 서브리스트 ----

function GatewayRow({
  gateway,
  isOpen,
  onToggle,
  t,
}: {
  gateway: ChirpstackGateway;
  isOpen: boolean;
  onToggle: () => void;
  t: (key: string) => string;
}) {
  return (
    <>
      <tr
        onClick={onToggle}
        className="cursor-pointer border-b border-(--color-border-default) transition-colors last:border-0 hover:bg-(--color-bg-elevated)"
      >
        <td className="w-8 px-3 py-2">
          {/* 키보드 접근용 토글 버튼. 행 클릭과 중복 토글되지 않도록 전파를 막는다. */}
          <button
            type="button"
            aria-expanded={isOpen}
            aria-label={
              isOpen
                ? t('agents.detail.gateways.collapse')
                : t('agents.detail.gateways.expand')
            }
            onClick={(e) => {
              e.stopPropagation();
              onToggle();
            }}
            className="flex items-center text-(--color-text-muted)"
          >
            {isOpen ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
          </button>
        </td>
        <td className="px-3 py-2 font-mono text-xs break-all text-(--color-text-primary)">
          {gateway.gateway_id}
        </td>
        <td className="px-3 py-2 tabular-nums text-(--color-text-secondary)">
          {gateway.device_count}
        </td>
        <td
          className="px-3 py-2 text-xs text-(--color-text-muted)"
          title={formatEpochMs(gateway.last_seen_ms)}
        >
          {formatRelativeEpochMs(gateway.last_seen_ms)}
        </td>
      </tr>

      {isOpen && (
        <tr className="border-b border-(--color-border-default) last:border-0">
          <td colSpan={GATEWAY_COLSPAN} className="bg-(--color-bg-sunken) px-3 py-2">
            {gateway.devices.length === 0 ? (
              <p className="py-2 text-center text-xs text-(--color-text-muted)">
                {t('agents.detail.gateways.noDevices')}
              </p>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-xs">
                  <thead>
                    <tr className="border-b border-(--color-border-default) text-left text-[10px] font-medium uppercase tracking-wider text-(--color-text-muted)">
                      <th className="px-2 py-1.5">{t('agents.detail.gateways.devEui')}</th>
                      <th className="px-2 py-1.5">{t('agents.detail.gateways.deviceName')}</th>
                      <th className="px-2 py-1.5">{t('agents.detail.gateways.deviceProfile')}</th>
                      <th className="px-2 py-1.5">{t('agents.detail.gateways.rssi')}</th>
                      <th className="px-2 py-1.5">{t('agents.detail.gateways.snr')}</th>
                      {/* channel 은 게이트웨이 로컬 IF 인덱스다 — 주파수 컬럼과 반드시 분리한다. */}
                      <th className="px-2 py-1.5" title={t('agents.detail.gateways.channelHelp')}>
                        {t('agents.detail.gateways.channel')}
                      </th>
                      <th className="px-2 py-1.5" title={t('agents.detail.gateways.frequencyHelp')}>
                        {t('agents.detail.gateways.frequency')}
                      </th>
                      <th className="px-2 py-1.5">{t('agents.detail.gateways.modulation')}</th>
                      <th className="px-2 py-1.5">{t('agents.detail.gateways.lastSeen')}</th>
                      <th className="px-2 py-1.5">{t('agents.detail.gateways.linkState')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {gateway.devices.map((d) => (
                      <tr
                        key={d.dev_eui}
                        className={cn(
                          'border-b border-(--color-border-default) last:border-0',
                          // stale 링크는 흐리게 처리해 live 링크와 시각적으로 구분한다(REQ-M4-05).
                          d.stale && 'opacity-60',
                        )}
                      >
                        <td className="px-2 py-1.5 font-mono break-all text-(--color-text-primary)">
                          {d.dev_eui}
                        </td>
                        <td className="px-2 py-1.5 text-(--color-text-secondary)">
                          {d.device_name || '-'}
                        </td>
                        <td className="px-2 py-1.5 text-(--color-text-secondary)">
                          {d.device_profile_name || '-'}
                        </td>
                        <td className="px-2 py-1.5 tabular-nums text-(--color-text-secondary)">
                          {d.rssi}
                        </td>
                        <td className="px-2 py-1.5 tabular-nums text-(--color-text-secondary)">
                          {formatSnr(d.snr)}
                        </td>
                        <td
                          className="px-2 py-1.5 tabular-nums text-(--color-text-secondary)"
                          title={t('agents.detail.gateways.channelHelp')}
                        >
                          {d.channel}
                        </td>
                        <td className="px-2 py-1.5 tabular-nums text-(--color-text-secondary)">
                          {formatFrequencyHz(d.frequency_hz)}
                        </td>
                        <td className="px-2 py-1.5 text-(--color-text-muted)">
                          {`SF${d.spreading_factor} / ${formatBandwidthHz(d.bandwidth)}`}
                        </td>
                        <td
                          className="px-2 py-1.5 text-(--color-text-muted)"
                          title={formatEpochMs(d.last_seen_ms)}
                        >
                          {formatRelativeEpochMs(d.last_seen_ms)}
                        </td>
                        <td className="px-2 py-1.5">
                          <span
                            className={cn(
                              'inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-medium',
                              d.stale
                                ? 'bg-gray-100 text-gray-500 dark:bg-gray-700 dark:text-gray-400'
                                : 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
                            )}
                          >
                            {d.stale
                              ? t('agents.detail.gateways.stale')
                              : t('agents.detail.gateways.live')}
                          </span>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </td>
        </tr>
      )}
    </>
  );
}
