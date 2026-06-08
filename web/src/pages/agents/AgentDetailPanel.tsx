// 에이전트 상세 패널.
// 행 확장 시 표시되며, 통계 탭과 설정 탭으로 구성된다.
//
// @spec SPEC-WEB-005
// - TSDB 타입 에이전트: 독립 '시리즈' 탭을 통해 데이터 뷰어 모달을 노출한다.
// - Store 타입 에이전트: '저장소' 탭 내부에서 데이터 뷰어 모달 트리거 및 페이지네이션을
//   제공한다 (v0.4.0 통합 UI).

import React, { useCallback, useEffect, useMemo, useState, type ChangeEvent } from 'react';
import { Activity, AlertTriangle, ArrowUpCircle, ChevronDown, ChevronRight, HardDrive, LineChart, Lock, Pencil, Plus, RefreshCw, Save, Server, Trash2, X } from 'lucide-react';

import { useQueryClient } from '@tanstack/react-query';

import { useAgent, useConfigureAgent, useExecAgent } from '@/hooks/useAgent';
import { useAgentDetailTarget, useAgentStatsTarget } from '@/hooks/useDetailTargets';
import { useDevicesRealtime } from '@/hooks/useDevice';
import { useUpdateRemoteAgent } from '@/hooks/useRemote';
import { useTargetGating } from '@/hooks/useTargetGating';
import { useTranslation } from '@/lib/i18n';
import { remoteEditErrorMessage } from '@/lib/remote/editError';
import { omitMaskedSecrets } from '@/lib/remote/secretOmission';
import { useTargetContext } from '@/lib/remote/TargetContext';
import { isRemoteTarget } from '@/lib/remote/target';
import {
  useSeriesDataSource,
  type SeriesDataSourceKind,
} from '@/services/api/seriesDataSource';
import * as agentService from '@/services/api/agentService';
import {
  resetAllStoreKeys,
  resetStoreKey,
  useStoreKeysWithTags,
  useStoreTagPairs,
  type DataType,
  type StoreKeyObject,
  type StoreTagPair,
} from '@/services/api/store';
import { mapStoreError } from '@/lib/errors/storeErrorMapper';
import { cn } from '@/lib/utils/cn';
import { getDeviceTypeLabel } from '@/lib/utils/deviceLabels';
import {
  getAgentConfigSchema,
  HVACR_QUADRANT_AGENT_TYPES,
  STORE_DATA_FIELDS,
  STORE_OPERATION_FIELDS,
} from '@/config/agentSchemas';
import type { ConfigSchema, ConfigSection } from '@/types/node';
import { DynamicForm } from '@/components/property/DynamicForm';
import { FormField } from '@/components/property/FormField';
import {
  StoreKeysEditor,
  type StoreKeyEntry,
} from '@/components/property/StoreKeysEditor';
import { ConfirmDialog } from '@/components/property/ConfirmDialog';
import {
  PromoteToStaticDialog,
  type PromoteToStaticPayload,
} from '@/components/property/PromoteToStaticDialog';
import {
  TagFilterChips,
  matchesTagFilter,
} from '@/components/property/TagFilterChips';
import DeviceDetailPanel from '@/pages/devices/DeviceDetailPanel';
import DeviceStatusBadge from '@/pages/devices/DeviceStatusBadge';
import {
  getLogLevels,
  setComponentLogLevel,
  resetComponentLogLevel,
} from '@/services/api/monitorService';
import { useUIStore } from '@/stores/uiStore';

import TsdbDataViewerModal from './TsdbDataViewerModal';
import TsdbSeriesListPanel from './TsdbSeriesListPanel';

interface AgentDetailPanelProps {
  agentId: string;
  agentType: string;
  /**
   * 에이전트 이름. Store 데이터 소스의 URL 라우트에 사용된다 (필수).
   * TSDB/기타 타입은 현재 name 을 사용하지 않으므로 optional 이지만
   * SPEC-WEB-005 v0.2.0 이후 전 콜사이트에서 전달한다.
   */
  agentName?: string;
}

type Tab = 'stats' | 'config' | 'devices' | 'topics' | 'store' | 'sessions' | 'series';

/** 통계 카드 항목 */
function StatCard({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) p-3">
      <p className="text-xs font-medium text-(--color-text-muted)">{label}</p>
      <p className="mt-1 text-lg font-semibold text-(--color-text-primary)">{value}</p>
    </div>
  );
}

/** 디바이스 탭을 표시하지 않는 에이전트 타입 */
const NO_DEVICES_TAB = new Set(['mqtt-client', 'logger', 'http', 'http-sender', 'influxdb', 'tsdb', 'store', 'serial', 'tcp-server']);

/** 세션 탭을 표시하는 에이전트 타입 */
const HAS_SESSIONS_TAB = new Set(['tcp-server']);

/** 토픽 탭을 표시하는 에이전트 타입 */
const HAS_TOPICS_TAB = new Set(['mqtt-client']);

/** 저장소 탭을 표시하는 에이전트 타입 */
const HAS_STORE_TAB = new Set(['store']);

/**
 * 시리즈 탭을 표시하는 에이전트 타입 (SPEC-WEB-005).
 * TSDB 전용 엔드포인트를 노출하는 'tsdb' 에이전트만 독립 '시리즈' 탭을 사용한다.
 * Store 에이전트는 v0.4.0 에서 '저장소' 탭 내부에 데이터 뷰어 트리거가 통합되어
 * 더 이상 별도 '시리즈' 탭을 노출하지 않는다.
 */
const HAS_SERIES_TAB = new Set<string>(['tsdb']);

export default function AgentDetailPanel({ agentId, agentType, agentName }: AgentDetailPanelProps) {
  const showDevices = !NO_DEVICES_TAB.has(agentType);
  const showTopics = HAS_TOPICS_TAB.has(agentType);
  const showStore = HAS_STORE_TAB.has(agentType);
  const showSessions = HAS_SESSIONS_TAB.has(agentType);
  const showSeries = HAS_SERIES_TAB.has(agentType);

  // TSDB 에이전트는 기본 탭을 '시리즈', Store 에이전트는 '저장소',
  // 그 외에는 '통계' 를 기본 탭으로 선택한다.
  const [tab, setTab] = useState<Tab>(
    showSeries ? 'series' : showStore ? 'store' : 'stats',
  );

  return (
    <div>
      {/* 탭 헤더 */}
      <div className="flex border-b border-(--color-border-default) px-4">
        <TabButton label="통계" active={tab === 'stats'} onClick={() => setTab('stats')} />
        <TabButton label="설정" active={tab === 'config'} onClick={() => setTab('config')} />
        {showTopics && <TabButton label="토픽" active={tab === 'topics'} onClick={() => setTab('topics')} />}
        {showStore && <TabButton label="저장소" active={tab === 'store'} onClick={() => setTab('store')} />}
        {showSeries && <TabButton label="시리즈" active={tab === 'series'} onClick={() => setTab('series')} />}
        {showSessions && <TabButton label="세션" active={tab === 'sessions'} onClick={() => setTab('sessions')} />}
        {showDevices && <TabButton label="디바이스" active={tab === 'devices'} onClick={() => setTab('devices')} />}
      </div>

      {/* 탭 컨텐츠 */}
      {tab === 'stats' && <StatsTab agentId={agentId} />}
      {tab === 'config' && <ConfigTab agentId={agentId} agentType={agentType} />}
      {tab === 'topics' && showTopics && <TopicsTab agentId={agentId} />}
      {tab === 'store' && showStore && <StoreTab agentId={agentId} agentName={agentName} />}
      {tab === 'series' && showSeries && (
        <SeriesTab agentId={agentId} agentType={agentType} agentName={agentName} />
      )}
      {tab === 'sessions' && showSessions && <SessionsTab agentId={agentId} />}
      {tab === 'devices' && showDevices && <DevicesTab agentId={agentId} agentType={agentType} />}
    </div>
  );
}

// ---- 시리즈 탭 (TSDB/Store, SPEC-WEB-005 v0.2.0) ----

/**
 * TSDB/Store 에이전트의 시리즈 목록과 데이터 뷰어 모달을 관리한다.
 * 모달 열림 상태와 전체 시리즈 키 풀은 이 컴포넌트에서 hoist 한다.
 *
 * agentType 에 따라 `SeriesDataSource` 구현을 선택한다:
 *   - `tsdb` → 서버 측 페이지네이션 + 서버 측 집계 (기존 TSDB 엔드포인트)
 *   - `store` → 전체 키 로드 후 클라이언트 슬라이스 + 클라이언트 버킷 집계
 */
function SeriesTab({
  agentId,
  agentType,
  agentName,
}: {
  agentId: string;
  agentType: string;
  agentName?: string;
}) {
  const [modalOpen, setModalOpen] = useState(false);

  // 데이터 소스를 에이전트 타입에 맞춰 생성.
  // Store 인 경우 agentName 이 필요하며, 미전달 시 useSeriesDataSource 가 에러를 던진다.
  // SPEC-REMOTE-001 M8 (그룹 J): 시리즈 데이터 뷰어는 local TSDB/store 서비스
  // 기반이라 원격 READ 프록시 매핑이 없다. 원격 타깃은 안내만 표시한다.
  const seriesTarget = useTargetContext();
  const seriesRemote = isRemoteTarget(seriesTarget);
  const kind: SeriesDataSourceKind = agentType === 'store' ? 'store' : 'tsdb';
  const dataSource = useSeriesDataSource({ kind, agentName, agentId });

  // 모달의 멀티셀렉트 옵션으로 쓰일 전체 키 풀 (최대 1페이지 = 100개).
  // TODO(SPEC-WEB-005): 100개 초과 대응이 필요해지면 전용 전체 조회 hook 을 도입한다.
  const allKeysQuery = dataSource.useKeys({ page: 1, size: 100 });
  const allSeriesKeys = allKeysQuery.data?.keys ?? [];

  const handleOpen = useCallback(() => {
    setModalOpen(true);
  }, []);

  const handleClose = useCallback(() => {
    setModalOpen(false);
  }, []);

  if (seriesRemote) {
    return (
      <div className="flex flex-col items-center gap-2 py-12 text-(--color-text-muted)">
        <LineChart className="h-8 w-8 opacity-40" aria-hidden="true" />
        <p className="text-sm">원격 노드에서는 시리즈 뷰어를 제공하지 않습니다.</p>
      </div>
    );
  }

  return (
    <>
      {/* 탭 상단 액션 바 — 데이터 뷰어를 여는 단일 트리거 (SPEC-WEB-005 v0.3.0). */}
      <div className="flex items-center justify-between border-b border-(--color-border-default) px-4 py-2">
        <h3 className="text-sm font-medium text-(--color-text-primary)">시리즈</h3>
        <button
          type="button"
          onClick={handleOpen}
          className="inline-flex items-center gap-1 rounded-md border border-(--color-border-strong) px-2.5 py-1 text-xs font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)"
        >
          <LineChart className="h-3.5 w-3.5" aria-hidden="true" />
          데이터 보기
        </button>
      </div>
      <TsdbSeriesListPanel dataSource={dataSource} />
      <TsdbDataViewerModal
        isOpen={modalOpen}
        onClose={handleClose}
        allSeriesKeys={allSeriesKeys}
        dataSource={dataSource}
      />
    </>
  );
}

// ---- 탭 버튼 ----

function TabButton({ label, active, onClick }: { label: string; active: boolean; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'px-4 py-2 text-sm font-medium transition-colors',
        active
          ? 'border-b-2 border-blue-500 text-blue-600 dark:text-blue-400'
          : 'text-(--color-text-muted) hover:text-(--color-text-secondary)',
      )}
    >
      {label}
    </button>
  );
}

// ---- 통계 탭 ----

/** 바이트를 읽기 쉬운 단위로 변환 (통계 탭용) */
function formatStatsBytes(n: number | undefined | null): string {
  if (n == null || n === 0) return '0 B';
  if (n >= 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`;
  if (n >= 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${n} B`;
}

function StatsTab({ agentId }: { agentId: string }) {
  // SPEC-REMOTE-001 M8 (그룹 J): 라이브 통계는 타깃에 따라 로컬 폴링 또는 원격
  // SSE 스트림(+폴백 폴링)으로 취득한다.
  const target = useTargetContext();
  const { data: stats, isLoading } = useAgentStatsTarget(target, agentId);

  if (isLoading) {
    return (
      <div className="grid grid-cols-2 gap-4 p-4 md:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <div
            key={i}
            className="h-20 animate-pulse rounded-lg bg-(--color-bg-elevated)"
          />
        ))}
      </div>
    );
  }

  if (!stats) {
    return (
      <div className="p-4 text-sm text-(--color-text-muted)">
        통계 데이터를 불러올 수 없습니다.
      </div>
    );
  }

  return (
    <div className="space-y-4 p-4">
      {/* 요약 통계 */}
      <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
        <StatCard label="총 수신" value={stats.messages_in.toLocaleString()} />
        <StatCard label="총 송신" value={stats.messages_out.toLocaleString()} />
        <StatCard label="에러" value={stats.error_count.toLocaleString()} />
        <StatCard label="업타임" value={stats.uptime ?? '-'} />
      </div>

      {/* 외부/내부 메시지 분리 */}
      {stats.messages && (
        <div>
          <p className="mb-2 text-xs font-medium text-(--color-text-muted)">메시지 상세</p>
          <div className="grid grid-cols-2 gap-4">
            <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) p-3">
              <p className="mb-2 text-xs font-semibold text-(--color-text-secondary)">외부 (External)</p>
              <div className="grid grid-cols-3 gap-2 text-xs">
                <div>
                  <p className="text-(--color-text-muted)">수신</p>
                  <p className="font-semibold text-(--color-text-primary)">{(stats.messages.external.received ?? 0).toLocaleString()}</p>
                </div>
                <div>
                  <p className="text-(--color-text-muted)">송신</p>
                  <p className="font-semibold text-(--color-text-primary)">{(stats.messages.external.sent ?? 0).toLocaleString()}</p>
                </div>
                <div>
                  <p className="text-(--color-text-muted)">에러</p>
                  <p className="font-semibold text-(--color-text-primary)">{(stats.messages.external.errored ?? 0).toLocaleString()}</p>
                </div>
              </div>
            </div>
            <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) p-3">
              <p className="mb-2 text-xs font-semibold text-(--color-text-secondary)">내부 (Internal)</p>
              <div className="grid grid-cols-3 gap-2 text-xs">
                <div>
                  <p className="text-(--color-text-muted)">수신</p>
                  <p className="font-semibold text-(--color-text-primary)">{(stats.messages.internal.received ?? 0).toLocaleString()}</p>
                </div>
                <div>
                  <p className="text-(--color-text-muted)">송신</p>
                  <p className="font-semibold text-(--color-text-primary)">{(stats.messages.internal.sent ?? 0).toLocaleString()}</p>
                </div>
                <div>
                  <p className="text-(--color-text-muted)">에러</p>
                  <p className="font-semibold text-(--color-text-primary)">{(stats.messages.internal.errored ?? 0).toLocaleString()}</p>
                </div>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* 운영 통계 */}
      <div>
        <p className="mb-2 text-xs font-medium text-(--color-text-muted)">운영 통계</p>
        <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
          <StatCard label="드롭 메시지" value={(stats.dropped_messages ?? 0).toLocaleString()} />
          <StatCard label="로드 시간" value={stats.load_time || '-'} />
          <StatCard label="재시작 횟수" value={(stats.restart_count ?? 0).toLocaleString()} />
          <StatCard label="평균 처리 지연" value={stats.avg_processing_latency || '-'} />
        </div>
      </div>

      {/* 바이트 통계 */}
      <div>
        <p className="mb-2 text-xs font-medium text-(--color-text-muted)">전송량</p>
        <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
          <StatCard label="읽기" value={formatStatsBytes(stats.bytes?.read)} />
          <StatCard label="쓰기" value={formatStatsBytes(stats.bytes?.written)} />
        </div>
      </div>

      {/* 연결 테이블 */}
      {(stats.connections?.length ?? 0) > 0 && (
        <div>
          <p className="mb-2 text-xs font-medium text-(--color-text-muted)">연결 ({stats.connections!.length})</p>
          <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-(--color-border-default) bg-(--color-bg-elevated)">
                  <th className="px-3 py-2 text-left font-medium text-(--color-text-muted)">ID</th>
                  <th className="px-3 py-2 text-right font-medium text-(--color-text-muted)">수신</th>
                  <th className="px-3 py-2 text-right font-medium text-(--color-text-muted)">송신</th>
                  <th className="px-3 py-2 text-right font-medium text-(--color-text-muted)">에러</th>
                  <th className="px-3 py-2 text-right font-medium text-(--color-text-muted)">읽기(B)</th>
                  <th className="px-3 py-2 text-right font-medium text-(--color-text-muted)">쓰기(B)</th>
                  <th className="px-3 py-2 text-right font-medium text-(--color-text-muted)">연결시각</th>
                  <th className="px-3 py-2 text-right font-medium text-(--color-text-muted)">최근활동</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-(--color-border-default)">
                {stats.connections!.map((c) => (
                  <tr key={c.id} className="hover:bg-(--color-bg-elevated)">
                    <td className="px-3 py-2 font-mono text-(--color-text-primary)">{c.id}</td>
                    <td className="px-3 py-2 text-right text-(--color-text-secondary)">{(c.messages_received ?? 0).toLocaleString()}</td>
                    <td className="px-3 py-2 text-right text-(--color-text-secondary)">{(c.messages_sent ?? 0).toLocaleString()}</td>
                    <td className="px-3 py-2 text-right text-(--color-text-secondary)">{(c.messages_errored ?? 0).toLocaleString()}</td>
                    <td className="px-3 py-2 text-right text-(--color-text-muted)">{formatStatsBytes(c.bytes_read)}</td>
                    <td className="px-3 py-2 text-right text-(--color-text-muted)">{formatStatsBytes(c.bytes_written)}</td>
                    <td className="px-3 py-2 text-right text-(--color-text-muted)">{c.connected_at ? new Date(c.connected_at).toLocaleString('ko-KR') : '-'}</td>
                    <td className="px-3 py-2 text-right text-(--color-text-muted)">{c.last_activity_at ? new Date(c.last_activity_at).toLocaleString('ko-KR') : '-'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* 노드 참조 테이블 */}
      {(stats.node_refs?.length ?? 0) > 0 && (
        <div>
          <p className="mb-2 text-xs font-medium text-(--color-text-muted)">노드 참조 ({stats.node_refs!.length})</p>
          <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-(--color-border-default) bg-(--color-bg-elevated)">
                  <th className="px-3 py-2 text-left font-medium text-(--color-text-muted)">노드 이름</th>
                  <th className="px-3 py-2 text-left font-medium text-(--color-text-muted)">플로우 이름</th>
                  <th className="px-3 py-2 text-right font-medium text-(--color-text-muted)">수신</th>
                  <th className="px-3 py-2 text-right font-medium text-(--color-text-muted)">송신</th>
                  <th className="px-3 py-2 text-right font-medium text-(--color-text-muted)">에러</th>
                  <th className="px-3 py-2 text-right font-medium text-(--color-text-muted)">최근활동</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-(--color-border-default)">
                {stats.node_refs!.map((nr) => (
                  <tr key={`${nr.flow_id}-${nr.node_id}`} className="hover:bg-(--color-bg-elevated)">
                    <td className="px-3 py-2 text-(--color-text-primary)">{nr.node_name || nr.node_id}</td>
                    <td className="px-3 py-2 text-(--color-text-secondary)">{nr.flow_name || nr.flow_id}</td>
                    <td className="px-3 py-2 text-right text-(--color-text-secondary)">{(nr.messages_received ?? 0).toLocaleString()}</td>
                    <td className="px-3 py-2 text-right text-(--color-text-secondary)">{(nr.messages_sent ?? 0).toLocaleString()}</td>
                    <td className="px-3 py-2 text-right text-(--color-text-secondary)">{(nr.messages_errored ?? 0).toLocaleString()}</td>
                    <td className="px-3 py-2 text-right text-(--color-text-muted)">{nr.last_activity_at ? new Date(nr.last_activity_at).toLocaleString('ko-KR') : '-'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}

// ---- 2열 설정 레이아웃 (HVACR 외 에이전트) ----
//
// 3 HVACR 에이전트 (samsung_hvacr01 / lg_hvacr01 / century_hvacr01) 는
// FourQuadrantConfigLayout 으로 분기되므로 여기서 제거되었다
// (refactor/hvacr-ui-rendering-fix).

/** 에이전트 타입별 좌측 컬럼 필드 및 컬럼 라벨 */
const TWO_COL_CONFIG: Record<string, { left: Set<string>; leftLabel: string; rightLabel: string }> = {
  'mqtt-client': {
    left: new Set(['broker', 'client_id', 'username', 'password', 'keep_alive_sec', 'connect_timeout_sec']),
    leftLabel: '연결',
    rightLabel: '운영',
  },
  'modbus-tcp': {
    left: new Set(['mode', 'read_mode', 'reconnect_interval', 'request_timeout', 'max_retries']),
    leftLabel: '연결',
    rightLabel: '운영',
  },
  'modbus-tcp-server': {
    left: new Set(['listen_address', 'listen_port']),
    leftLabel: '연결',
    rightLabel: '운영',
  },
  http: {
    left: new Set(['listen_addr', 'path', 'method']),
    leftLabel: '수신',
    rightLabel: '운영',
  },
  'http-sender': {
    left: new Set(['url', 'method', 'content_type']),
    leftLabel: '전송',
    rightLabel: '운영',
  },
  influxdb: {
    left: new Set(['url', 'token', 'org', 'bucket', 'version']),
    leftLabel: '연결',
    rightLabel: '운영',
  },
  logger: {
    left: new Set(['output', 'output_path', 'format', 'max_size', 'max_age', 'max_backups', 'compress']),
    leftLabel: '출력',
    rightLabel: '운영',
  },
  lgap: {
    left: new Set(['transport_type', 'serial_port', 'baud_rate', 'data_bits', 'stop_bits', 'parity', 'connect_timeout', 'read_timeout']),
    leftLabel: '연결',
    rightLabel: '운영',
  },
  lg_hvacr02: {
    left: new Set(['transport_type', 'serial_port', 'baud_rate', 'data_bits', 'stop_bits', 'parity', 'read_timeout', 'tcp_host', 'tcp_port']),
    leftLabel: '연결',
    rightLabel: '운영',
  },
  serial: {
    left: new Set(['port', 'baud_rate', 'data_bits', 'stop_bits', 'parity', 'read_timeout', 'buffer_size']),
    leftLabel: '연결',
    rightLabel: '운영',
  },
  'tcp-server': {
    left: new Set(['host', 'port', 'buffer_size']),
    leftLabel: '연결',
    rightLabel: '운영',
  },
};

function TwoColumnConfigLayout({
  data, schema, onChange, readOnly, agentType, logLevel,
}: {
  nodeId: string;
  data: Record<string, unknown>;
  schema: ConfigSchema;
  onChange: (data: Record<string, unknown>) => void;
  readOnly?: boolean;
  agentType: string;
  logLevel?: { agentId: string; value: string; updating: boolean; onChangeLevel: (v: string) => void };
}) {
  const colConfig = TWO_COL_CONFIG[agentType];
  if (!colConfig) return null;

  const leftFields = schema.fields.filter((f) => colConfig.left.has(f.name));
  const rightFields = schema.fields.filter((f) => !colConfig.left.has(f.name));

  const filterVisible = (fields: typeof schema.fields) =>
    fields.filter((f) => {
      if (!f.visibleWhen) return true;
      const actual = data[f.visibleWhen.field];
      const expected = f.visibleWhen.value;
      if (Array.isArray(expected)) return (expected as unknown[]).includes(actual);
      return actual === expected;
    });

  const handleChange = (fieldName: string, value: unknown) => {
    onChange({ ...data, [fieldName]: value });
  };

  return (
    <div className="grid grid-cols-2 gap-4">
      {/* 좌측 */}
      <div className="space-y-3">
        <h4 className="text-xs font-semibold text-(--color-text-muted) uppercase tracking-wide">{colConfig.leftLabel}</h4>
        {filterVisible(leftFields).map((field) => (
          <FormField
            key={field.name}
            field={field}
            value={data[field.name]}
            onChange={(v) => handleChange(field.name, v)}
            readOnly={readOnly}
          />
        ))}
      </div>
      {/* 우측 */}
      <div className="space-y-3">
        <h4 className="text-xs font-semibold text-(--color-text-muted) uppercase tracking-wide">{colConfig.rightLabel}</h4>
        {filterVisible(rightFields).map((field) => (
          <FormField
            key={field.name}
            field={field}
            value={data[field.name]}
            onChange={(v) => handleChange(field.name, v)}
            readOnly={readOnly}
          />
        ))}
        {logLevel && (
          <div className="space-y-1">
            <label
              htmlFor={`agent-log-${logLevel.agentId}`}
              className="block text-xs font-medium text-(--color-text-secondary)"
            >
              로그 레벨
            </label>
            <select
              id={`agent-log-${logLevel.agentId}`}
              value={logLevel.value}
              onChange={(e) => logLevel.onChangeLevel(e.target.value)}
              disabled={logLevel.updating}
              className={cn(
                'block w-full rounded-md border border-(--color-border-strong) px-3 py-2 text-sm shadow-sm',
                'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500',
                'bg-(--color-bg-surface) text-(--color-text-primary)',
                'disabled:cursor-not-allowed disabled:opacity-50',
              )}
            >
              <option value="">기본값</option>
              <option value="debug">DEBUG</option>
              <option value="info">INFO</option>
              <option value="warn">WARN</option>
              <option value="error">ERROR</option>
            </select>
          </div>
        )}
      </div>
    </div>
  );
}

// ---- 4-분면 설정 레이아웃 (HVACR-01 공용: Samsung / LG / Century) ----

/** HVACR 4-quadrant 분면 헤더 라벨 (한국어 고정) */
const QUADRANT_LABELS: Record<ConfigSection, string> = {
  transport: '연결',
  protocol: '프로토콜',
  operation: '운영',
  logging: '로그',
};

/** 4-분면 렌더 순서 (좌상 → 우상 → 좌하 → 우하 grid flow) */
const QUADRANT_ORDER: ConfigSection[] = ['transport', 'operation', 'protocol', 'logging'];

/**
 * HVACR 에이전트 설정 폼을 4-분면 그리드로 렌더링한다.
 *
 *   ┌─────────────────┬─────────────────┐
 *   │ Transport (연결) │ Operation (운영) │
 *   ├─────────────────┼─────────────────┤
 *   │ Protocol (프로토콜) │ Logging (로그)   │
 *   └─────────────────┴─────────────────┘
 *
 * - section 미지정 필드는 transport 분면으로 폴백한다 (방어적 기본값).
 * - 로그 레벨 셀렉트는 logging 분면 끝에 렌더링한다.
 * - 좁은 화면에서는 grid-cols-1 로 자동 스택된다.
 * - visibleWhen 조건은 분면 분류와 독립적으로 평가된다.
 */
function FourQuadrantConfigLayout({
  data, schema, onChange, readOnly, logLevel,
}: {
  nodeId: string;
  data: Record<string, unknown>;
  schema: ConfigSchema;
  onChange: (data: Record<string, unknown>) => void;
  readOnly?: boolean;
  agentType: string;
  logLevel?: { agentId: string; value: string; updating: boolean; onChangeLevel: (v: string) => void };
}) {
  // section 별로 필드를 분류한다. 스키마 정의 순서는 그대로 유지된다.
  const fieldsBySection: Record<ConfigSection, typeof schema.fields> = {
    transport: [],
    protocol: [],
    operation: [],
    logging: [],
  };
  for (const field of schema.fields) {
    const section: ConfigSection = field.section ?? 'transport';
    fieldsBySection[section].push(field);
  }

  // visibleWhen 평가는 TwoColumnConfigLayout 과 동일 규칙을 사용한다.
  const filterVisible = (fields: typeof schema.fields) =>
    fields.filter((f) => {
      if (!f.visibleWhen) return true;
      const actual = data[f.visibleWhen.field];
      const expected = f.visibleWhen.value;
      if (Array.isArray(expected)) return (expected as unknown[]).includes(actual);
      return actual === expected;
    });

  const handleChange = (fieldName: string, value: unknown) => {
    onChange({ ...data, [fieldName]: value });
  };

  return (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
      {QUADRANT_ORDER.map((section) => {
        const visible = filterVisible(fieldsBySection[section]);
        const showLogLevel = section === 'logging' && logLevel;
        // 분면 자체는 항상 렌더링하되, 빈 분면은 placeholder 텍스트를 출력한다.
        return (
          <div key={section} className="space-y-3">
            <h4 className="text-xs font-semibold text-(--color-text-muted) uppercase tracking-wide">
              {QUADRANT_LABELS[section]}
            </h4>
            {visible.length === 0 && !showLogLevel && (
              <p className="text-xs text-(--color-text-muted) italic">설정 항목 없음</p>
            )}
            {showLogLevel && (
              <div className="space-y-1">
                <label
                  htmlFor={`agent-log-${logLevel.agentId}`}
                  className="block text-xs font-medium text-(--color-text-secondary)"
                >
                  로그 레벨
                </label>
                <select
                  id={`agent-log-${logLevel.agentId}`}
                  value={logLevel.value}
                  onChange={(e) => logLevel.onChangeLevel(e.target.value)}
                  disabled={logLevel.updating}
                  className={cn(
                    'block w-full rounded-md border border-(--color-border-strong) px-3 py-2 text-sm shadow-sm',
                    'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500',
                    'bg-(--color-bg-surface) text-(--color-text-primary)',
                    'disabled:cursor-not-allowed disabled:opacity-50',
                  )}
                >
                  <option value="">기본값</option>
                  <option value="debug">DEBUG</option>
                  <option value="info">INFO</option>
                  <option value="warn">WARN</option>
                  <option value="error">ERROR</option>
                </select>
              </div>
            )}
            {visible.map((field) => (
              <FormField
                key={field.name}
                field={field}
                value={data[field.name]}
                onChange={(v) => handleChange(field.name, v)}
                readOnly={readOnly}
              />
            ))}
          </div>
        );
      })}
    </div>
  );
}

// ---- Store 에이전트 전용 설정 에디터 (SPEC-STORE-003) ----

/**
 * Store 에이전트 설정을 "운영" / "데이터" 두 섹션으로 나누어 렌더링한다.
 *
 * - 운영 섹션: max_key_length, scan_interval, default_ttl, max_history_size, history_ttl
 * - 데이터 섹션:
 *     * registration_type (enum 셀렉트 — v0.7.0 M12, 이전 allow_dynamic_keys 토글 대체)
 *     * keys (정적 키 + data_type + metric_type + 태그) — StoreKeysEditor 를 통해 편집한다.
 *
 * `keys` 는 ConfigSchema 에 포함되지 않는 커스텀 UI 필드로, 이 컴포넌트에서
 * 직접 data.keys 를 읽고 onChange 로 병합한다.
 *
 * v0.7.0 (M12, M13):
 *   - 데이터 섹션의 `registration_type` 값을 StoreKeysEditor 에 전달하여
 *     manual 모드에서 data_type 필수 검증을 활성화한다.
 *   - 검증 실패 시 부모에 알리도록 `onValidityChange` 콜백 노출.
 *
 * @spec SPEC-WEB-005 v0.7.0 (M12, M13)
 * @spec SPEC-STORE-003 v0.3.0
 */
function StoreConfigEditor({
  data,
  schema,
  onChange,
  onValidityChange,
  readOnly,
}: {
  data: Record<string, unknown>;
  schema: ConfigSchema;
  onChange: (data: Record<string, unknown>) => void;
  /**
   * StoreKeysEditor 의 검증 결과를 부모에 전파.
   * 부모는 `valid=false` 수신 시 저장 버튼을 비활성화해야 한다.
   *
   * @spec SPEC-WEB-005 v0.7.0 (M13)
   */
  onValidityChange?: (valid: boolean) => void;
  readOnly?: boolean;
}) {
  // 섹션별 필드 분할. `keys` 는 스키마에 없으므로 여기서 명시적으로 처리한다.
  const operationFields = schema.fields.filter((f) =>
    STORE_OPERATION_FIELDS.has(f.name),
  );
  const dataFields = schema.fields.filter((f) => STORE_DATA_FIELDS.has(f.name));

  // v0.7.0 (M12): registration_type 값을 읽어 StoreKeysEditor 에 전달.
  // 백엔드 default 는 'auto'. 빈 값/알 수 없는 값은 'auto' 로 폴백.
  const registrationType =
    data.registration_type === 'manual' ? 'manual' : 'auto';

  const handleFieldChange = (name: string, value: unknown) => {
    onChange({ ...data, [name]: value });
  };

  const handleKeysChange = (next: StoreKeyEntry[]) => {
    onChange({ ...data, keys: next });
  };

  return (
    <div className="space-y-6">
      {/* 운영 섹션 */}
      <section>
        <h4 className="mb-3 text-xs font-semibold uppercase tracking-wide text-(--color-text-muted)">
          운영
        </h4>
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          {operationFields.map((field) => (
            <FormField
              key={field.name}
              field={field}
              value={data[field.name]}
              onChange={(v) => handleFieldChange(field.name, v)}
              readOnly={readOnly}
            />
          ))}
        </div>
      </section>

      {/* 데이터 섹션 */}
      <section>
        <h4 className="mb-3 text-xs font-semibold uppercase tracking-wide text-(--color-text-muted)">
          데이터
        </h4>
        <div className="space-y-4">
          {dataFields.map((field) => (
            <FormField
              key={field.name}
              field={field}
              value={data[field.name]}
              onChange={(v) => handleFieldChange(field.name, v)}
              readOnly={readOnly}
            />
          ))}
          <div>
            <p className="mb-1.5 text-xs font-medium text-(--color-text-secondary)">
              정적 키 목록
            </p>
            <p className="mb-2 text-[11px] text-(--color-text-muted)">
              미리 등록된 키와 태그. 태그는 필터링과 그룹화에 사용됩니다.
              {registrationType === 'manual' && (
                <span className="ml-1 text-amber-600 dark:text-amber-400">
                  manual 모드에서는 모든 행에 data_type 입력이 필요합니다.
                </span>
              )}
            </p>
            <StoreKeysEditor
              value={data.keys}
              onChange={handleKeysChange}
              registrationType={registrationType}
              onValidityChange={onValidityChange}
              readOnly={readOnly}
            />
          </div>
        </div>
      </section>
    </div>
  );
}

// ---- 설정 탭 ----

function ConfigTab({ agentId, agentType }: { agentId: string; agentType: string }) {
  // SPEC-REMOTE-001 review: 설정 읽기는 타깃에 따라 전환한다. 편집/저장은
  // 로컬·원격 모두 본 인라인 폼에서 수행한다(로컬과 동형 UX). 로컬은
  // useConfigureAgent, 원격은 useUpdateRemoteAgent(그룹 I, REQ-I04/I07) 로 분기한다.
  // 로그레벨 변경(local monitorService)은 원격 대응 백엔드가 없어 원격에서는 숨긴다.
  const { t } = useTranslation();
  const target = useTargetContext();
  const remote = isRemoteTarget(target);
  const gating = useTargetGating(target);
  const { data: agent, isLoading } = useAgentDetailTarget(target, agentId);
  const configureAgent = useConfigureAgent();
  const updateRemoteAgent = useUpdateRemoteAgent();
  const addNotification = useUIStore((s) => s.addNotification);
  // 저장 진행 상태(로컬/원격 통합) 및 편집 게이팅(원격은 노드 승인+온라인 필요).
  const isSaving = remote ? updateRemoteAgent.isPending : configureAgent.isPending;
  const editGated = remote && !gating.nodeReady;
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<Record<string, unknown>>({});
  // v0.7.0 (M13): Store 에이전트의 keys 행 검증 상태.
  // StoreConfigEditor → StoreKeysEditor 에서 data_type/metric_type 검증 결과를 받아
  // manual 모드 미입력 시 저장 버튼을 비활성화한다. 다른 에이전트 타입에서는 항상 true.
  const [storeKeysValid, setStoreKeysValid] = useState(true);

  // 컴포넌트별 로그 레벨 상태
  const [componentLogLevel, setComponentLogLevel_] = useState<string>('');
  const [isLogLevelUpdating, setIsLogLevelUpdating] = useState(false);

  // 컴포넌트 이름: observe 패키지에서 "agent.<type>.<name>" 형식으로 등록한다.
  const agentName = agent?.name ?? '';
  const componentKey = agentName ? `agent.${agentType}.${agentName}` : '';

  // 마운트 시 현재 에이전트의 로그 레벨 로드
  useEffect(() => {
    if (!componentKey) return;
    let cancelled = false;
    getLogLevels()
      .then((info) => {
        if (cancelled) return;
        const level = info.components[componentKey];
        setComponentLogLevel_(level ?? '');
      })
      .catch(() => {
        // 로드 실패 시 무시 (기본값 유지)
      });
    return () => {
      cancelled = true;
    };
  }, [componentKey]);

  /** 로그 레벨 변경 핸들러 */
  async function handleLogLevelChange(value: string) {
    const componentName = componentKey;
    setIsLogLevelUpdating(true);
    try {
      if (value === '') {
        await resetComponentLogLevel(componentName);
        setComponentLogLevel_('');
        addNotification({ type: 'success', message: '로그 레벨이 기본값으로 리셋되었습니다' });
      } else {
        await setComponentLogLevel(componentName, value);
        setComponentLogLevel_(value);
        addNotification({ type: 'success', message: `로그 레벨이 "${value.toUpperCase()}"로 변경되었습니다` });
      }
    } catch {
      addNotification({ type: 'error', message: '로그 레벨 변경에 실패했습니다' });
    } finally {
      setIsLogLevelUpdating(false);
    }
  }

  const config = agent?.config ?? {};
  const schema = getAgentConfigSchema(agentType);

  // 에이전트 데이터 로드 시 드래프트 초기화
  useEffect(() => {
    if (agent?.config) {
      setDraft(agent.config);
    }
  }, [agent?.config]);

  const handleEdit = useCallback(() => {
    setDraft(config);
    setEditing(true);
  }, [config]);

  const handleCancel = useCallback(() => {
    setDraft(config);
    setEditing(false);
    // v0.7.0 (M13): 편집 종료 시 검증 상태 리셋 (다음 편집 시작점에서 컴포넌트가 재계산).
    setStoreKeysValid(true);
  }, [config]);

  const handleSave = useCallback(async () => {
    // 원격 타깃: 그룹 I(M7) 와 동일한 update 명령 경로로 분기한다(REQ-I04/I07).
    // 마스킹/미입력 시크릿 필드는 omitMaskedSecrets 로 생략(노드 backfill).
    // 종류(type)는 수정 대상이 아니며, 이름은 행 인라인 편집 경로에서 별도 처리하므로
    // 여기서는 config 만 전송한다. 에러는 토스트(remoteEditErrorMessage)로 안내한다.
    if (remote && isRemoteTarget(target)) {
      try {
        await updateRemoteAgent.mutateAsync({
          instanceID: target.instanceId,
          agentID: agentId,
          req: { config: omitMaskedSecrets(draft) },
        });
        setEditing(false);
        addNotification({ type: 'success', message: t('remote.edit.saveSuccess') });
      } catch (err) {
        addNotification({ type: 'error', message: remoteEditErrorMessage(err, t) });
      }
      return;
    }

    // 로컬 타깃: 기존 useConfigureAgent 경로(불변).
    try {
      await configureAgent.mutateAsync({ id: agentId, config: draft });
      setEditing(false);

      // transport 설정 변경 감지 시 재시작 알림
      const transportKeys = ['transport_type', 'serial_port', 'baud_rate', 'data_bits', 'stop_bits', 'parity', 'tcp_host', 'tcp_port'];
      const changed = transportKeys.some((k) => String(config[k] ?? '') !== String(draft[k] ?? ''));
      if (changed) {
        addNotification({ type: 'info', message: '연결 설정이 변경되어 에이전트가 재시작됩니다' });
      } else {
        addNotification({ type: 'success', message: '설정이 저장되었습니다' });
      }
    } catch {
      // 에러는 mutation 상태에서 표시
    }
  }, [
    remote,
    target,
    updateRemoteAgent,
    t,
    agentId,
    draft,
    config,
    configureAgent,
    addNotification,
  ]);

  if (isLoading) {
    return (
      <div className="space-y-3 p-4">
        {Array.from({ length: 3 }).map((_, i) => (
          <div key={i} className="h-10 animate-pulse rounded bg-(--color-bg-elevated)" />
        ))}
      </div>
    );
  }

  if (!agent) {
    return (
      <div className="p-4 text-sm text-(--color-text-muted)">
        에이전트 정보를 불러올 수 없습니다.
      </div>
    );
  }

  return (
    <div className="p-4">
      {/* 액션 버튼 — 로컬·원격 동형 인라인 편집(편집 → 폼 수정 → 저장).
          원격은 노드 승인+온라인(gating.nodeReady)일 때만 편집/저장 가능(REQ-J05). */}
      <div className="mb-3 flex items-center justify-end gap-2">
        {editing ? (
          <>
            <button
              type="button"
              onClick={handleCancel}
              disabled={isSaving}
              className="inline-flex items-center gap-1 rounded-md border border-(--color-border-strong) px-3 py-1.5 text-xs font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated) disabled:opacity-50"
            >
              <X className="h-3.5 w-3.5" />
              취소
            </button>
            <button
              type="button"
              onClick={handleSave}
              // v0.7.0 (M13): Store keys 검증 실패 시 저장 차단.
              // 원격은 노드 미-ready 시에도 저장 차단(REQ-J05 게이팅).
              disabled={isSaving || !storeKeysValid || editGated}
              data-testid="agent-config-save-button"
              className="inline-flex items-center gap-1 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
              title={
                editGated
                  ? t('remote.edit.actionGateHint')
                  : !storeKeysValid
                  ? '정적 키 설정에 오류가 있습니다 (data_type / metric_type 확인)'
                  : undefined
              }
            >
              <Save className="h-3.5 w-3.5" />
              {isSaving ? '저장 중...' : '저장'}
            </button>
          </>
        ) : (
          <button
            type="button"
            onClick={handleEdit}
            disabled={editGated}
            data-testid="agent-config-edit-button"
            title={editGated ? t('remote.edit.actionGateHint') : undefined}
            className="inline-flex items-center gap-1 rounded-md border border-(--color-border-strong) px-3 py-1.5 text-xs font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated) disabled:cursor-not-allowed disabled:opacity-50"
          >
            <Pencil className="h-3.5 w-3.5" />
            편집
          </button>
        )}
      </div>

      {/* 에러 메시지(로컬 저장 실패). 원격은 토스트로 안내하므로 표시하지 않는다. */}
      {!remote && configureAgent.isError && (
        <p className="mb-3 text-xs text-red-500 dark:text-red-400">
          설정 저장에 실패했습니다. 다시 시도해주세요.
        </p>
      )}

      {/* 연결 방식 변경 경고 (Samsung HVACR-01) */}
      {editing && agentType === 'samsung_hvacr01' && config.transport_type !== draft.transport_type && (
        <div className="mb-3 flex items-center gap-2 rounded-lg border border-amber-300 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-700 dark:bg-amber-950 dark:text-amber-300">
          <AlertTriangle className="h-4 w-4 shrink-0" />
          연결 방식 변경은 에이전트 재시작 후 적용됩니다.
        </div>
      )}

      {/* 즉시 적용 안내 (Samsung HVACR-01 편집 모드) */}
      {editing && agentType === 'samsung_hvacr01' && config.transport_type === draft.transport_type && (
        <p className="mb-3 text-xs text-(--color-text-muted)">
          상태 확인 요청 간격, 부저, 알람 설정은 저장 즉시 적용됩니다.
        </p>
      )}

      {/* 설정 폼 */}
      {agentType === 'store' && schema ? (
        // Store 는 운영/데이터 섹션으로 분리된 커스텀 레이아웃을 사용한다.
        // (SPEC-STORE-003)
        // v0.7.0 (M13): keys 검증 결과를 받아 저장 버튼 게이팅에 사용.
        <StoreConfigEditor
          data={editing ? draft : config}
          schema={schema}
          onChange={setDraft}
          onValidityChange={setStoreKeysValid}
          readOnly={!editing}
        />
      ) : HVACR_QUADRANT_AGENT_TYPES.has(agentType) && schema ? (
        // 3 HVACR 에이전트는 4-분면 (transport/protocol/operation/logging) 그리드를 사용한다.
        <FourQuadrantConfigLayout
          nodeId={agentId}
          data={editing ? draft : config}
          schema={schema}
          onChange={setDraft}
          readOnly={!editing}
          agentType={agentType}
          logLevel={
            remote
              ? undefined
              : {
                  agentId,
                  value: componentLogLevel,
                  updating: isLogLevelUpdating,
                  onChangeLevel: handleLogLevelChange,
                }
          }
        />
      ) : agentType in TWO_COL_CONFIG && schema ? (
        <TwoColumnConfigLayout
          nodeId={agentId}
          data={editing ? draft : config}
          schema={schema}
          onChange={setDraft}
          readOnly={!editing}
          agentType={agentType}
          logLevel={
            remote
              ? undefined
              : {
                  agentId,
                  value: componentLogLevel,
                  updating: isLogLevelUpdating,
                  onChangeLevel: handleLogLevelChange,
                }
          }
        />
      ) : (
        <DynamicForm
          nodeId={agentId}
          data={editing ? draft : config}
          schema={schema}
          onChange={setDraft}
          readOnly={!editing}
        />
      )}

      {/* 로그 레벨 설정 (HVACR 4-분면 + 2열 레이아웃은 자체적으로 로그 레벨을 포함).
          원격 타깃은 로그 레벨 변경(local monitorService)에 대응 백엔드가 없어 숨긴다. */}
      {!remote && !(agentType in TWO_COL_CONFIG) && !HVACR_QUADRANT_AGENT_TYPES.has(agentType) && <div className="mt-4 rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) p-3">
        <label
          htmlFor={`agent-log-level-${agentId}`}
          className="mb-1 block text-xs font-medium text-(--color-text-muted)"
        >
          로그 레벨
        </label>
        <select
          id={`agent-log-level-${agentId}`}
          value={componentLogLevel}
          onChange={(e) => handleLogLevelChange(e.target.value)}
          disabled={isLogLevelUpdating}
          className={cn(
            'block w-full rounded-md border border-(--color-border-strong) px-3 py-2 text-sm shadow-sm',
            'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500',
            'bg-(--color-bg-surface) text-(--color-text-primary)',
            'disabled:cursor-not-allowed disabled:opacity-50',
          )}
        >
          <option value="">기본값(Default)</option>
          <option value="debug">DEBUG</option>
          <option value="info">INFO</option>
          <option value="warn">WARN</option>
          <option value="error">ERROR</option>
        </select>
      </div>}
    </div>
  );
}

// ---- Modbus TCP Server 디바이스 섹션 ----

/** list_devices 응답 내 개별 디바이스 */
interface ModbusDevice {
  unit_id: number;
  name: string;
  register_counts: {
    coils: number;
    discrete_inputs: number;
    holding_registers: number;
    input_registers: number;
  };
  status: string;
  stats: {
    read_count: number;
    write_count: number;
    error_count: number;
  };
}

/** get_device_status 응답 */
interface ModbusDeviceDetail {
  unit_id: number;
  name: string;
  register_counts: Record<string, number>;
  register_map: {
    coils?: Record<string, boolean>;
    discrete_inputs?: Record<string, boolean>;
    holding_registers?: Record<string, number>;
    input_registers?: Record<string, number>;
  };
  stats: {
    read_count: number;
    write_count: number;
    error_count: number;
    last_access?: string;
  };
  created_at?: string;
}

/** 레지스터 영역 라벨 (Modbus 기능 코드 포함) */
const REGISTER_AREA_LABELS: Record<string, string> = {
  coils: 'Coils (FC01/05)',
  discrete_inputs: 'Discrete Inputs (FC02)',
  holding_registers: 'Holding Registers (FC03/06)',
  input_registers: 'Input Registers (FC04)',
};

/** 레지스터 영역 순서 */
const REGISTER_AREA_ORDER = ['coils', 'discrete_inputs', 'holding_registers', 'input_registers'] as const;

/** 레지스터 맵 테이블 컴포넌트 */
function RegisterMapTable({ registerMap }: { registerMap: ModbusDeviceDetail['register_map'] }) {
  const [expandedAreas, setExpandedAreas] = useState<Record<string, boolean>>({});

  const toggleArea = (area: string) => {
    setExpandedAreas((prev) => ({ ...prev, [area]: !prev[area] }));
  };

  // boolean 영역 (coils, discrete_inputs)
  const isBooleanArea = (area: string) => area === 'coils' || area === 'discrete_inputs';

  // 주소 정렬 (숫자 기준)
  const sortedEntries = (data: Record<string, unknown>) =>
    Object.entries(data).sort(([a], [b]) => Number(a) - Number(b));

  // 표시할 영역만 필터 (데이터가 있는 것만)
  const visibleAreas = REGISTER_AREA_ORDER.filter((area) => {
    const data = registerMap[area];
    return data && Object.keys(data).length > 0;
  });

  if (visibleAreas.length === 0) return null;

  return (
    <div className="space-y-1.5">
      <p className="text-xs font-medium text-(--color-text-muted)">레지스터 맵</p>
      {visibleAreas.map((area) => {
        const data = registerMap[area]!;
        const entries = sortedEntries(data);
        const isExpanded = expandedAreas[area] ?? false;
        const label = REGISTER_AREA_LABELS[area] ?? area;

        return (
          <div key={area} className="rounded border border-(--color-border-default)">
            {/* 영역 헤더 (클릭으로 접기/펼치기) */}
            <button
              type="button"
              onClick={() => toggleArea(area)}
              className="flex w-full items-center gap-1.5 px-2 py-1.5 text-left text-xs font-medium text-(--color-text-secondary) hover:bg-(--color-bg-elevated)"
            >
              {isExpanded ? (
                <ChevronDown className="h-3.5 w-3.5 flex-shrink-0 text-gray-400" />
              ) : (
                <ChevronRight className="h-3.5 w-3.5 flex-shrink-0 text-gray-400" />
              )}
              <span>{label}</span>
              <span className="ml-auto rounded-full bg-(--color-bg-elevated) px-1.5 py-0.5 text-[10px] font-normal text-(--color-text-muted)">
                {entries.length}
              </span>
            </button>

            {/* 레지스터 테이블 */}
            {isExpanded && (
              <div className="max-h-64 overflow-y-auto border-t border-(--color-border-default)">
                <table className="w-full text-xs">
                  <thead>
                    <tr className="bg-(--color-bg-primary) text-left text-(--color-text-muted)">
                      <th className="px-2 py-1 font-medium">주소</th>
                      {isBooleanArea(area) ? (
                        <th className="px-2 py-1 font-medium">값</th>
                      ) : (
                        <>
                          <th className="px-2 py-1 font-medium">Dec</th>
                          <th className="px-2 py-1 font-medium">Hex</th>
                        </>
                      )}
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-(--color-border-default)">
                    {entries.map(([addr, value]) => (
                      <tr key={addr} className="hover:bg-(--color-bg-elevated)">
                        <td className="px-2 py-1 font-mono text-(--color-text-secondary)">{addr}</td>
                        {isBooleanArea(area) ? (
                          <td className="px-2 py-1">
                            <span
                              className={cn(
                                'inline-block rounded px-1.5 py-0.5 text-[10px] font-medium',
                                value
                                  ? 'bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-400'
                                  : 'bg-(--color-bg-elevated) text-(--color-text-muted)',
                              )}
                            >
                              {value ? 'ON' : 'OFF'}
                            </span>
                          </td>
                        ) : (
                          <>
                            <td className="px-2 py-1 font-mono text-(--color-text-secondary)">
                              {typeof value === 'number' ? value : '-'}
                            </td>
                            <td className="px-2 py-1 font-mono text-(--color-text-muted)">
                              {typeof value === 'number'
                                ? `0x${value.toString(16).toUpperCase().padStart(4, '0')}`
                                : '-'}
                            </td>
                          </>
                        )}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

function ModbusDevicesSection({ agentId }: { agentId: string }) {
  const execAgent = useExecAgent();
  const addNotification = useUIStore((s) => s.addNotification);

  // 디바이스 목록
  const [devices, setDevices] = useState<ModbusDevice[]>([]);
  const [isLoadingDevices, setIsLoadingDevices] = useState(true);

  // 상세 보기
  const [selectedUnitId, setSelectedUnitId] = useState<number | null>(null);
  const [deviceDetail, setDeviceDetail] = useState<ModbusDeviceDetail | null>(null);
  const [isLoadingDetail, setIsLoadingDetail] = useState(false);

  // 추가 모달
  const [showAddModal, setShowAddModal] = useState(false);
  const [addUnitId, setAddUnitId] = useState('');
  const [addName, setAddName] = useState('');
  const [isAdding, setIsAdding] = useState(false);

  // 레지스터 맵 폼 상태 (영역별 블록 배열, 빈 배열 = 비활성)
  type RegBlock = { start: string; count: string };
  const [addRegAreas, setAddRegAreas] = useState<Record<string, RegBlock[]>>({
    holding_registers: [{ start: '0', count: '100' }],
    input_registers: [],
    coils: [],
    discrete_inputs: [],
  });

  // 삭제 확인
  const [deleteTarget, setDeleteTarget] = useState<number | null>(null);

  // 디바이스 목록 로드
  const fetchDevices = useCallback(() => {
    setIsLoadingDevices(true);
    execAgent.mutate(
      { id: agentId, req: { command: 'list_devices' } },
      {
        onSuccess: (res) => {
          const result = res as { result?: { devices?: ModbusDevice[]; total?: number } };
          const list = result?.result?.devices ?? [];
          setDevices(list);
          setIsLoadingDevices(false);
        },
        onError: () => {
          setIsLoadingDevices(false);
          addNotification({ type: 'error', message: '디바이스 목록을 불러올 수 없습니다' });
        },
      },
    );
  }, [agentId, execAgent, addNotification]);

  useEffect(() => {
    fetchDevices();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agentId]);

  // 디바이스 상세 로드
  const handleSelectDevice = useCallback((unitId: number) => {
    if (selectedUnitId === unitId) {
      setSelectedUnitId(null);
      setDeviceDetail(null);
      return;
    }
    setSelectedUnitId(unitId);
    setIsLoadingDetail(true);
    execAgent.mutate(
      { id: agentId, req: { command: 'get_device_status', params: { unit_id: unitId } } },
      {
        onSuccess: (res) => {
          const result = res as { result?: ModbusDeviceDetail };
          setDeviceDetail(result?.result ?? null);
          setIsLoadingDetail(false);
        },
        onError: () => {
          setDeviceDetail(null);
          setIsLoadingDetail(false);
        },
      },
    );
  }, [agentId, selectedUnitId, execAgent]);

  // 디바이스 추가
  const handleAddDevice = useCallback(() => {
    const unitId = parseInt(addUnitId, 10);
    if (isNaN(unitId) || unitId < 1 || unitId > 247) {
      addNotification({ type: 'error', message: '유닛 ID는 1~247 범위여야 합니다' });
      return;
    }

    const params: Record<string, unknown> = { unit_id: unitId };
    if (addName.trim()) params.name = addName.trim();

    // 구조화된 레지스터 맵 조립 (영역당 다중 블록 지원)
    const regMap: Record<string, unknown> = {};
    for (const [area, blocks] of Object.entries(addRegAreas)) {
      if (!blocks || blocks.length === 0) continue;
      const parsed: { start_address: number; count: number }[] = [];
      for (const blk of blocks) {
        const start = parseInt(blk.start, 10);
        const cnt = parseInt(blk.count, 10);
        if (isNaN(start) || isNaN(cnt) || cnt <= 0) {
          addNotification({ type: 'error', message: `${REGISTER_AREA_LABELS[area] ?? area}: 올바른 주소와 개수를 입력하세요` });
          return;
        }
        parsed.push({ start_address: start, count: cnt });
      }
      // 블록 1개면 객체, 2개 이상이면 배열 (백엔드 호환)
      regMap[area] = parsed.length === 1 ? parsed[0] : parsed;
    }
    if (Object.keys(regMap).length === 0) {
      addNotification({ type: 'error', message: '최소 하나의 레지스터 영역을 활성화하세요' });
      return;
    }
    params.register_map = regMap;

    setIsAdding(true);
    execAgent.mutate(
      { id: agentId, req: { command: 'add_device', params } },
      {
        onSuccess: (res) => {
          const result = res as { result?: { success?: boolean; error?: string } };
          if (result?.result?.success === false) {
            addNotification({ type: 'error', message: result.result.error ?? '디바이스 추가 실패' });
          } else {
            addNotification({ type: 'success', message: `디바이스 (Unit ${unitId})가 추가되었습니다` });
            setShowAddModal(false);
            setAddUnitId('');
            setAddName('');
            setAddRegAreas({
              holding_registers: [{ start: '0', count: '100' }],
              input_registers: [],
              coils: [],
              discrete_inputs: [],
            });
            fetchDevices();
          }
          setIsAdding(false);
        },
        onError: (err) => {
          addNotification({ type: 'error', message: `디바이스 추가 실패: ${err instanceof Error ? err.message : '알 수 없는 오류'}` });
          setIsAdding(false);
        },
      },
    );
  }, [agentId, addUnitId, addName, addRegAreas, execAgent, addNotification, fetchDevices]);

  // 디바이스 삭제
  const handleDeleteDevice = useCallback((unitId: number) => {
    execAgent.mutate(
      { id: agentId, req: { command: 'remove_device', params: { unit_id: unitId } } },
      {
        onSuccess: (res) => {
          const result = res as { result?: { success?: boolean; error?: string } };
          if (result?.result?.success === false) {
            addNotification({ type: 'error', message: result.result.error ?? '디바이스 제거 실패' });
          } else {
            addNotification({ type: 'success', message: `디바이스 (Unit ${unitId})가 제거되었습니다` });
            if (selectedUnitId === unitId) {
              setSelectedUnitId(null);
              setDeviceDetail(null);
            }
            fetchDevices();
          }
          setDeleteTarget(null);
        },
        onError: (err) => {
          addNotification({ type: 'error', message: `디바이스 제거 실패: ${err instanceof Error ? err.message : '알 수 없는 오류'}` });
          setDeleteTarget(null);
        },
      },
    );
  }, [agentId, selectedUnitId, execAgent, addNotification, fetchDevices]);

  const canDelete = useMemo(() => devices.length > 1, [devices.length]);

  if (isLoadingDevices) {
    return (
      <div className="grid grid-cols-1 gap-3 p-4 sm:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 3 }).map((_, i) => (
          <div
            key={i}
            className="h-32 animate-pulse rounded-lg bg-(--color-bg-elevated)"
          />
        ))}
      </div>
    );
  }

  return (
    <div className="space-y-3 p-4">
      {/* 헤더 */}
      <div className="flex items-center justify-between">
        <span className="text-xs text-(--color-text-muted)">
          {devices.length}개 디바이스
        </span>
        <button
          type="button"
          onClick={() => setShowAddModal(true)}
          className="inline-flex items-center gap-1 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
        >
          <Plus className="h-3.5 w-3.5" />
          디바이스 추가
        </button>
      </div>

      {/* 추가 모달 */}
      {showAddModal && (
        <div className="space-y-2 rounded-lg border border-blue-200 bg-blue-50 p-3 dark:border-blue-800 dark:bg-blue-950">
          <div className="text-sm font-medium text-(--color-text-primary)">디바이스 추가</div>
          <div>
            <label htmlFor="modbus-add-unit-id" className="mb-1 block text-xs font-medium text-(--color-text-muted)">
              유닛 ID (1-247) *
            </label>
            <input
              id="modbus-add-unit-id"
              type="number"
              min={1}
              max={247}
              placeholder="1"
              value={addUnitId}
              onChange={(e) => setAddUnitId(e.target.value)}
              className="block w-full rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm bg-(--color-bg-surface) text-(--color-text-primary)"
            />
          </div>
          <div>
            <label htmlFor="modbus-add-name" className="mb-1 block text-xs font-medium text-(--color-text-muted)">
              이름 (선택)
            </label>
            <input
              id="modbus-add-name"
              type="text"
              placeholder="예: 센서 디바이스 1"
              value={addName}
              onChange={(e) => setAddName(e.target.value)}
              className="block w-full rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm bg-(--color-bg-surface) text-(--color-text-primary)"
            />
          </div>
          <div>
            <p className="mb-1.5 text-xs font-medium text-(--color-text-muted)">
              레지스터 맵
            </p>
            <div className="space-y-1.5">
              {REGISTER_AREA_ORDER.map((area) => {
                const blocks = addRegAreas[area] ?? [];
                const isActive = blocks.length > 0;
                const label = REGISTER_AREA_LABELS[area] ?? area;
                const defaultCount = area === 'coils' || area === 'discrete_inputs' ? '8' : '100';
                return (
                  <div
                    key={area}
                    className={cn(
                      'rounded-md border p-2 transition-colors',
                      isActive
                        ? 'border-blue-200 bg-blue-50/50 dark:border-blue-800 dark:bg-blue-950/30'
                        : 'border-(--color-border-default) bg-(--color-bg-primary)',
                    )}
                  >
                    <div className="flex items-center justify-between">
                      <label className="flex items-center gap-2">
                        <input
                          type="checkbox"
                          checked={isActive}
                          onChange={(e) => {
                            setAddRegAreas((prev) => ({
                              ...prev,
                              [area]: e.target.checked ? [{ start: '0', count: defaultCount }] : [],
                            }));
                          }}
                          className="h-3.5 w-3.5 rounded border-gray-300 text-blue-600"
                        />
                        <span className="text-xs font-medium text-(--color-text-secondary)">
                          {label}
                        </span>
                      </label>
                      {isActive && (
                        <button
                          type="button"
                          onClick={() => {
                            setAddRegAreas((prev) => {
                              const cur = prev[area] ?? [];
                              return { ...prev, [area]: [...cur, { start: '0', count: defaultCount }] };
                            });
                          }}
                          className="text-[10px] font-medium text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300"
                        >
                          + 블록 추가
                        </button>
                      )}
                    </div>
                    {isActive && (
                      <div className="mt-1.5 space-y-1 pl-5">
                        {blocks.map((blk, idx) => (
                          <div key={idx} className="flex items-center gap-2">
                            <div className="flex items-center gap-1">
                              <span className="text-[10px] text-(--color-text-muted)">Start:</span>
                              <input
                                type="number"
                                min={0}
                                value={blk.start}
                                onChange={(e) => {
                                  const val = e.target.value;
                                  setAddRegAreas((prev) => {
                                    const cur = [...(prev[area] ?? [])];
                                    cur[idx] = { start: val, count: cur[idx]?.count ?? defaultCount };
                                    return { ...prev, [area]: cur };
                                  });
                                }}
                                className="w-20 rounded border border-(--color-border-strong) px-1.5 py-0.5 font-mono text-xs bg-(--color-bg-surface) text-(--color-text-primary)"
                              />
                            </div>
                            <div className="flex items-center gap-1">
                              <span className="text-[10px] text-(--color-text-muted)">Count:</span>
                              <input
                                type="number"
                                min={1}
                                value={blk.count}
                                onChange={(e) => {
                                  const val = e.target.value;
                                  setAddRegAreas((prev) => {
                                    const cur = [...(prev[area] ?? [])];
                                    cur[idx] = { start: cur[idx]?.start ?? '0', count: val };
                                    return { ...prev, [area]: cur };
                                  });
                                }}
                                className="w-20 rounded border border-(--color-border-strong) px-1.5 py-0.5 font-mono text-xs bg-(--color-bg-surface) text-(--color-text-primary)"
                              />
                            </div>
                            {blocks.length > 1 && (
                              <button
                                type="button"
                                onClick={() => {
                                  setAddRegAreas((prev) => {
                                    const cur = [...(prev[area] ?? [])];
                                    cur.splice(idx, 1);
                                    return { ...prev, [area]: cur };
                                  });
                                }}
                                className="rounded p-0.5 text-gray-400 hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-950"
                                title="블록 삭제"
                              >
                                <X className="h-3 w-3" />
                              </button>
                            )}
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={handleAddDevice}
              disabled={!addUnitId.trim() || isAdding}
              className="rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500"
            >
              {isAdding ? '추가 중...' : '추가'}
            </button>
            <button
              type="button"
              onClick={() => {
                setShowAddModal(false);
                setAddUnitId('');
                setAddName('');
                setAddRegAreas({
                  holding_registers: [{ start: '0', count: '100' }],
                  input_registers: [],
                  coils: [],
                  discrete_inputs: [],
                });
              }}
              className="rounded-md border border-(--color-border-strong) px-3 py-1.5 text-xs font-medium text-(--color-text-secondary) hover:bg-(--color-bg-elevated)"
            >
              취소
            </button>
          </div>
        </div>
      )}

      {/* 삭제 확인 다이얼로그 */}
      {deleteTarget !== null && (
        <div className="flex items-center gap-3 rounded-lg border border-red-200 bg-red-50 p-3 dark:border-red-800 dark:bg-red-950">
          <AlertTriangle className="h-4 w-4 flex-shrink-0 text-red-500" />
          <div className="flex-1">
            <p className="text-sm text-red-700 dark:text-red-300">
              Unit {deleteTarget} 디바이스를 삭제하시겠습니까?
            </p>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => handleDeleteDevice(deleteTarget)}
              className="rounded-md bg-red-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-red-700"
            >
              삭제
            </button>
            <button
              type="button"
              onClick={() => setDeleteTarget(null)}
              className="rounded-md border border-(--color-border-strong) px-3 py-1.5 text-xs font-medium text-(--color-text-secondary) hover:bg-(--color-bg-elevated)"
            >
              취소
            </button>
          </div>
        </div>
      )}

      {/* 디바이스 목록 */}
      {devices.length === 0 ? (
        <div className="p-6 text-center">
          <Server className="mx-auto h-8 w-8 text-gray-300 dark:text-gray-600" />
          <p className="mt-2 text-sm text-(--color-text-muted)">
            등록된 디바이스가 없습니다
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {devices.map((d) => (
            <div
              key={d.unit_id}
              className={cn(
                'cursor-pointer rounded-lg border bg-(--color-bg-surface) p-3 transition-colors',
                selectedUnitId === d.unit_id
                  ? 'border-blue-400 ring-1 ring-blue-400 dark:border-blue-500'
                  : 'border-(--color-border-default) hover:border-gray-300 dark:hover:border-gray-600',
              )}
              onClick={() => handleSelectDevice(d.unit_id)}
              role="button"
              tabIndex={0}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  handleSelectDevice(d.unit_id);
                }
              }}
            >
              {/* 카드 헤더 */}
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <span className="inline-flex h-6 min-w-[1.5rem] items-center justify-center rounded bg-(--color-bg-elevated) px-1.5 text-xs font-bold text-(--color-text-secondary)">
                    {d.unit_id}
                  </span>
                  <span className="text-sm font-medium text-(--color-text-primary)">
                    {d.name || `Device ${d.unit_id}`}
                  </span>
                </div>
                <div className="flex items-center gap-1.5">
                  <span
                    className={cn(
                      'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-medium',
                      d.status === 'active'
                        ? 'bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-400'
                        : 'bg-(--color-bg-elevated) text-(--color-text-muted)',
                    )}
                  >
                    <span
                      className={cn(
                        'h-1.5 w-1.5 rounded-full',
                        d.status === 'active' ? 'bg-green-500' : 'bg-gray-400',
                      )}
                    />
                    {d.status === 'active' ? '활성' : '비활성'}
                  </span>
                  <button
                    type="button"
                    onClick={(e) => {
                      e.stopPropagation();
                      setDeleteTarget(d.unit_id);
                    }}
                    disabled={!canDelete}
                    className={cn(
                      'rounded p-1 transition-colors',
                      canDelete
                        ? 'text-gray-400 hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-950'
                        : 'cursor-not-allowed text-gray-300 dark:text-gray-600',
                    )}
                    title={canDelete ? '디바이스 삭제' : '마지막 디바이스는 삭제할 수 없습니다'}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
              </div>

              {/* 레지스터 영역 카운트 */}
              <div className="mt-2 grid grid-cols-2 gap-1">
                <div className="text-[10px] text-(--color-text-muted)">
                  <span className="font-medium">Coils:</span> {d.register_counts.coils}
                </div>
                <div className="text-[10px] text-(--color-text-muted)">
                  <span className="font-medium">DI:</span> {d.register_counts.discrete_inputs}
                </div>
                <div className="text-[10px] text-(--color-text-muted)">
                  <span className="font-medium">HR:</span> {d.register_counts.holding_registers}
                </div>
                <div className="text-[10px] text-(--color-text-muted)">
                  <span className="font-medium">IR:</span> {d.register_counts.input_registers}
                </div>
              </div>

              {/* 통계 요약 */}
              <div className="mt-2 flex items-center gap-3 border-t border-(--color-border-default) pt-2">
                <span className="flex items-center gap-1 text-[10px] text-(--color-text-muted)">
                  <Activity className="h-3 w-3" />
                  R:{d.stats.read_count} W:{d.stats.write_count}
                </span>
                {d.stats.error_count > 0 && (
                  <span className="text-[10px] text-red-500">
                    E:{d.stats.error_count}
                  </span>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* 디바이스 상세 보기 */}
      {selectedUnitId !== null && (
        <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) p-4">
          <div className="mb-3 flex items-center justify-between">
            <h4 className="text-sm font-medium text-(--color-text-primary)">
              Unit {selectedUnitId} 상세 정보
            </h4>
            <button
              type="button"
              onClick={() => {
                setSelectedUnitId(null);
                setDeviceDetail(null);
              }}
              className="rounded p-1 text-gray-400 hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary)"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
          {isLoadingDetail ? (
            <div className="space-y-2">
              <div className="h-4 w-1/3 animate-pulse rounded bg-(--color-bg-elevated)" />
              <div className="h-4 w-2/3 animate-pulse rounded bg-(--color-bg-elevated)" />
            </div>
          ) : deviceDetail ? (
            <div className="space-y-3">
              {/* 요청 통계 */}
              <div>
                <p className="mb-1 text-xs font-medium text-(--color-text-muted)">요청 통계</p>
                <div className="grid grid-cols-3 gap-2">
                  <div className="rounded border border-(--color-border-default) bg-(--color-bg-surface) p-2 text-center">
                    <p className="text-xs text-(--color-text-muted)">읽기</p>
                    <p className="text-sm font-semibold text-(--color-text-primary)">{deviceDetail.stats.read_count}</p>
                  </div>
                  <div className="rounded border border-(--color-border-default) bg-(--color-bg-surface) p-2 text-center">
                    <p className="text-xs text-(--color-text-muted)">쓰기</p>
                    <p className="text-sm font-semibold text-(--color-text-primary)">{deviceDetail.stats.write_count}</p>
                  </div>
                  <div className="rounded border border-(--color-border-default) bg-(--color-bg-surface) p-2 text-center">
                    <p className="text-xs text-(--color-text-muted)">에러</p>
                    <p className={cn(
                      'text-sm font-semibold',
                      deviceDetail.stats.error_count > 0 ? 'text-red-500' : 'text-(--color-text-primary)',
                    )}>{deviceDetail.stats.error_count}</p>
                  </div>
                </div>
              </div>

              {/* 레지스터 맵 정보 */}
              {deviceDetail.register_map && Object.keys(deviceDetail.register_map).length > 0 && (
                <RegisterMapTable registerMap={deviceDetail.register_map} />
              )}

              {/* 생성 시간 */}
              {deviceDetail.created_at && (
                <p className="text-xs text-(--color-text-muted)">
                  생성: {new Date(deviceDetail.created_at).toLocaleString('ko-KR')}
                </p>
              )}
            </div>
          ) : (
            <p className="text-xs text-(--color-text-muted)">
              상세 정보를 불러올 수 없습니다.
            </p>
          )}
        </div>
      )}
    </div>
  );
}

// ---- 토픽 탭 (MQTT) ----

/** 토픽 통계 타입 */
interface TopicStatEntry {
  topic: string;
  count: number;
  bytes: number;
  updated_at: string;
}

/** 바이트를 읽기 쉬운 단위로 변환 */
function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB'];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const val = bytes / Math.pow(1024, i);
  return `${val < 10 ? val.toFixed(1) : Math.round(val)} ${units[i]}`;
}

interface SubscribedTopicEntry {
  topic: string;
  qos: number;
  total_count: number;
  total_bytes: number;
  received_topics: TopicStatEntry[];
}

function TopicsTab({ agentId }: { agentId: string }) {
  const target = useTargetContext();
  const { data: agent, isLoading: agentLoading } = useAgentDetailTarget(target, agentId, 'full');
  const { data: stats } = useAgentStatsTarget(target, agentId);

  const state = agent?.state as {
    subscribed_topics?: SubscribedTopicEntry[];
    unmatched_topics?: TopicStatEntry[];
    pub_topics?: TopicStatEntry[];
    max_pub_topics?: number;
  } | undefined;

  const subscribedTopics = state?.subscribed_topics ?? [];
  const unmatchedTopics = state?.unmatched_topics ?? [];
  const pubTopics = state?.pub_topics ?? [];

  const totalReceivedCount = subscribedTopics.reduce(
    (sum, s) => sum + s.received_topics.length, 0,
  ) + unmatchedTopics.length;

  if (agentLoading) {
    return (
      <div className="p-4">
        <div className="h-32 animate-pulse rounded-lg bg-(--color-bg-elevated)" />
      </div>
    );
  }

  return (
    <div className="space-y-4 p-4">
      {/* 통계 요약 */}
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <StatCard label="구독 토픽" value={subscribedTopics.length} />
        <StatCard label="수신 토픽" value={totalReceivedCount} />
        <StatCard label="발행 토픽" value={pubTopics.length} />
        <StatCard label="수신 메시지" value={stats?.messages_in.toLocaleString() ?? '-'} />
      </div>

      {/* 구독/수신 토픽 트리 */}
      <SubscriptionTree
        subscriptions={subscribedTopics}
        unmatchedTopics={unmatchedTopics}
      />

      {/* 발행 토픽 */}
      <TopicStatsTable
        title={`발행 토픽${state?.max_pub_topics ? ` (최대 ${state.max_pub_topics})` : ''}`}
        topics={pubTopics}
        emptyMessage="발행된 토픽이 없습니다."
      />
    </div>
  );
}

/** 구독/수신 토픽 트리 (접이식) */
function SubscriptionTree({
  subscriptions,
  unmatchedTopics,
}: {
  subscriptions: SubscribedTopicEntry[];
  unmatchedTopics: TopicStatEntry[];
}) {
  const [expanded, setExpanded] = useState<Record<string, boolean>>(() => {
    // 기본: 모두 펼침
    const init: Record<string, boolean> = {};
    for (const s of subscriptions) {
      init[s.topic] = true;
    }
    if (unmatchedTopics.length > 0) {
      init['__unmatched__'] = true;
    }
    return init;
  });

  const toggle = (key: string) =>
    setExpanded((prev) => ({ ...prev, [key]: !prev[key] }));

  const allKeys = [
    ...subscriptions.map((s) => s.topic),
    ...(unmatchedTopics.length > 0 ? ['__unmatched__'] : []),
  ];
  const allExpanded = allKeys.every((k) => expanded[k]);

  const toggleAll = () => {
    const next: Record<string, boolean> = {};
    for (const k of allKeys) {
      next[k] = !allExpanded;
    }
    setExpanded(next);
  };

  if (subscriptions.length === 0 && unmatchedTopics.length === 0) {
    return (
      <div>
        <h4 className="mb-2 text-sm font-medium text-(--color-text-primary)">구독 토픽</h4>
        <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-4 text-center">
          <p className="text-xs text-(--color-text-muted)">구독 중인 토픽이 없습니다.</p>
        </div>
      </div>
    );
  }

  return (
    <div>
      <div className="mb-2 flex items-center justify-between">
        <h4 className="text-sm font-medium text-(--color-text-primary)">구독 / 수신 토픽</h4>
        <button
          onClick={toggleAll}
          className="text-xs text-(--color-text-muted) hover:text-(--color-text-primary)"
        >
          {allExpanded ? '모두 접기' : '모두 펼치기'}
        </button>
      </div>
      <div className="overflow-hidden rounded-lg border border-(--color-border-default)">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-(--color-border-default) bg-(--color-bg-elevated)">
              <th className="px-4 py-2 text-left text-xs font-medium text-(--color-text-muted)">토픽</th>
              <th className="px-4 py-2 text-right text-xs font-medium text-(--color-text-muted)">메시지 수</th>
              <th className="px-4 py-2 text-right text-xs font-medium text-(--color-text-muted)">데이터량</th>
              <th className="px-4 py-2 text-right text-xs font-medium text-(--color-text-muted)">QoS</th>
              <th className="px-4 py-2 text-right text-xs font-medium text-(--color-text-muted)">최근 활동</th>
            </tr>
          </thead>
          <tbody>
            {subscriptions.map((sub) => (
              <SubscriptionRow
                key={sub.topic}
                sub={sub}
                isExpanded={!!expanded[sub.topic]}
                onToggle={() => toggle(sub.topic)}
              />
            ))}
            {unmatchedTopics.length > 0 && (
              <UnmatchedRow
                topics={unmatchedTopics}
                isExpanded={!!expanded['__unmatched__']}
                onToggle={() => toggle('__unmatched__')}
              />
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

/** 구독 패턴 행 + 하위 수신 토픽 행 */
function SubscriptionRow({
  sub,
  isExpanded,
  onToggle,
}: {
  sub: SubscribedTopicEntry;
  isExpanded: boolean;
  onToggle: () => void;
}) {
  const hasChildren = sub.received_topics.length > 0;
  const sorted = [...sub.received_topics].sort((a, b) => b.count - a.count);

  return (
    <>
      {/* 구독 패턴 행 */}
      <tr
        className={cn(
          'border-b border-(--color-border-default) bg-(--color-bg-elevated)/50',
          hasChildren && 'cursor-pointer hover:bg-(--color-bg-elevated)',
        )}
        onClick={hasChildren ? onToggle : undefined}
      >
        <td className="px-4 py-2 font-mono text-xs font-medium text-(--color-text-primary)">
          <span className="mr-1.5 inline-block w-3 text-center text-(--color-text-muted)">
            {hasChildren ? (isExpanded ? '\u25BC' : '\u25B6') : '\u00B7'}
          </span>
          {sub.topic}
          {hasChildren && (
            <span className="ml-2 text-(--color-text-muted)">({sub.received_topics.length})</span>
          )}
        </td>
        <td className="px-4 py-2 text-right text-xs font-medium text-(--color-text-primary)">
          {sub.total_count > 0 ? sub.total_count.toLocaleString() : '-'}
        </td>
        <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">
          {sub.total_bytes > 0 ? formatBytes(sub.total_bytes) : '-'}
        </td>
        <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">{sub.qos}</td>
        <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">-</td>
      </tr>
      {/* 수신 토픽 (자식) 행 */}
      {isExpanded && sorted.map((t, idx) => (
        <tr
          key={t.topic}
          className={cn(
            'border-b border-(--color-border-default) last:border-b-0',
            idx % 2 === 0 ? 'bg-(--color-bg-surface)' : 'bg-(--color-bg-primary)',
          )}
        >
          <td className="py-2 pr-4 pl-10 font-mono text-xs text-(--color-text-secondary)">{t.topic}</td>
          <td className="px-4 py-2 text-right text-xs text-(--color-text-primary)">{t.count.toLocaleString()}</td>
          <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">{formatBytes(t.bytes)}</td>
          <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">-</td>
          <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">
            {t.updated_at ? new Date(t.updated_at).toLocaleTimeString('ko-KR') : '-'}
          </td>
        </tr>
      ))}
    </>
  );
}

/** 매칭되지 않은 수신 토픽 그룹 */
function UnmatchedRow({
  topics,
  isExpanded,
  onToggle,
}: {
  topics: TopicStatEntry[];
  isExpanded: boolean;
  onToggle: () => void;
}) {
  const sorted = [...topics].sort((a, b) => b.count - a.count);
  const totalCount = topics.reduce((s, t) => s + t.count, 0);
  const totalBytes = topics.reduce((s, t) => s + t.bytes, 0);

  return (
    <>
      <tr
        className="cursor-pointer border-b border-(--color-border-default) bg-(--color-bg-elevated)/50 hover:bg-(--color-bg-elevated)"
        onClick={onToggle}
      >
        <td className="px-4 py-2 text-xs font-medium text-(--color-text-muted)">
          <span className="mr-1.5 inline-block w-3 text-center">
            {isExpanded ? '\u25BC' : '\u25B6'}
          </span>
          (기타 수신 토픽)
          <span className="ml-2">({topics.length})</span>
        </td>
        <td className="px-4 py-2 text-right text-xs font-medium text-(--color-text-primary)">
          {totalCount.toLocaleString()}
        </td>
        <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">{formatBytes(totalBytes)}</td>
        <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">-</td>
        <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">-</td>
      </tr>
      {isExpanded && sorted.map((t, idx) => (
        <tr
          key={t.topic}
          className={cn(
            'border-b border-(--color-border-default) last:border-b-0',
            idx % 2 === 0 ? 'bg-(--color-bg-surface)' : 'bg-(--color-bg-primary)',
          )}
        >
          <td className="py-2 pr-4 pl-10 font-mono text-xs text-(--color-text-secondary)">{t.topic}</td>
          <td className="px-4 py-2 text-right text-xs text-(--color-text-primary)">{t.count.toLocaleString()}</td>
          <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">{formatBytes(t.bytes)}</td>
          <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">-</td>
          <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">
            {t.updated_at ? new Date(t.updated_at).toLocaleTimeString('ko-KR') : '-'}
          </td>
        </tr>
      ))}
    </>
  );
}

/** 토픽 통계 테이블 (발행 토픽용) */
function TopicStatsTable({
  title,
  topics,
  emptyMessage,
}: {
  title: string;
  topics: TopicStatEntry[];
  emptyMessage: string;
}) {
  const sorted = [...topics].sort((a, b) => b.count - a.count);

  return (
    <div>
      <h4 className="mb-2 text-sm font-medium text-(--color-text-primary)">{title}</h4>
      {sorted.length === 0 ? (
        <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-4 text-center">
          <p className="text-xs text-(--color-text-muted)">{emptyMessage}</p>
        </div>
      ) : (
        <div className="overflow-hidden rounded-lg border border-(--color-border-default)">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-(--color-border-default) bg-(--color-bg-elevated)">
                <th className="px-4 py-2 text-left text-xs font-medium text-(--color-text-muted)">토픽</th>
                <th className="px-4 py-2 text-right text-xs font-medium text-(--color-text-muted)">메시지 수</th>
                <th className="px-4 py-2 text-right text-xs font-medium text-(--color-text-muted)">데이터량</th>
                <th className="px-4 py-2 text-right text-xs font-medium text-(--color-text-muted)">최근 활동</th>
              </tr>
            </thead>
            <tbody>
              {sorted.map((t, idx) => (
                <tr
                  key={t.topic}
                  className={cn(
                    'border-b border-(--color-border-default) last:border-b-0',
                    idx % 2 === 0 ? 'bg-(--color-bg-surface)' : 'bg-(--color-bg-primary)',
                  )}
                >
                  <td className="px-4 py-2 font-mono text-xs text-(--color-text-primary)">{t.topic}</td>
                  <td className="px-4 py-2 text-right text-xs text-(--color-text-primary)">{t.count.toLocaleString()}</td>
                  <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">{formatBytes(t.bytes)}</td>
                  <td className="px-4 py-2 text-right text-xs text-(--color-text-muted)">
                    {t.updated_at ? new Date(t.updated_at).toLocaleTimeString('ko-KR') : '-'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

// ---- 저장소 탭 ----

function formatTimeAgo(date: Date): string {
  const seconds = Math.floor((Date.now() - date.getTime()) / 1000);
  if (seconds < 60) return `${seconds}초 전`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}분 전`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}시간 전`;
  const days = Math.floor(hours / 24);
  return `${days}일 전`;
}

/**
 * 엔트리의 `tags` 필드에서 태그 맵을 추출한다.
 * 정적 키가 아닌 동적 키 엔트리는 `tags` 를 가지지 않아 null 을 반환한다.
 *
 * @spec SPEC-STORE-003
 */
function extractEntryTags(
  entry: Record<string, unknown>,
): Record<string, string> | null {
  const raw = entry.tags;
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null;
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(raw as Record<string, unknown>)) {
    if (typeof v === 'string') out[k] = v;
  }
  return Object.keys(out).length > 0 ? out : null;
}

function StoreEntryRow({
  entry,
  maxHistorySize,
  agentId,
  showTagsColumn,
  isStatic,
  onPromote,
  onReset,
  readOnly = false,
}: {
  entry: Record<string, unknown>;
  maxHistorySize: number;
  agentId: string;
  /** 태그 컬럼을 렌더링할지 여부. 어떤 엔트리도 태그를 갖지 않으면 부모가 false 전달. */
  showTagsColumn: boolean;
  /** READ-ONLY(원격 타깃): 변환/초기화/히스토리(exec) 어포던스를 숨긴다. */
  readOnly?: boolean;
  /**
   * 이 엔트리의 키가 정적(설정의 keys 배열에 등록됨)인지 여부.
   * @spec SPEC-STORE-003
   */
  isStatic: boolean;
  /**
   * 동적 키를 정적으로 승격할 때 호출되는 핸들러.
   * 정적 키 행에서는 사용되지 않는다.
   * @spec SPEC-STORE-003
   */
  onPromote: (key: string) => void;
  /**
   * 행별 초기화 핸들러. 정적/동적 모두에서 노출되며 클릭 시 부모가
   * 확인 다이얼로그를 띄운다.
   * @spec SPEC-STORE-003
   */
  onReset: (key: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [historyData, setHistoryData] = useState<Array<{ value: unknown; timestamp: string }> | null>(null);
  const [historyLoading, setHistoryLoading] = useState(false);
  const execAgent = useExecAgent();

  const valueStr = typeof entry.value === 'string' ? entry.value : JSON.stringify(entry.value);
  const truncated = valueStr.length > 60;
  const displayValue = truncated && !expanded ? valueStr.slice(0, 60) + '...' : valueStr;

  const updatedAt = entry.updated_at ? new Date(entry.updated_at as string) : null;
  const timeAgo = updatedAt ? formatTimeAgo(updatedAt) : '-';

  const stateHistoryCount = (entry.history_count as number) || 0;
  const historyCount = historyData !== null ? historyData.length : stateHistoryCount;
  // 히스토리 토글은 exec(get_history) 에 의존하므로 원격 READ-ONLY 에서는 비활성.
  const hasHistory = maxHistorySize > 0 && !readOnly;

  const handleRowClick = useCallback(() => {
    if (!hasHistory) return;

    if (historyOpen) {
      setHistoryOpen(false);
      return;
    }

    setHistoryOpen(true);
    setHistoryLoading(true);
    execAgent.mutate(
      {
        id: agentId,
        req: {
          command: 'get_history',
          params: {
            key: entry.key as string,
            namespace: (entry.namespace as string) || 'default',
          },
        },
      },
      {
        onSuccess: (res) => {
          const data = res as unknown as Record<string, unknown>;
          const history = (data?.history as Array<{ value: unknown; timestamp: string }>) ?? [];
          setHistoryData(history);
          setHistoryLoading(false);
        },
        onError: () => {
          setHistoryData([]);
          setHistoryLoading(false);
        },
      },
    );
  }, [hasHistory, historyOpen, execAgent, agentId, entry.key, entry.namespace]);

  // 히스토리 확장 행의 colSpan 계산:
  //   key + 타입 + value + ns + (선택적 tags) + ttl + (선택적 history) + updated + 액션
  // 타입과 액션 컬럼은 항상 렌더링되므로 +2.
  const colSpan =
    7 + (maxHistorySize > 0 ? 1 : 0) + (showTagsColumn ? 1 : 0);

  const entryTags = extractEntryTags(entry);

  // 동적 키의 "정적으로 변환" 버튼 클릭 핸들러.
  // 행 클릭(히스토리 토글)과 분리하기 위해 이벤트 전파를 막는다.
  const handlePromoteClick = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation();
      onPromote(entry.key as string);
    },
    [entry.key, onPromote],
  );

  // 행별 초기화 버튼 클릭 핸들러. 행 클릭(히스토리 토글)과 분리한다.
  // @spec SPEC-STORE-003
  const handleResetClick = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation();
      onReset(entry.key as string);
    },
    [entry.key, onReset],
  );

  return (
    <>
      <tr
        className={cn('hover:bg-(--color-bg-secondary)/50', hasHistory && 'cursor-pointer')}
        onClick={handleRowClick}
      >
        <td className="px-3 py-2 font-mono text-xs text-(--color-text-primary) max-w-[200px] truncate" title={entry.key as string}>
          <span className="inline-flex items-center gap-1">
            {hasHistory && (
              <ChevronRight className={cn('h-3 w-3 text-(--color-text-muted) transition-transform', historyOpen && 'rotate-90')} />
            )}
            {entry.key as string}
          </span>
        </td>
        {/* 타입 컬럼: 정적/동적 배지만 표시 (SPEC-STORE-003).
            액션 버튼은 마지막 "액션" 컬럼으로 분리하였다. */}
        <td className="px-3 py-2 text-xs">
          {isStatic ? (
            <span
              className="inline-flex items-center gap-1 rounded-full bg-emerald-100 px-2 py-0.5 text-[10px] font-medium text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300"
              title="설정에 등록된 정적 키"
            >
              <Lock className="h-2.5 w-2.5" aria-hidden="true" />
              정적
            </span>
          ) : (
            <span
              className="inline-flex items-center rounded-full bg-amber-100 px-2 py-0.5 text-[10px] font-medium text-amber-700 dark:bg-amber-900/40 dark:text-amber-300"
              title="설정에 없는 동적 키"
            >
              동적
            </span>
          )}
        </td>
        <td className="px-3 py-2 font-mono text-xs text-(--color-text-secondary) max-w-[300px]">
          <span
            className={truncated ? 'cursor-pointer hover:text-(--color-text-primary)' : ''}
            onClick={(e) => {
              if (truncated) {
                e.stopPropagation();
                setExpanded(!expanded);
              }
            }}
          >
            {displayValue}
          </span>
        </td>
        <td className="px-3 py-2 text-xs text-(--color-text-muted)">
          {(entry.namespace as string) || '-'}
        </td>
        {showTagsColumn && (
          <td className="px-3 py-2 text-xs text-(--color-text-muted)">
            {entryTags ? (
              <div className="flex flex-wrap gap-1">
                {Object.entries(entryTags).map(([k, v]) => (
                  <span
                    key={k}
                    className="inline-flex items-center rounded-full bg-(--color-bg-elevated) px-1.5 py-0.5 font-mono text-[10px] font-medium text-(--color-text-secondary)"
                  >
                    {k}={v}
                  </span>
                ))}
              </div>
            ) : (
              <span className="text-(--color-text-muted)">-</span>
            )}
          </td>
        )}
        <td className="px-3 py-2 text-xs text-(--color-text-muted)">
          {(entry.ttl as string) || '\u221E'}
        </td>
        {maxHistorySize > 0 && (
          <td className="px-3 py-2 text-xs text-(--color-text-muted)">
            {historyCount}
          </td>
        )}
        <td className="px-3 py-2 text-xs text-(--color-text-muted)" title={entry.updated_at as string}>
          {timeAgo}
        </td>
        {/* 액션 컬럼 (SPEC-STORE-003): 정적으로 변환 + 초기화 버튼.
            동적 키: [정적으로 변환] [초기화]
            정적 키: [초기화] */}
        <td className="px-3 py-2 text-right text-xs">
          <span className="inline-flex items-center gap-1">
            {!readOnly && !isStatic && (
              <button
                type="button"
                onClick={handlePromoteClick}
                className="inline-flex items-center gap-0.5 rounded p-1 text-(--color-text-muted) transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400"
                title="정적으로 변환"
                aria-label={`${entry.key as string} 키를 정적으로 변환`}
              >
                <ArrowUpCircle className="h-3.5 w-3.5" aria-hidden="true" />
              </button>
            )}
            {!readOnly && (
              <button
                type="button"
                onClick={handleResetClick}
                className="inline-flex items-center gap-0.5 rounded p-1 text-(--color-text-muted) transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                title={isStatic ? '히스토리 초기화' : '항목 삭제'}
                aria-label={`${entry.key as string} 키 초기화`}
              >
                <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
              </button>
            )}
            {readOnly && <span className="text-(--color-text-muted)">-</span>}
          </span>
        </td>
      </tr>
      {historyOpen && (
        <tr>
          <td colSpan={colSpan} className="bg-(--color-bg-secondary)/30 px-6 py-3">
            {historyLoading ? (
              <p className="text-xs text-(--color-text-muted)">로딩 중...</p>
            ) : historyData && historyData.length > 0 ? (
              <div className="space-y-1">
                <p className="text-xs font-medium text-(--color-text-muted) mb-2">
                  히스토리 ({historyData.length}건)
                </p>
                <div className="space-y-1">
                  {historyData.map((h, i) => (
                    <div key={i} className="flex items-baseline gap-3 text-xs">
                      <span className="text-(--color-text-muted) whitespace-nowrap">
                        {new Date(h.timestamp).toLocaleString('ko-KR')}
                      </span>
                      <span className="font-mono text-(--color-text-secondary)">
                        {typeof h.value === 'string' ? h.value : JSON.stringify(h.value)}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            ) : (
              <p className="text-xs text-(--color-text-muted)">히스토리가 없습니다</p>
            )}
          </td>
        </tr>
      )}
    </>
  );
}

/** 저장소 탭의 페이지네이션 옵션 값. */
const STORE_PAGE_SIZE_OPTIONS = [10, 25, 50, 100] as const;
type StorePageSize = (typeof STORE_PAGE_SIZE_OPTIONS)[number];

/**
 * 저장소 탭.
 *
 * @spec SPEC-WEB-005 v0.4.0
 *   - 데이터 뷰어 모달 트리거("데이터 보기" 버튼)를 '저장소' 탭 내부로 통합.
 *   - Store 에이전트 키 목록에 클라이언트 측 페이지네이션을 추가.
 *
 * `agentName` 은 Store 데이터 소스(라우트가 name 기반) 생성에 필수이므로
 * 부모에서 전달받는다.
 */
function StoreTab({ agentId, agentName }: { agentId: string; agentName?: string }) {
  const queryClient = useQueryClient();
  // SPEC-REMOTE-001 M8 (그룹 J): 저장소 엔트리 읽기는 타깃 전환. 원격 타깃은
  // READ-ONLY(REQ-J03) — 변환/초기화/데이터뷰어(local store 서비스 기반)는
  // 대응 백엔드가 없어 숨긴다(노드 측 store 쓰기는 그룹 D/I 경로에서만).
  const target = useTargetContext();
  const remote = isRemoteTarget(target);
  const { data: agent, isLoading } = useAgentDetailTarget(target, agentId, 'full');
  const configureAgent = useConfigureAgent();
  const addNotification = useUIStore((s) => s.addNotification);
  const [isRefreshing, setIsRefreshing] = useState(false);

  // --- 데이터 뷰어 모달 상태 ---
  const [modalOpen, setModalOpen] = useState(false);

  // --- 동적→정적 변환 모달 상태 (SPEC-STORE-003) ---
  // 변환 대상 키 이름. null 이면 모달 닫힘.
  const [promotingKey, setPromotingKey] = useState<string | null>(null);

  // --- 초기화 모달 상태 (SPEC-STORE-003) ---
  // 행별 초기화 대상 키 이름. null 이면 모달 닫힘.
  const [resettingKey, setResettingKey] = useState<string | null>(null);
  // 행별 초기화 진행 상태 (HTTP 요청 중).
  const [isResettingKey, setIsResettingKey] = useState(false);
  // 전체 초기화 모달 표시 여부.
  const [bulkResetOpen, setBulkResetOpen] = useState(false);
  // 전체 초기화 진행 상태.
  const [isBulkResetting, setIsBulkResetting] = useState(false);

  // --- 페이지네이션 상태 ---
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState<StorePageSize>(10);

  // --- 태그 필터 상태 (SPEC-STORE-003) ---
  // "tagKey=tagValue" 문자열 집합. AND 로직 (모두 일치하는 엔트리만 표시).
  const [selectedTags, setSelectedTags] = useState<Set<string>>(() => new Set());

  // 태그 쌍 목록 조회 (구버전 서버/태그 없음 은 빈 배열로 폴백).
  const tagPairsQuery = useStoreTagPairs(agentName);
  const tagPairs: StoreTagPair[] = tagPairsQuery.data ?? [];

  // v0.7.0 (M14, Phase D): 백엔드에서 자동 등록된 키의 메타데이터(data_type 포함)를
  // 가져온다. PromoteToStaticDialog 가 defaultDataType 으로 사전 채움하기 위함이다.
  // 구버전 서버에서는 빈 배열로 안전하게 폴백한다.
  // useMemo 로 감싸 참조 안정성을 보장 (downstream useMemo deps 안정화).
  // @spec SPEC-WEB-005 v0.7.0 (M14)
  const storeKeysQuery = useStoreKeysWithTags(agentName);
  const storeKeyObjects: StoreKeyObject[] = useMemo(
    () => storeKeysQuery.data?.keyObjects ?? [],
    [storeKeysQuery.data?.keyObjects],
  );

  const allEntries = useMemo(() => {
    const state = agent?.state as { entries?: Array<Record<string, unknown>> } | undefined;
    return state?.entries ?? [];
  }, [agent?.state]);

  // 정적 키 이름 집합 — 설정의 `keys` 배열에 등록된 키들.
  // 한 번 계산하여 행 렌더링 시 O(1) 조회로 사용한다.
  // @spec SPEC-STORE-003
  const staticKeyNames = useMemo(() => {
    const config = (agent?.config as Record<string, unknown> | undefined) ?? {};
    const rawKeys = config.keys;
    const set = new Set<string>();
    if (!Array.isArray(rawKeys)) return set;
    for (const item of rawKeys) {
      if (item && typeof item === 'object' && !Array.isArray(item)) {
        const rec = item as Record<string, unknown>;
        if (typeof rec.key === 'string' && rec.key !== '') {
          set.add(rec.key);
        }
      }
    }
    return set;
  }, [agent?.config]);

  const totalKeys = (agent?.state as { total_keys?: number } | undefined)?.total_keys ?? 0;
  const totalHistoryEntries = (agent?.state as { total_history_entries?: number } | undefined)?.total_history_entries ?? 0;
  const maxHistorySize = (agent?.state as { max_history_size?: number } | undefined)?.max_history_size ?? 0;

  // 태그 필터 적용 (AND 로직). 선택이 없으면 원본 그대로.
  const entries = useMemo(() => {
    if (selectedTags.size === 0) return allEntries;
    return allEntries.filter((e) => matchesTagFilter(extractEntryTags(e), selectedTags));
  }, [allEntries, selectedTags]);

  // 태그 컬럼 표시 여부: 필터링 전 전체 엔트리 중 하나라도 태그가 있으면 표시.
  // (필터링 후 엔트리만 기준으로 하면, 필터 해제 시 컬럼이 사라지는 UX 문제가 발생)
  const showTagsColumn = useMemo(() => {
    return allEntries.some((e) => extractEntryTags(e) !== null);
  }, [allEntries]);

  // --- 페이지네이션 파생 값 ---
  // 현재 페이지가 총 페이지 수를 초과할 때 (예: 새로고침 후 항목이 줄어든 경우)
  // 마지막 페이지로 clamp 한다.
  const totalEntries = entries.length;
  const totalPages = Math.max(1, Math.ceil(totalEntries / pageSize));
  const currentPage = Math.min(page, totalPages);
  const startIdx = (currentPage - 1) * pageSize;
  const visibleEntries = useMemo(
    () => entries.slice(startIdx, startIdx + pageSize),
    [entries, startIdx, pageSize],
  );

  // --- 태그 필터 핸들러 ---

  const handleToggleTag = useCallback((filterId: string) => {
    setSelectedTags((prev) => {
      const next = new Set(prev);
      if (next.has(filterId)) {
        next.delete(filterId);
      } else {
        next.add(filterId);
      }
      return next;
    });
    // 필터 변경 시 1페이지로 리셋.
    setPage(1);
  }, []);

  const handleClearTags = useCallback(() => {
    setSelectedTags(new Set());
    setPage(1);
  }, []);

  // --- 데이터 뷰어 모달용 데이터 소스 (Store 전용) ---
  // agentName 이 없는 에지 케이스 (이전 콜사이트 호환) 에서는 데이터 소스를 생성하지 않고
  // "데이터 보기" 버튼을 비활성화한다. 현재 콜사이트에서는 모두 agentName 을 전달한다.
  const dataSource = useSeriesDataSource({
    kind: 'store',
    agentName: agentName ?? '__no_agent_name__',
  });
  // 모달의 멀티셀렉트 옵션 풀 — Store 는 보통 100개 이상 키를 가지지 않으므로
  // 한 페이지에 1000개까지 로드한다.
  const allKeysQuery = dataSource.useKeys({ page: 1, size: 1000 });
  const allSeriesKeys = allKeysQuery.data?.keys ?? [];

  const handleRefresh = useCallback(async () => {
    setIsRefreshing(true);
    await queryClient.invalidateQueries({ queryKey: ['agents', agentId, 'full'] });
    setIsRefreshing(false);
  }, [queryClient, agentId]);

  const handlePageSizeChange = useCallback(
    (e: ChangeEvent<HTMLSelectElement>) => {
      const next = Number(e.target.value) as StorePageSize;
      setPageSize(next);
      // 페이지 크기 변경 시 1페이지로 리셋하여 가시 범위 혼란을 방지.
      setPage(1);
    },
    [],
  );

  const handlePrevPage = useCallback(() => {
    setPage((p) => Math.max(1, p - 1));
  }, []);

  const handleNextPage = useCallback(() => {
    setPage((p) => Math.min(totalPages, p + 1));
  }, [totalPages]);

  const handleOpenModal = useCallback(() => {
    setModalOpen(true);
  }, []);

  const handleCloseModal = useCallback(() => {
    setModalOpen(false);
  }, []);

  // 동적→정적 변환 모달 트리거 / 닫기 (SPEC-STORE-003).
  const handleOpenPromote = useCallback((key: string) => {
    setPromotingKey(key);
  }, []);

  const handleClosePromote = useCallback(() => {
    // 진행 중일 때는 무시 (PromoteToStaticDialog 자체가 isSubmitting=true 인 동안
    // Esc/배경 클릭을 막지만, 명시적 호출 경로에서도 안전하게 가드).
    if (configureAgent.isPending) return;
    setPromotingKey(null);
  }, [configureAgent.isPending]);

  /**
   * 동적 키를 정적으로 변환한다.
   * 기존 config 의 `keys` 배열에 새 엔트리를 추가하여 PUT /agents/{id}/config 을 호출한다.
   * 백엔드의 Configure 가 런타임 정책을 즉시 적용하고, NodeStoreAdapter 의 lazy resolver
   * 가 흐름 연속성을 보장한다.
   *
   * v0.7.0 (M14, Phase D): payload 시그니처가 진화하여 data_type / metric_type 을
   * 포함한다. data_type 은 dialog 가 manual 모드 검증으로 강제하므로 항상 존재한다.
   * metric_type 은 비어있으면 백엔드 default `"unknown"` 적용을 위해 entry 에서 생략한다.
   *
   * 에러 핸들링: SPEC-STORE-003 v0.3.0 신규 4종 + 마이그레이션 에러를 mapStoreError 로
   * 사용자 친화 한글 메시지로 매핑한다.
   *
   * @spec SPEC-STORE-003 v0.3.0
   * @spec SPEC-WEB-005 v0.7.0 (M14, M15)
   */
  const handlePromoteConfirm = useCallback(
    async (payload: PromoteToStaticPayload) => {
      if (!promotingKey) return;
      const currentConfig = (agent?.config as Record<string, unknown> | undefined) ?? {};
      const rawKeys = currentConfig.keys;
      const currentKeys: StoreKeyEntry[] = Array.isArray(rawKeys)
        ? (rawKeys as StoreKeyEntry[])
        : [];
      // 이미 등록된 키라면 (중복 클릭 등) 별도 알림 후 모달만 닫는다.
      if (staticKeyNames.has(promotingKey)) {
        addNotification({
          type: 'info',
          message: `'${promotingKey}' 키는 이미 정적 키입니다`,
        });
        setPromotingKey(null);
        return;
      }
      // v0.7.0 (M14): data_type 은 항상 포함, metric_type 은 비어있을 때만 생략.
      const newEntry: StoreKeyEntry = {
        key: promotingKey,
        data_type: payload.data_type,
        tags: payload.tags,
      };
      if (payload.metric_type && payload.metric_type !== '') {
        newEntry.metric_type = payload.metric_type;
      }
      const newKeys: StoreKeyEntry[] = [...currentKeys, newEntry];
      try {
        await configureAgent.mutateAsync({
          id: agentId,
          config: { ...currentConfig, keys: newKeys },
        });
        addNotification({
          type: 'success',
          message: `'${promotingKey}' 키가 정적으로 변환되었습니다`,
        });
        setPromotingKey(null);
        // 즉시 최신 config/state 를 반영하기 위해 캐시 무효화.
        await queryClient.invalidateQueries({ queryKey: ['agents', agentId] });
      } catch (err) {
        // 다이얼로그를 닫지 않고 사용자가 재시도할 수 있도록 한다.
        // v0.7.0 (M15): mapStoreError 로 백엔드 에러를 사용자 친화 메시지로 변환.
        const mapped = mapStoreError(err);
        addNotification({
          type: 'error',
          message: `변환 실패: ${mapped.userMessage}`,
        });
      }
    },
    [
      promotingKey,
      agent?.config,
      staticKeyNames,
      configureAgent,
      agentId,
      addNotification,
      queryClient,
    ],
  );

  // v0.7.0 (M14): 변환 대상 키의 default data_type 을 백엔드 메타데이터에서 조회한다.
  // 자동 등록된 동적 키의 경우 백엔드가 이미 직렬화 타입을 추론해 두었으므로,
  // 사용자가 다시 선택하지 않도록 사전 채움한다.
  // 메타데이터가 없는 경우(구버전 서버/조회 실패) undefined 를 반환하여 사용자가 명시적으로
  // 선택하도록 한다.
  // @spec SPEC-WEB-005 v0.7.0 (M14)
  const promotingKeyDefaultDataType = useMemo<DataType | undefined>(() => {
    if (!promotingKey) return undefined;
    const obj = storeKeyObjects.find((o) => o.key === promotingKey);
    return obj?.data_type;
  }, [promotingKey, storeKeyObjects]);

  // --- 초기화 핸들러 (SPEC-STORE-003) ---

  // 행별 초기화 모달 트리거.
  const handleOpenReset = useCallback((key: string) => {
    setResettingKey(key);
  }, []);

  // 행별 초기화 모달 닫기. 진행 중이면 무시.
  const handleCloseReset = useCallback(() => {
    if (isResettingKey) return;
    setResettingKey(null);
  }, [isResettingKey]);

  // 행별 초기화 실행.
  // - 정적 키: 히스토리만 삭제 (백엔드에서 자동 분기)
  // - 동적 키: 항목 자체 삭제 (백엔드에서 자동 분기)
  const handlePerKeyReset = useCallback(async () => {
    if (!resettingKey || !agentName) return;
    setIsResettingKey(true);
    try {
      const result = await resetStoreKey(agentName, resettingKey);
      const action =
        result.action === 'history_cleared' ? '히스토리 삭제' : '항목 삭제';
      addNotification({
        type: 'success',
        message: `'${resettingKey}' ${action} 완료`,
      });
      setResettingKey(null);
      // store / agents 캐시를 모두 무효화하여 즉시 UI 반영.
      // 실제 cache key 는 store.ts:356,404,424 에 정의되어 있음.
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['agents', agentId] }),
        queryClient.invalidateQueries({ queryKey: ['store', 'tags', agentName] }),
        queryClient.invalidateQueries({ queryKey: ['store', 'keys', agentName] }),
        queryClient.invalidateQueries({ queryKey: ['store', 'keys-with-tags', agentName] }),
      ]);
    } catch (err) {
      addNotification({
        type: 'error',
        message: `초기화 실패: ${err instanceof Error ? err.message : '알 수 없는 오류'}`,
      });
    } finally {
      setIsResettingKey(false);
    }
  }, [resettingKey, agentName, addNotification, queryClient, agentId]);

  // 전체 초기화 모달 트리거 / 닫기.
  const handleOpenBulkReset = useCallback(() => {
    setBulkResetOpen(true);
  }, []);

  const handleCloseBulkReset = useCallback(() => {
    if (isBulkResetting) return;
    setBulkResetOpen(false);
  }, [isBulkResetting]);

  // 전체 초기화 실행.
  const handleBulkReset = useCallback(async () => {
    if (!agentName) return;
    setIsBulkResetting(true);
    try {
      const result = await resetAllStoreKeys(agentName);
      addNotification({
        type: 'success',
        message: `초기화 완료: 정적 키 ${result.history_cleared}건 history 삭제, 동적 키 ${result.entries_deleted}건 삭제됨`,
      });
      setBulkResetOpen(false);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['agents', agentId] }),
        queryClient.invalidateQueries({ queryKey: ['store', 'tags', agentName] }),
        queryClient.invalidateQueries({ queryKey: ['store', 'keys', agentName] }),
        queryClient.invalidateQueries({ queryKey: ['store', 'keys-with-tags', agentName] }),
      ]);
    } catch (err) {
      addNotification({
        type: 'error',
        message: `초기화 실패: ${err instanceof Error ? err.message : '알 수 없는 오류'}`,
      });
    } finally {
      setIsBulkResetting(false);
    }
  }, [agentName, addNotification, queryClient, agentId]);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center p-8 text-(--color-text-muted)">
        로딩 중...
      </div>
    );
  }

  // agentName 이 전달되지 않은 경우 데이터 뷰어를 표시하지 않는다 (방어적 처리).
  // 원격 타깃은 local store 서비스(데이터 뷰어/전체 초기화) 백엔드가 없어 비활성.
  const canOpenViewer = Boolean(agentName) && !remote;

  return (
    <div className="p-4 space-y-3">
      {/* 헤더: 전체 카운트 + 페이지 크기 선택 + 데이터 보기/새로고침 */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <p className="text-sm text-(--color-text-muted)">
            전체 <span className="font-semibold text-(--color-text-primary)">{totalKeys}</span>개 키
            {maxHistorySize > 0 && (
              <> · 히스토리 <span className="font-semibold text-(--color-text-primary)">{totalHistoryEntries}</span>건</>
            )}
          </p>
          {/* 페이지 크기 선택 */}
          <label className="flex items-center gap-1.5 text-xs text-(--color-text-muted)">
            페이지 크기
            <select
              value={pageSize}
              onChange={handlePageSizeChange}
              className="rounded-md border border-(--color-border-default) bg-(--color-bg-primary) px-1.5 py-1 text-xs text-(--color-text-primary) focus:outline-none focus:ring-1 focus:ring-blue-500"
            >
              {STORE_PAGE_SIZE_OPTIONS.map((size) => (
                <option key={size} value={size}>
                  {size}
                </option>
              ))}
            </select>
          </label>
        </div>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={handleOpenModal}
            disabled={!canOpenViewer}
            className="inline-flex items-center gap-1.5 rounded-md border border-(--color-border-default) bg-(--color-bg-primary) px-2.5 py-1.5 text-xs font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-secondary) disabled:opacity-50"
          >
            <LineChart className="h-3.5 w-3.5" aria-hidden="true" />
            데이터 보기
          </button>
          {/* 전체 초기화 버튼 (SPEC-STORE-003).
              스타일: 다른 헤더 버튼(데이터 보기/새로고침)과 동일한 중립 톤.
              실제 destructive 동작은 ConfirmDialog 의 danger variant 가 담당한다.
              agentName 이 없거나 진행 중이면 비활성. totalEntries=0 이어도 클릭 가능
              (백엔드가 0건 응답을 정상 반환하므로 다이얼로그에서 "0개 키" 로 안내). */}
          <button
            type="button"
            onClick={handleOpenBulkReset}
            disabled={!canOpenViewer || isBulkResetting}
            className="inline-flex items-center gap-1.5 rounded-md border border-(--color-border-default) bg-(--color-bg-primary) px-2.5 py-1.5 text-xs font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-secondary) disabled:opacity-50"
          >
            <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
            전체 초기화
          </button>
          <button
            type="button"
            onClick={handleRefresh}
            disabled={isRefreshing}
            className="inline-flex items-center gap-1.5 rounded-md border border-(--color-border-default) bg-(--color-bg-primary) px-2.5 py-1.5 text-xs font-medium text-(--color-text-secondary) hover:bg-(--color-bg-secondary) disabled:opacity-50"
          >
            <RefreshCw className={cn('h-3.5 w-3.5', isRefreshing && 'animate-spin')} />
            새로고침
          </button>
        </div>
      </div>

      {/* 태그 필터 섹션 (SPEC-STORE-003): 태그 쌍이 하나도 없으면 전체를 숨긴다. */}
      {tagPairs.length > 0 && (
        <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) p-3">
          <TagFilterChips
            pairs={tagPairs}
            selected={selectedTags}
            onToggle={handleToggleTag}
            onClearAll={handleClearTags}
          />
        </div>
      )}

      {/* 테이블 */}
      {entries.length === 0 ? (
        <div className="rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) p-8 text-center text-sm text-(--color-text-muted)">
          {allEntries.length === 0
            ? '저장된 데이터가 없습니다'
            : '선택한 태그와 일치하는 항목이 없습니다'}
        </div>
      ) : (
        <>
          <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-(--color-border-default) bg-(--color-bg-secondary)">
                  <th className="px-3 py-2 text-left font-medium text-(--color-text-muted)">키</th>
                  {/* 타입 컬럼: 정적 vs 동적 (SPEC-STORE-003) — 항상 표시 */}
                  <th className="px-3 py-2 text-left font-medium text-(--color-text-muted)">타입</th>
                  <th className="px-3 py-2 text-left font-medium text-(--color-text-muted)">값</th>
                  <th className="px-3 py-2 text-left font-medium text-(--color-text-muted)">네임스페이스</th>
                  {showTagsColumn && (
                    <th className="px-3 py-2 text-left font-medium text-(--color-text-muted)">태그</th>
                  )}
                  <th className="px-3 py-2 text-left font-medium text-(--color-text-muted)">TTL</th>
                  {maxHistorySize > 0 && (
                    <th className="px-3 py-2 text-left font-medium text-(--color-text-muted)">히스토리</th>
                  )}
                  <th className="px-3 py-2 text-left font-medium text-(--color-text-muted)">갱신</th>
                  {/* 액션 컬럼 (SPEC-STORE-003): 행별 액션 버튼들. 항상 표시. */}
                  <th className="px-3 py-2 text-right font-medium text-(--color-text-muted)">액션</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-(--color-border-default)">
                {visibleEntries.map((entry) => (
                  <StoreEntryRow
                    key={entry.key as string}
                    entry={entry}
                    maxHistorySize={maxHistorySize}
                    agentId={agentId}
                    showTagsColumn={showTagsColumn}
                    isStatic={staticKeyNames.has(entry.key as string)}
                    onPromote={handleOpenPromote}
                    onReset={handleOpenReset}
                    readOnly={remote}
                  />
                ))}
              </tbody>
            </table>
          </div>

          {/* 페이지 네비게이션: 한 페이지에 모두 들어갈 때는 비노출 */}
          {totalPages > 1 && (
            <div className="flex items-center justify-end gap-2">
              <button
                type="button"
                onClick={handlePrevPage}
                disabled={currentPage === 1}
                className="inline-flex items-center gap-1 rounded-md border border-(--color-border-default) bg-(--color-bg-primary) px-2.5 py-1.5 text-xs font-medium text-(--color-text-secondary) hover:bg-(--color-bg-secondary) disabled:opacity-50"
              >
                이전
              </button>
              <span className="text-xs text-(--color-text-muted)">
                {currentPage} / {totalPages} 페이지
              </span>
              <button
                type="button"
                onClick={handleNextPage}
                disabled={currentPage === totalPages}
                className="inline-flex items-center gap-1 rounded-md border border-(--color-border-default) bg-(--color-bg-primary) px-2.5 py-1.5 text-xs font-medium text-(--color-text-secondary) hover:bg-(--color-bg-secondary) disabled:opacity-50"
              >
                다음
              </button>
            </div>
          )}
        </>
      )}

      {/* 데이터 뷰어 모달 (Store 에이전트 전용) */}
      {canOpenViewer && (
        <TsdbDataViewerModal
          isOpen={modalOpen}
          onClose={handleCloseModal}
          allSeriesKeys={allSeriesKeys}
          dataSource={dataSource}
          agentName={agentName}
        />
      )}

      {/* 동적→정적 변환 모달 (SPEC-STORE-003 v0.3.0, SPEC-WEB-005 v0.7.0 M14) */}
      <PromoteToStaticDialog
        isOpen={promotingKey !== null}
        onClose={handleClosePromote}
        keyName={promotingKey ?? ''}
        onConfirm={handlePromoteConfirm}
        isSubmitting={configureAgent.isPending}
        defaultDataType={promotingKeyDefaultDataType}
      />

      {/* 행별 초기화 모달 (SPEC-STORE-003) */}
      <ConfirmDialog
        isOpen={resettingKey !== null}
        onClose={handleCloseReset}
        onConfirm={handlePerKeyReset}
        title="키 초기화"
        message={
          <div className="space-y-2">
            <p>
              키:{' '}
              <code className="font-mono text-(--color-text-primary)">
                {resettingKey}
              </code>
            </p>
            <p>
              {resettingKey && staticKeyNames.has(resettingKey)
                ? '정적 키입니다 — 히스토리만 삭제됩니다 (현재 값은 유지).'
                : '동적 키입니다 — 항목 자체가 삭제됩니다 (다음 쓰기 시 재생성).'}
            </p>
            <p className="text-xs text-red-600 dark:text-red-400">
              이 작업은 되돌릴 수 없습니다.
            </p>
          </div>
        }
        confirmLabel="초기화"
        variant="danger"
        isSubmitting={isResettingKey}
      />

      {/* 전체 초기화 모달 (SPEC-STORE-003) */}
      <ConfirmDialog
        isOpen={bulkResetOpen}
        onClose={handleCloseBulkReset}
        onConfirm={handleBulkReset}
        title="저장소 전체 초기화"
        message={
          <div className="space-y-2">
            <p>
              총{' '}
              <strong className="text-(--color-text-primary)">
                {totalEntries}개 키
              </strong>
              {maxHistorySize > 0 ? `, ${totalHistoryEntries}건 히스토리` : ''}{' '}
              가 영향을 받습니다.
            </p>
            <ul className="ml-4 list-disc text-xs text-(--color-text-muted)">
              <li>정적 키: 히스토리만 삭제 (현재 값은 유지)</li>
              <li>동적 키: 항목 자체 삭제 (다음 쓰기 시 재생성)</li>
            </ul>
            <p className="text-xs text-red-600 dark:text-red-400">
              이 작업은 되돌릴 수 없습니다.
            </p>
          </div>
        }
        confirmLabel="초기화"
        variant="danger"
        isSubmitting={isBulkResetting}
      />
    </div>
  );
}

// ---- 세션 탭 (TCP Server) ----

/** TCP 연결 세션 정보 */
interface ConnectionSession {
  remote_addr: string;
  connected_at: string;
  bytes_sent: number;
  bytes_received: number;
  packets_sent: number;
  packets_received: number;
}

/** 연결 경과 시간 표시 */
function formatDuration(connectedAt: string): string {
  const diff = Date.now() - new Date(connectedAt).getTime();
  const sec = Math.floor(diff / 1000);
  if (sec < 60) return `${sec}초`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}분 ${sec % 60}초`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}시간 ${min % 60}분`;
  const d = Math.floor(hr / 24);
  return `${d}일 ${hr % 24}시간`;
}

function SessionsTab({ agentId }: { agentId: string }) {
  // SPEC-REMOTE-001 M8 (그룹 J): 세션 목록은 exec(list_connections) 기반이라
  // 원격 READ 프록시 매핑이 없다. 원격 타깃에서는 안내만 표시한다(graceful).
  const target = useTargetContext();
  const remote = isRemoteTarget(target);
  const [sessions, setSessions] = useState<ConnectionSession[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshKey, setRefreshKey] = useState(0);

  useEffect(() => {
    if (remote) {
      setLoading(false);
      return;
    }
    let cancelled = false;

    const fetchSessions = async () => {
      try {
        const res = await agentService.execAgent(agentId, { command: 'list_connections' });
        if (cancelled) return;
        // API envelope unwrap 결과에 따라 connections가 직접 또는 result 내부에 있을 수 있음
        const raw = res as unknown as Record<string, unknown>;
        const conns = (
          (raw?.connections as ConnectionSession[] | undefined)
          ?? ((raw?.result as Record<string, unknown> | undefined)?.connections as ConnectionSession[] | undefined)
          ?? []
        );
        setSessions(conns);
      } catch (err) {
        console.error('[SessionsTab] list_connections failed:', err);
        if (!cancelled) setSessions([]);
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    void fetchSessions();
    const timer = setInterval(() => void fetchSessions(), 5000);
    return () => { cancelled = true; clearInterval(timer); };
  }, [agentId, refreshKey, remote]);

  if (remote) {
    return (
      <div className="flex flex-col items-center gap-2 py-12 text-(--color-text-muted)">
        <Server className="h-8 w-8 opacity-40" aria-hidden="true" />
        <p className="text-sm">원격 노드에서는 세션 정보를 제공하지 않습니다.</p>
      </div>
    );
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12 text-sm text-(--color-text-muted)">
        로딩 중...
      </div>
    );
  }

  return (
    <div className="space-y-4 p-4">
      {/* 헤더 */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Server className="h-4 w-4 text-(--color-text-muted)" aria-hidden="true" />
          <span className="text-sm font-medium text-(--color-text-primary)">
            {sessions.length}개 연결
          </span>
        </div>
        <button
          type="button"
          onClick={() => setRefreshKey((k) => k + 1)}
          className="flex items-center gap-1 rounded-md px-2 py-1 text-xs text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated)"
        >
          <RefreshCw className="h-3 w-3" aria-hidden="true" />
          새로고침
        </button>
      </div>

      {sessions.length === 0 ? (
        <div className="flex flex-col items-center gap-2 py-12 text-(--color-text-muted)">
          <Server className="h-8 w-8 opacity-40" aria-hidden="true" />
          <p className="text-sm">연결된 세션이 없습니다</p>
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-(--color-border-default)">
                <th className="py-2 pr-4 text-left font-medium text-(--color-text-muted)">IP</th>
                <th className="py-2 pr-4 text-left font-medium text-(--color-text-muted)">포트</th>
                <th className="py-2 pr-4 text-left font-medium text-(--color-text-muted)">연결 시간</th>
                <th className="py-2 pr-4 text-right font-medium text-(--color-text-muted)">수신 패킷</th>
                <th className="py-2 pr-4 text-right font-medium text-(--color-text-muted)">송신 패킷</th>
                <th className="py-2 pr-4 text-right font-medium text-(--color-text-muted)">수신 데이터</th>
                <th className="py-2 text-right font-medium text-(--color-text-muted)">송신 데이터</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-(--color-border-default)">
              {sessions.map((s) => {
                const [ip, port] = s.remote_addr.includes(']')
                  ? [s.remote_addr.slice(0, s.remote_addr.lastIndexOf(':')), s.remote_addr.slice(s.remote_addr.lastIndexOf(':') + 1)]
                  : s.remote_addr.split(':').length === 2
                    ? s.remote_addr.split(':')
                    : [s.remote_addr, '-'];
                return (
                  <tr key={s.remote_addr}>
                    <td className="py-2 pr-4 font-mono text-(--color-text-primary)">{ip}</td>
                    <td className="py-2 pr-4 font-mono text-(--color-text-muted)">{port}</td>
                    <td className="py-2 pr-4 text-(--color-text-muted)" title={new Date(s.connected_at).toLocaleString()}>
                      {formatDuration(s.connected_at)}
                    </td>
                    <td className="py-2 pr-4 text-right font-mono text-(--color-text-muted)">
                      {(s.packets_received ?? 0).toLocaleString()}
                    </td>
                    <td className="py-2 pr-4 text-right font-mono text-(--color-text-muted)">
                      {(s.packets_sent ?? 0).toLocaleString()}
                    </td>
                    <td className="py-2 pr-4 text-right font-mono text-(--color-text-muted)">
                      {formatBytes(s.bytes_received)}
                    </td>
                    <td className="py-2 text-right font-mono text-(--color-text-muted)">
                      {formatBytes(s.bytes_sent)}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

// ---- 디바이스 탭 ----

// 디바이스 source 값을 사용자 친화적 라벨/색상으로 매핑.
// 수동(manual)=config|pinned, 자동(auto)=auto|bridge.
function sourceVariant(source: string): { label: string; manual: boolean } | null {
  switch (source) {
    case 'config':
      return { label: '설정', manual: true };
    case 'pinned':
      return { label: '고정', manual: true };
    case 'auto':
      return { label: '자동', manual: false };
    case 'bridge':
      return { label: '브리지', manual: false };
    default:
      return source ? { label: source, manual: false } : null;
  }
}

function DevicesTab({ agentId, agentType }: { agentId: string; agentType: string }) {
  // SPEC-REMOTE-001 M8 (그룹 J): 디바이스 탭은 exec(list_devices/add/remove)
  // 기반이라 원격 READ 프록시 매핑이 제한적이다. 원격 타깃은 안내만 표시한다.
  const detailTarget = useTargetContext();
  const detailRemote = isRemoteTarget(detailTarget);
  const { data: agent } = useAgent(detailRemote ? '' : agentId);
  // 에이전트 이름 로드 전에는 fetch skip — undefined 를 넘기면 useDevices 가
  // 전체 디바이스를 반환하여 다른 에이전트의 디바이스가 잠깐 노출되었다 사라지는
  // flash 발생. 빈 sentinel agent 이름으로 backend 가 빈 결과를 반환하게 함.
  const { data, isLoading } = useDevicesRealtime(
    agent?.name ? { agent: agent.name } : { agent: '__pending__' },
  );
  // 클릭 시 상세 패널 expand. 동시 1개만 펼침.
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const execAgent = useExecAgent();
  const addNotification = useUIStore((s) => s.addNotification);

  const devices = data?.data ?? [];
  const isNasa = agentType === 'samsung_hvacr01';
  const isLgap = agentType === 'lgap';

  // 소스 정보 (list_devices 응답에서 획득).
  // SPEC-DEVICE-IDENTITY-001 Phase D (M11 / D-T20):
  // backend list_devices 응답의 `device_id` (UUID, agent.ResolveDeviceID 가 반환)
  // 를 키로 사용하여 sourceMap 을 구성한다. 기존 composite key `agent:local_id`
  // 의 colon 분리 매칭은 PR4 (backend composite 제거) 이후 깨지므로 UUID
  // 직접 매칭으로 변경. Samsung NASA/LGAP backend 가 list_devices 응답에 `device_id`
  // (UUID) 와 `address` 둘 다 보내므로 양쪽 키 모두 구축하여 graceful 동작 보장.
  const [sourceMap, setSourceMap] = useState<Record<string, string>>({});
  // device UUID -> agent bus address (e.g., Samsung NASA "20.00.01" / LGAP zone "01").
  // ID 컬럼 표시에 사용. metadata.name 으로 이름이 사용자 정의된 경우에도
  // bus address 가 보존되도록 별도 map 유지.
  const [addressMap, setAddressMap] = useState<Record<string, string>>({});
  useEffect(() => {
    if ((!isNasa && !isLgap) || !agent) return;
    execAgent.mutate(
      { id: agentId, req: { command: 'list_devices' } },
      {
        onSuccess: (res) => {
          const items = (res as { data?: Array<{ address?: string; device_id?: string; source?: string }> })?.data;
          if (!Array.isArray(items)) return;
          const map: Record<string, string> = {};
          const addrs: Record<string, string> = {};
          for (const item of items) {
            const source = item.source ?? 'bridge';
            // UUID 키 (PR4 후에도 동작) + address 키 (Phase A~C 호환).
            if (item.device_id) map[item.device_id] = source;
            if (item.address) map[item.address] = source;
            if (item.device_id && item.address) addrs[item.device_id] = item.address;
          }
          setSourceMap(map);
          setAddressMap(addrs);
        },
      },
    );
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agentId, agent?.name]);

  // 추가 폼 상태
  const [showAddForm, setShowAddForm] = useState(false);
  const [newAddress, setNewAddress] = useState('');
  const [newDeviceId, setNewDeviceId] = useState('');
  const [newDeviceType, setNewDeviceType] = useState('');
  const [newDeviceName, setNewDeviceName] = useState('');
  // LGAP 전용
  const [newZone, setNewZone] = useState('');
  const [newLgapDeviceId, setNewLgapDeviceId] = useState('');

  // 원격 타깃: 디바이스 관리(exec 기반)는 그룹 D 명령 경로로만 가능하므로 안내만
  // 표시한다(REQ-J03/J12). 디바이스 자체 제어는 디바이스 페이지에서 수행한다.
  if (detailRemote) {
    return (
      <div className="flex flex-col items-center gap-2 py-12 text-(--color-text-muted)">
        <HardDrive className="h-8 w-8 opacity-40" aria-hidden="true" />
        <p className="text-sm">원격 노드의 디바이스는 디바이스 페이지에서 제어합니다.</p>
      </div>
    );
  }

  // Modbus TCP Server: 전용 디바이스 섹션 사용 (hooks 이후에 분기)
  if (agentType === 'modbus-tcp-server') {
    return <ModbusDevicesSection agentId={agentId} />;
  }

  function handleAddDevice() {
    if (isLgap) {
      const zoneVal = parseInt(newZone.trim(), 0);
      if (isNaN(zoneVal) || zoneVal < 0 || zoneVal > 255) {
        addNotification({ type: 'error', message: '존 주소는 0~255 범위여야 합니다 (0x00~0xFF)' });
        return;
      }
      execAgent.mutate(
        {
          id: agentId,
          req: {
            command: 'add_device',
            params: {
              zone: zoneVal,
              ...(newLgapDeviceId.trim() && { device_id: newLgapDeviceId.trim() }),
              ...(newDeviceName.trim() && { name: newDeviceName.trim() }),
            },
          },
        },
        {
          onSuccess: () => {
            setShowAddForm(false);
            setNewZone('');
            setNewLgapDeviceId('');
            setNewDeviceName('');
            addNotification({ type: 'success', message: 'LGAP 디바이스가 추가되었습니다' });
          },
          onError: (err) => {
            addNotification({ type: 'error', message: `디바이스 추가 실패: ${err instanceof Error ? err.message : '알 수 없는 오류'}` });
          },
        },
      );
      return;
    }

    if (!newAddress.trim()) return;
    execAgent.mutate(
      {
        id: agentId,
        req: {
          command: 'add_device',
          params: {
            address: newAddress.trim(),
            ...(newDeviceId.trim() && { device_id: newDeviceId.trim() }),
            ...(newDeviceType && { device_type: newDeviceType }),
            ...(newDeviceName.trim() && { name: newDeviceName.trim() }),
          },
        },
      },
      {
        onSuccess: () => {
          setShowAddForm(false);
          setNewAddress('');
          setNewDeviceId('');
          setNewDeviceType('');
          setNewDeviceName('');
          addNotification({ type: 'success', message: '디바이스가 추가되었습니다' });
        },
        onError: (err) => {
          addNotification({ type: 'error', message: `디바이스 추가 실패: ${err instanceof Error ? err.message : '알 수 없는 오류'}` });
        },
      },
    );
  }

  function handleRemoveDevice(deviceId: string, address?: string) {
    const params: Record<string, unknown> = {};
    if (deviceId) params.device_id = deviceId;
    else if (address) params.address = address;
    execAgent.mutate(
      { id: agentId, req: { command: 'remove_device', params } },
      {
        onSuccess: () => {
          addNotification({ type: 'success', message: '디바이스가 제거되었습니다' });
        },
        onError: (err) => {
          addNotification({ type: 'error', message: `디바이스 제거 실패: ${err instanceof Error ? err.message : '알 수 없는 오류'}` });
        },
      },
    );
  }

  // SPEC-DEVICE-IDENTITY-001 Phase D (M11 / D-T20):
  // ID 컬럼에 표시할 bus address (Samsung NASA "20.00.01", LGAP zone "01") 를 반환한다.
  // metadata.name 으로 사용자 정의 이름이 설정된 디바이스에서도 bus address 가
  // 보존되도록 addressMap (list_devices 응답) 을 우선 조회하고, fallback 으로
  // UUID short form 또는 composite legacy 형식을 사용한다.
  //
  // 이전 구현은 device.name 을 우선 반환했으나, "이름" 컬럼과 동일 값이 노출되어
  // ID 컬럼의 의미를 상실하던 결함을 수정 (2026-05-26).
  function deviceAddressLabel(device: { id: string; uid?: string; name: string }): string {
    // 1순위: list_devices 응답에서 받은 bus address (사람이 읽기 좋은 hex).
    const uid = device.uid ?? device.id;
    if (uid && addressMap[uid]) return addressMap[uid];
    // 2순위: UUID short form (Phase D+ 호환, address 미수신 시).
    if (device.uid) return device.uid.slice(0, 8);
    // 3순위: Phase A~C composite — colon 뒷부분만 추출 (legacy fallback).
    const parts = device.id.split(':');
    if (parts.length > 1) return parts.slice(1).join(':');
    // 4순위: 그 외 (UUID 자체) — UUID 8자리로 trim 하여 가독성 확보.
    if (device.id.length >= 8 && device.id.includes('-')) {
      return device.id.slice(0, 8);
    }
    return device.id;
  }

  // SPEC-DEVICE-IDENTITY-001 Phase D (M11 / D-T20):
  // sourceMap 은 UUID (`device.uid`) 와 address 두 키로 채워져 있으므로 (위 useEffect),
  // UUID 우선 lookup → name → composite colon 분리 (legacy) 순서로 매칭.
  function getSource(device: { id: string; uid?: string; name: string }): string {
    // UUID 직접 매칭 (PR4 후에도 동작, backend list_devices `device_id` UUID 키).
    if (device.uid) {
      const v = sourceMap[device.uid];
      if (v) return v;
    }
    // name (Samsung NASA address 같은 의미) 매칭.
    if (device.name) {
      const v = sourceMap[device.name];
      if (v) return v;
    }
    // Phase A~C 호환: composite colon 분리 후 dot-spaced 변환.
    const parts = device.id.split(':');
    const addr = parts.length > 1 ? parts.slice(1).join(':') : device.id;
    const spaced = addr.replace(/\./g, ' ');
    return sourceMap[spaced] ?? sourceMap[addr] ?? '';
  }

  if (isLoading) {
    return (
      <div className="grid grid-cols-2 gap-3 p-4 md:grid-cols-3">
        {Array.from({ length: 3 }).map((_, i) => (
          <div
            key={i}
            className="h-20 animate-pulse rounded-lg bg-(--color-bg-elevated)"
          />
        ))}
      </div>
    );
  }

  return (
    <div className="space-y-3 p-4">
      {/* 헤더: 추가 버튼 */}
      {(isNasa || isLgap) && (
        <div className="flex items-center justify-between">
          <span className="text-xs text-(--color-text-muted)">
            {devices.length}개 디바이스
          </span>
          <button
            type="button"
            onClick={() => setShowAddForm(!showAddForm)}
            className="inline-flex items-center gap-1 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            <Plus className="h-3.5 w-3.5" />
            디바이스 추가
          </button>
        </div>
      )}

      {/* 디바이스 추가 다이얼로그 */}
      {showAddForm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={() => setShowAddForm(false)}>
          <div
            className="mx-4 w-full max-w-md rounded-lg border border-(--color-border-default) bg-(--color-bg-primary) shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="border-b border-(--color-border-default) px-4 py-3">
              <h3 className="text-sm font-semibold text-(--color-text-primary)">디바이스 추가</h3>
            </div>
            <div className="space-y-3 p-4">
              {isLgap ? (
                <>
                  <div>
                    <label className="mb-1 block text-xs font-medium text-(--color-text-secondary)">존 주소</label>
                    <input
                      type="text"
                      placeholder="예: 0x11 또는 17"
                      value={newZone}
                      onChange={(e) => setNewZone(e.target.value)}
                      className="block w-full rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm bg-(--color-bg-surface) text-(--color-text-primary)"
                    />
                    <p className="mt-1 text-xs text-(--color-text-muted)">상위 니블=그룹, 하위 니블=유닛 (예: 0x11 = 그룹1 유닛1)</p>
                  </div>
                  <div>
                    <label className="mb-1 block text-xs font-medium text-(--color-text-secondary)">디바이스 ID (선택)</label>
                    <input
                      type="text"
                      placeholder="디바이스 ID"
                      value={newLgapDeviceId}
                      onChange={(e) => setNewLgapDeviceId(e.target.value)}
                      className="block w-full rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm bg-(--color-bg-surface) text-(--color-text-primary)"
                    />
                  </div>
                </>
              ) : (
                <>
                  <div>
                    <label className="mb-1 block text-xs font-medium text-(--color-text-secondary)">주소</label>
                    <input
                      type="text"
                      placeholder="예: 20 00 03"
                      value={newAddress}
                      onChange={(e) => setNewAddress(e.target.value)}
                      className="block w-full rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm bg-(--color-bg-surface) text-(--color-text-primary)"
                    />
                  </div>
                  <div>
                    <label className="mb-1 block text-xs font-medium text-(--color-text-secondary)">디바이스 ID (선택)</label>
                    <input
                      type="text"
                      placeholder="디바이스 ID"
                      value={newDeviceId}
                      onChange={(e) => setNewDeviceId(e.target.value)}
                      className="block w-full rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm bg-(--color-bg-surface) text-(--color-text-primary)"
                    />
                  </div>
                  <div>
                    <label className="mb-1 block text-xs font-medium text-(--color-text-secondary)">디바이스 종류</label>
                    <select
                      value={newDeviceType}
                      onChange={(e) => setNewDeviceType(e.target.value)}
                      className="block w-full rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm bg-(--color-bg-surface) text-(--color-text-primary)"
                    >
                      <option value="">자동 감지</option>
                      <option value="HVACR.IDU">실내기</option>
                      <option value="HVACR.ODU">실외기</option>
                    </select>
                  </div>
                </>
              )}
              {/* 이름 (공통, 선택) */}
              <div>
                <label className="mb-1 block text-xs font-medium text-(--color-text-secondary)">이름 (선택)</label>
                <input
                  type="text"
                  placeholder="디바이스 이름"
                  value={newDeviceName}
                  onChange={(e) => setNewDeviceName(e.target.value)}
                  className="block w-full rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm bg-(--color-bg-surface) text-(--color-text-primary)"
                />
              </div>
              <p className="text-xs text-(--color-text-muted)">동적으로 추가된 디바이스는 에이전트 재시작 시 초기화됩니다.</p>
            </div>
            <div className="flex items-center justify-end gap-2 border-t border-(--color-border-default) px-4 py-3">
              <button
                type="button"
                onClick={() => setShowAddForm(false)}
                className="rounded-md border border-(--color-border-strong) px-3 py-1.5 text-xs font-medium text-(--color-text-secondary) hover:bg-(--color-bg-elevated)"
              >
                취소
              </button>
              <button
                type="button"
                onClick={handleAddDevice}
                disabled={(isLgap ? !newZone.trim() : !newAddress.trim()) || execAgent.isPending}
                className="rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500"
              >
                {execAgent.isPending ? '추가 중...' : '추가'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 디바이스 목록 (테이블) */}
      {devices.length === 0 ? (
        <div className="p-6 text-center">
          <HardDrive className="mx-auto h-8 w-8 text-gray-300 dark:text-gray-600" />
          <p className="mt-2 text-sm text-(--color-text-muted)">
            등록된 디바이스가 없습니다
          </p>
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-(--color-border-default) text-left text-xs text-(--color-text-muted)">
                <th className="pb-2 pr-3 font-medium">이름</th>
                <th className="pb-2 pr-3 font-medium">ID</th>
                <th className="pb-2 pr-3 font-medium">종류</th>
                <th className="pb-2 pr-3 font-medium">연결</th>
                <th className="pb-2 pr-3 font-medium">소스</th>
                {(isNasa || isLgap) && <th className="pb-2 font-medium" />}
              </tr>
            </thead>
            <tbody className="divide-y divide-(--color-border-default)">
              {devices.map((d) => {
                const source = getSource(d);
                const variant = sourceVariant(source);
                const isManual = variant?.manual ?? false;
                const addressLabel = deviceAddressLabel(d);
                const rowKey = d.uid ?? d.id;
                const isExpanded = expandedId === rowKey;
                return (
                  <React.Fragment key={rowKey}>
                    <tr
                      onClick={() => setExpandedId(isExpanded ? null : rowKey)}
                      className="cursor-pointer text-(--color-text-primary) transition-colors hover:bg-(--color-bg-elevated)"
                    >
                      <td className="py-2 pr-3 font-medium">{d.name || addressLabel}</td>
                      <td className="py-2 pr-3 text-xs text-(--color-text-muted) font-mono">{addressLabel}</td>
                      <td className="py-2 pr-3 text-xs">{getDeviceTypeLabel(d.type)}</td>
                      <td className="py-2 pr-3"><DeviceStatusBadge online={d.online} /></td>
                      <td className="py-2 pr-3">
                        {variant && (
                          <span
                            className={cn(
                              'inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium',
                              isManual
                                ? 'bg-(--color-bg-elevated) text-(--color-text-muted)'
                                : 'bg-blue-100 text-blue-600 dark:bg-blue-900 dark:text-blue-400',
                            )}
                            title={isManual ? '수동 등록 (설정/고정)' : '자동 등록 (발견/브리지)'}
                          >
                            {isManual && <Lock className="h-2.5 w-2.5" />}
                            {variant.label}
                          </span>
                        )}
                      </td>
                      {(isNasa || isLgap) && (
                        <td className="py-2 text-right">
                          {!isManual && variant && (
                            <button
                              type="button"
                              onClick={(e) => {
                                e.stopPropagation();
                                handleRemoveDevice(d.name || '', addressLabel);
                              }}
                              disabled={execAgent.isPending}
                              className="rounded p-1 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 disabled:opacity-50 dark:hover:bg-red-950"
                              title="디바이스 제거"
                            >
                              <Trash2 className="h-3.5 w-3.5" />
                            </button>
                          )}
                        </td>
                      )}
                    </tr>
                    {isExpanded && (
                      <tr>
                        <td colSpan={(isNasa || isLgap) ? 6 : 5} className="bg-(--color-bg-sunken)">
                          <DeviceDetailPanel deviceId={d.id} />
                        </td>
                      </tr>
                    )}
                  </React.Fragment>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
