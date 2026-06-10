// 하단 터미널 패널 — 두 종류의 실시간 출력을 탭으로 보여준다.
//
//  1) Debug Output : output 노드의 editor 출력(WebSocket debug.message).
//  2) 탭 출력      : 관찰(tap) 중인 임의 노드의 출력(WebSocket node.output).
//
// 두 스트림은 같은 message 형상을 공유하지만 출처가 다르므로, 탭 출력의 각
// 라인에는 노드 라벨 + 포트 + "tap" 뱃지를 달아 debug.message 와 구분한다.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ChevronDown, ChevronUp, Eye, Trash2, Terminal } from 'lucide-react';
import { createWSClient, type WSClient } from '@/services/ws/wsClient';
import { WS_MESSAGE_TYPES } from '@/services/ws/wsHandlers';
import { useAuthStore } from '@/stores/authStore';
import { useEditorStore } from '@/stores/editorStore';
import {
  useTapStore,
  type NodeOutputPayload,
  type TapEntry,
} from '@/stores/tapStore';

interface DebugMessage {
  node_id: string;
  message: string;
  timestamp: string;
}

interface DebugEntry {
  id: number;
  nodeId: string;
  message: string;
  time: string;
}

const MAX_ENTRIES = 500;

type PanelTab = 'debug' | 'tap';

/** epoch ms 또는 ISO 문자열을 ko-KR HH:mm:ss.SSS 로 포맷한다. */
function formatTime(value: number | string): string {
  return new Date(value).toLocaleTimeString('ko-KR', {
    hour12: false,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    fractionalSecondDigits: 3,
  });
}

/** tap 메시지의 payload 를 한 줄로 직렬화한다(렌더 표시용). */
function formatTapPayload(payload: Record<string, unknown>): string {
  try {
    return JSON.stringify(payload);
  } catch {
    return String(payload);
  }
}

export function DebugPanel() {
  const [tab, setTab] = useState<PanelTab>('debug');
  const [entries, setEntries] = useState<DebugEntry[]>([]);
  const [isOpen, setIsOpen] = useState(false);
  const [autoScroll, setAutoScroll] = useState(true);
  // 탭 출력 필터: 노드/포트 ('all' = 전체).
  const [tapNodeFilter, setTapNodeFilter] = useState<string>('all');
  const [tapPortFilter, setTapPortFilter] = useState<string>('all');
  const scrollRef = useRef<HTMLDivElement>(null);
  const idRef = useRef(0);
  const wsRef = useRef<WSClient | null>(null);

  // 탭 출력 상태 (zustand) — 관찰 중인 노드별 링 버퍼.
  const outputsByNode = useTapStore((s) => s.outputsByNode);
  const appendOutput = useTapStore((s) => s.appendOutput);
  const clearOutputs = useTapStore((s) => s.clearOutputs);

  // 노드 ID → 라벨 해석 맵 (탭 출력 라인 태깅용).
  const nodes = useEditorStore((s) => s.nodes);
  const nodeLabelById = useMemo(() => {
    const map = new Map<string, string>();
    for (const n of nodes) {
      const data = n.data as { label?: unknown } | undefined;
      const label = typeof data?.label === 'string' ? data.label : '';
      map.set(n.id, label || n.id);
    }
    return map;
  }, [nodes]);

  // 모든 노드의 tap 엔트리를 시간순(클라이언트 ID 순)으로 평탄화한다.
  const tapEntries = useMemo<TapEntry[]>(() => {
    const all: TapEntry[] = [];
    for (const list of Object.values(outputsByNode)) {
      all.push(...list);
    }
    all.sort((a, b) => a.id - b.id);
    return all;
  }, [outputsByNode]);

  // 필터 옵션: 현재 탭 출력에 등장한 노드 ID / 포트 목록.
  const tapNodeOptions = useMemo<string[]>(
    () => Array.from(new Set(tapEntries.map((e) => e.nodeId))),
    [tapEntries],
  );
  const tapPortOptions = useMemo<string[]>(
    () => Array.from(new Set(tapEntries.map((e) => e.port))).sort(),
    [tapEntries],
  );

  // 노드/포트 필터 적용. 선택값이 더 이상 존재하지 않으면 'all' 로 간주(전체 표시).
  const visibleTapEntries = useMemo<TapEntry[]>(() => {
    const nodeOk = tapNodeFilter === 'all' || !tapNodeOptions.includes(tapNodeFilter);
    const portOk = tapPortFilter === 'all' || !tapPortOptions.includes(tapPortFilter);
    if (nodeOk && portOk) return tapEntries;
    return tapEntries.filter(
      (e) =>
        (tapNodeFilter === 'all' || e.nodeId === tapNodeFilter) &&
        (tapPortFilter === 'all' || e.port === tapPortFilter),
    );
  }, [tapEntries, tapNodeFilter, tapPortFilter, tapNodeOptions, tapPortOptions]);

  const handleDebugMessage = useCallback((data: unknown) => {
    const msg = data as DebugMessage;
    const entry: DebugEntry = {
      id: ++idRef.current,
      nodeId: msg.node_id,
      message: msg.message,
      time: formatTime(msg.timestamp),
    };
    setEntries((prev) => {
      const next = [...prev, entry];
      return next.length > MAX_ENTRIES ? next.slice(-MAX_ENTRIES) : next;
    });
    // 새 메시지가 오면 패널을 자동으로 연다
    setIsOpen(true);
  }, []);

  const handleNodeOutput = useCallback(
    (data: unknown) => {
      // node.output 페이로드를 tap 스토어 링 버퍼에 적재한다.
      appendOutput(data as NodeOutputPayload);
      // 새 탭 출력이 오면 패널을 열고 탭 출력 탭으로 전환한다.
      setIsOpen(true);
      setTab('tap');
    },
    [appendOutput],
  );

  useEffect(() => {
    // 인증 활성 환경에서 /ws 는 토큰을 요구한다. 메인 모니터링 WS(useWebSocket)와
    // 동일하게 connect 시점마다 최신 access_token 을 동반시킨다(미동반 시 인증 실패로
    // debug.message·node.output 을 한 건도 받지 못한다).
    const client = createWSClient({
      tokenGetter: () => useAuthStore.getState().tokens?.access_token,
    });
    wsRef.current = client;
    client.on(WS_MESSAGE_TYPES.DEBUG_MESSAGE, handleDebugMessage);
    client.on(WS_MESSAGE_TYPES.NODE_OUTPUT, handleNodeOutput);
    client.connect();

    return () => {
      client.off(WS_MESSAGE_TYPES.DEBUG_MESSAGE, handleDebugMessage);
      client.off(WS_MESSAGE_TYPES.NODE_OUTPUT, handleNodeOutput);
      client.disconnect();
    };
  }, [handleDebugMessage, handleNodeOutput]);

  // 자동 스크롤 (활성 탭의 엔트리 변화에 반응).
  useEffect(() => {
    if (autoScroll && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [entries, tapEntries, tab, autoScroll]);

  const handleScroll = useCallback(() => {
    if (!scrollRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = scrollRef.current;
    setAutoScroll(scrollHeight - scrollTop - clientHeight < 40);
  }, []);

  const clearActive = useCallback(() => {
    if (tab === 'debug') {
      setEntries([]);
    } else {
      clearOutputs();
    }
  }, [tab, clearOutputs]);

  const debugCount = entries.length;
  const tapCount = tapEntries.length;

  return (
    <div className="border-t border-(--color-border-default) bg-white dark:bg-gray-950">
      {/* 헤더 바: 탭 전환 + 펼침/접힘 + 비우기 */}
      <div className="flex w-full items-center gap-1 px-2 py-1">
        <button
          type="button"
          onClick={() => {
            setTab('debug');
            setIsOpen(true);
          }}
          className={
            'flex items-center gap-1.5 rounded px-2 py-1 text-xs font-medium ' +
            (tab === 'debug'
              ? 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-100'
              : 'text-gray-500 hover:bg-gray-50 dark:text-gray-400 dark:hover:bg-gray-900/50')
          }
        >
          <Terminal className="h-3.5 w-3.5" />
          <span>Debug Output</span>
          {debugCount > 0 && (
            <span className="rounded-full bg-blue-100 px-1.5 py-0.5 text-[10px] font-semibold text-blue-700 dark:bg-blue-900/40 dark:text-blue-300">
              {debugCount}
            </span>
          )}
        </button>

        <button
          type="button"
          onClick={() => {
            setTab('tap');
            setIsOpen(true);
          }}
          className={
            'flex items-center gap-1.5 rounded px-2 py-1 text-xs font-medium ' +
            (tab === 'tap'
              ? 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-100'
              : 'text-gray-500 hover:bg-gray-50 dark:text-gray-400 dark:hover:bg-gray-900/50')
          }
        >
          <Eye className="h-3.5 w-3.5" />
          <span>탭 출력</span>
          {tapCount > 0 && (
            <span className="rounded-full bg-sky-100 px-1.5 py-0.5 text-[10px] font-semibold text-sky-700 dark:bg-sky-900/40 dark:text-sky-300">
              {tapCount}
            </span>
          )}
        </button>

        <span className="flex-1" />

        {isOpen && (
          <button
            type="button"
            onClick={clearActive}
            className="rounded p-1 hover:bg-gray-200 dark:hover:bg-gray-700"
            title="Clear"
            aria-label="출력 비우기"
          >
            <Trash2 className="h-3.5 w-3.5 text-gray-500" />
          </button>
        )}
        <button
          type="button"
          onClick={() => setIsOpen((v) => !v)}
          className="rounded p-1 hover:bg-gray-200 dark:hover:bg-gray-700"
          aria-label={isOpen ? '패널 접기' : '패널 펼치기'}
        >
          {isOpen ? (
            <ChevronDown className="h-3.5 w-3.5 text-gray-500" />
          ) : (
            <ChevronUp className="h-3.5 w-3.5 text-gray-500" />
          )}
        </button>
      </div>

      {/* 탭 출력 필터 (노드/포트) */}
      {isOpen && tab === 'tap' && tapEntries.length > 0 && (
        <div className="flex items-center gap-2 border-t border-(--color-border-default) bg-gray-900 px-2 py-1 text-xs">
          <span className="text-gray-400">필터</span>
          <select
            value={tapNodeFilter}
            onChange={(e) => setTapNodeFilter(e.target.value)}
            aria-label="노드 필터"
            className="rounded border border-gray-700 bg-gray-950 px-1.5 py-0.5 text-gray-200"
          >
            <option value="all">전체 노드</option>
            {tapNodeOptions.map((nid) => (
              <option key={nid} value={nid}>
                {nodeLabelById.get(nid) ?? nid}
              </option>
            ))}
          </select>
          <select
            value={tapPortFilter}
            onChange={(e) => setTapPortFilter(e.target.value)}
            aria-label="포트 필터"
            className="rounded border border-gray-700 bg-gray-950 px-1.5 py-0.5 text-gray-200"
          >
            <option value="all">전체 포트</option>
            {tapPortOptions.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
          {(tapNodeFilter !== 'all' || tapPortFilter !== 'all') && (
            <button
              type="button"
              onClick={() => {
                setTapNodeFilter('all');
                setTapPortFilter('all');
              }}
              className="text-gray-400 hover:text-gray-200"
            >
              필터 해제
            </button>
          )}
          <span className="ml-auto text-gray-500">
            {visibleTapEntries.length}/{tapEntries.length}
          </span>
        </div>
      )}

      {/* 메시지 로그 영역 */}
      {isOpen && (
        <div
          ref={scrollRef}
          onScroll={handleScroll}
          className="h-48 overflow-y-auto border-t border-(--color-border-default) bg-gray-950 font-mono text-xs"
        >
          {tab === 'debug' ? (
            entries.length === 0 ? (
              <div className="flex h-full items-center justify-center px-4 text-center text-gray-500">
                output 노드의 출력 대상을 &quot;editor&quot;로 설정하면 여기에 메시지가 표시됩니다.
              </div>
            ) : (
              <table className="w-full">
                <tbody>
                  {entries.map((entry) => (
                    <tr
                      key={entry.id}
                      className="border-b border-gray-800/50 hover:bg-gray-900/50"
                    >
                      <td className="whitespace-nowrap px-2 py-0.5 text-gray-500">
                        {entry.time}
                      </td>
                      <td className="whitespace-nowrap px-2 py-0.5 text-cyan-400">
                        {entry.nodeId}
                      </td>
                      <td className="px-2 py-0.5 text-gray-200 break-all">
                        {entry.message}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )
          ) : tapEntries.length === 0 ? (
            <div className="flex h-full items-center justify-center px-4 text-center text-gray-500">
              노드 카드의 눈 아이콘으로 관찰을 켜면 해당 노드의 출력이 여기에 표시됩니다.
            </div>
          ) : visibleTapEntries.length === 0 ? (
            <div className="flex h-full items-center justify-center px-4 text-center text-gray-500">
              선택한 노드/포트 필터에 해당하는 출력이 없습니다.
            </div>
          ) : (
            <table className="w-full">
              <tbody>
                {visibleTapEntries.map((entry) => (
                  <tr
                    key={entry.id}
                    className="border-b border-gray-800/50 hover:bg-gray-900/50"
                  >
                    <td className="whitespace-nowrap px-2 py-0.5 align-top text-gray-500">
                      {formatTime(entry.message.timestamp)}
                    </td>
                    <td className="whitespace-nowrap px-2 py-0.5 align-top">
                      <span className="mr-1 rounded-sm bg-sky-500/20 px-1 py-0.5 text-[10px] font-semibold text-sky-300">
                        tap
                      </span>
                      <span className="text-sky-400">
                        {nodeLabelById.get(entry.nodeId) ?? entry.nodeId}
                      </span>
                      <span className="text-gray-500">:{entry.port}</span>
                    </td>
                    <td className="px-2 py-0.5 text-gray-200 break-all">
                      {formatTapPayload(entry.message.payload)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}
    </div>
  );
}
