// 하단 터미널 패널 — 두 종류의 실시간 출력을 탭으로 보여준다.
//
//  1) Debug Output : output 노드의 editor 출력(WebSocket debug.message).
//  2) 탭 출력      : 관찰(tap) 중인 임의 노드의 출력(WebSocket node.output).
//
// 두 스트림은 같은 message 형상을 공유하지만 출처가 다르므로, 탭 출력의 각
// 라인에는 노드 라벨 + 포트 + "tap" 뱃지를 달아 debug.message 와 구분한다.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ChevronDown, ChevronUp, Eye, Plus, Terminal, Trash2, X } from 'lucide-react';
import { useTranslation } from '@/lib/i18n';
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

// --- 로그 영역 크기 조절(드래그 리사이즈) 상수 ---
/** 로그 영역 기본 높이(px). 기존 h-48(=192px)과 동일 — 미저장/파싱 실패 시 폴백. */
const DEFAULT_LOG_HEIGHT = 192;
/** 로그 영역 최소 높이(px). 이 아래로 끌면 접힘 판정 대상이 된다. */
const MIN_LOG_HEIGHT = 96;
/** 최대 높이 비율 — 뷰포트 높이의 70%까지 늘릴 수 있다. */
const MAX_LOG_HEIGHT_RATIO = 0.7;
/** 조절된 높이 영속화용 localStorage 키. */
const LOG_HEIGHT_STORAGE_KEY = 'xflow.debugPanel.height';

/** 현재 뷰포트 기준 [min, max] 로 높이를 clamp 한다(SSR/미지원 방어). */
function clampLogHeight(h: number): number {
  const max =
    typeof window !== 'undefined'
      ? Math.max(MIN_LOG_HEIGHT, window.innerHeight * MAX_LOG_HEIGHT_RATIO)
      : DEFAULT_LOG_HEIGHT * 3;
  return Math.min(Math.max(h, MIN_LOG_HEIGHT), max);
}

/** localStorage 에서 저장 높이를 복원한다. 없거나 파싱 실패 시 기본값(192)으로 폴백. */
function readStoredLogHeight(): number {
  if (typeof window === 'undefined') return DEFAULT_LOG_HEIGHT;
  try {
    const raw = window.localStorage.getItem(LOG_HEIGHT_STORAGE_KEY);
    if (raw == null) return DEFAULT_LOG_HEIGHT;
    const n = Number.parseInt(raw, 10);
    return Number.isFinite(n) && n > 0 ? clampLogHeight(n) : DEFAULT_LOG_HEIGHT;
  } catch {
    return DEFAULT_LOG_HEIGHT;
  }
}

/** 조절된 높이를 localStorage 에 저장한다(미지원/차단 환경은 조용히 무시). */
function writeStoredLogHeight(h: number): void {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.setItem(LOG_HEIGHT_STORAGE_KEY, String(Math.round(h)));
  } catch {
    /* localStorage 미지원/할당량 초과 — 무시 */
  }
}

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

/** tap 메시지(전체 envelope 또는 임의 값)를 한 줄로 직렬화한다(렌더 표시용). */
function formatTapRecord(value: unknown): string {
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

/** 사용자가 만든 탭 출력 뷰. 노드/포트 필터는 서로 독립적이다('all' = 전체). */
interface TapView {
  /** 안정적인 뷰 식별자 (React key + 활성 뷰 선택용). */
  id: string;
  /** 노드 필터: 'all' = 전체, 그 외 = nodeId. */
  node: string;
  /** 포트 필터: 'all' = 전체, 그 외 = 포트명. */
  port: string;
}

export function DebugPanel() {
  const { t } = useTranslation();
  const [tab, setTab] = useState<PanelTab>('debug');
  const [entries, setEntries] = useState<DebugEntry[]>([]);
  const [isOpen, setIsOpen] = useState(false);
  const [autoScroll, setAutoScroll] = useState(true);
  const scrollRef = useRef<HTMLDivElement>(null);
  const idRef = useRef(0);
  const wsRef = useRef<WSClient | null>(null);

  // 로그 영역 높이(px). 마운트 시 localStorage 에서 복원(없으면 192px).
  const [logHeight, setLogHeight] = useState<number>(() => readStoredLogHeight());
  // 최신 높이 미러 — 포인터다운 시작 스냅샷 확보용(핸들러는 stable 유지).
  const logHeightRef = useRef(logHeight);
  useEffect(() => {
    logHeightRef.current = logHeight;
  }, [logHeight]);
  // 드래그 세션 상태. null 이면 유휴. startY/startHeight 는 시작 스냅샷,
  // currentHeight 는 매 move 마다 갱신되는 최신 높이(드래그 종료 시 저장).
  const dragRef = useRef<{
    startY: number;
    startHeight: number;
    currentHeight: number;
  } | null>(null);

  // 드래그 시작: 시작 좌표·높이를 스냅샷하고 포인터 캡처.
  const onHandlePointerDown = useCallback((e: React.PointerEvent) => {
    e.preventDefault();
    const startHeight = logHeightRef.current;
    dragRef.current = { startY: e.clientY, startHeight, currentHeight: startHeight };
    try {
      e.currentTarget.setPointerCapture(e.pointerId);
    } catch {
      /* jsdom 등 미지원 환경 — 무시 */
    }
  }, []);

  // 드래그 중: 패널이 에디터 하단이라 위로 끌면(clientY 감소) 높이 증가.
  // min의 절반 미만으로 끌어내리면 접기(setIsOpen(false)) + 직전 높이 복원.
  const onHandlePointerMove = useCallback((e: React.PointerEvent) => {
    const drag = dragRef.current;
    if (!drag) return;
    const delta = drag.startY - e.clientY;
    const rawTarget = drag.startHeight + delta;
    if (rawTarget < MIN_LOG_HEIGHT / 2) {
      dragRef.current = null;
      try {
        e.currentTarget.releasePointerCapture(e.pointerId);
      } catch {
        /* 무시 */
      }
      // 직전(드래그 시작) 높이를 유지해 재펼침 시 복원되게 한다. 저장은 하지 않는다.
      setLogHeight(drag.startHeight);
      setIsOpen(false);
      return;
    }
    const next = clampLogHeight(rawTarget);
    drag.currentHeight = next;
    setLogHeight(next);
  }, []);

  // 드래그 종료: 최종 높이를 localStorage 에 저장.
  const onHandlePointerUp = useCallback((e: React.PointerEvent) => {
    const drag = dragRef.current;
    if (!drag) return;
    dragRef.current = null;
    try {
      e.currentTarget.releasePointerCapture(e.pointerId);
    } catch {
      /* 무시 */
    }
    writeStoredLogHeight(drag.currentHeight);
  }, []);

  // 더블클릭: 기본 높이(192px)로 리셋 + localStorage 갱신.
  const onHandleDoubleClick = useCallback(() => {
    setLogHeight(DEFAULT_LOG_HEIGHT);
    writeStoredLogHeight(DEFAULT_LOG_HEIGHT);
  }, []);

  // 탭 출력 뷰 ID 발급용 카운터(렌더와 무관하게 단조 증가, 인메모리 전용).
  const viewIdRef = useRef(0);
  const genViewId = useCallback((): string => `view-${++viewIdRef.current}`, []);

  // 탭 출력 뷰 목록 — 각 뷰는 자체 {node, port} 독립 필터를 가진다.
  // 최초 1개 뷰({전체, 전체})로 시작한다. 영속화하지 않는다(인메모리 전용).
  const [tapViews, setTapViews] = useState<TapView[]>(() => [
    { id: 'view-0', node: 'all', port: 'all' },
  ]);
  const [activeTapView, setActiveTapView] = useState<string>('view-0');

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

  // 필터 옵션 소스 — 현재 탭 출력에 실제로 등장한 노드/포트만 노출한다.
  // 노드 옵션: 라벨순 정렬. (value=nodeId, label=노드 라벨)
  const tapNodeOptions = useMemo<{ id: string; label: string }[]>(() => {
    const seen = new Map<string, string>();
    for (const e of tapEntries) {
      if (!seen.has(e.nodeId)) {
        seen.set(e.nodeId, nodeLabelById.get(e.nodeId) ?? e.nodeId);
      }
    }
    return Array.from(seen, ([id, label]) => ({ id, label })).sort((a, b) =>
      a.label.localeCompare(b.label),
    );
  }, [tapEntries, nodeLabelById]);

  // 포트 옵션: 등장 포트의 정렬된 유니크 목록.
  const tapPortOptions = useMemo<string[]>(() => {
    const seen = new Set<string>();
    for (const e of tapEntries) seen.add(e.port);
    return Array.from(seen).sort((a, b) => a.localeCompare(b));
  }, [tapEntries]);

  // 활성 뷰 해석. 활성 ID가 없으면 첫 뷰로 폴백한다(빈 목록은 불가 — 최소 1개 유지).
  const activeView = useMemo<TapView>(() => {
    return (
      tapViews.find((v) => v.id === activeTapView) ??
      tapViews[0] ?? { id: 'view-0', node: 'all', port: 'all' }
    );
  }, [tapViews, activeTapView]);

  // 선택된 노드/포트가 더 이상 옵션에 없으면 'all'로 폴백한다(크래시·stale 방지).
  // 필터링·select 표시·뷰 라벨 모두 이 실효(effective) 값을 사용한다.
  const effectiveNode = useMemo<string>(() => {
    if (activeView.node === 'all') return 'all';
    return tapNodeOptions.some((o) => o.id === activeView.node) ? activeView.node : 'all';
  }, [activeView.node, tapNodeOptions]);

  const effectivePort = useMemo<string>(() => {
    if (activeView.port === 'all') return 'all';
    return tapPortOptions.includes(activeView.port) ? activeView.port : 'all';
  }, [activeView.port, tapPortOptions]);

  // 활성 뷰의 독립 AND 필터 적용: (노드='all' OR 일치) AND (포트='all' OR 일치).
  const visibleTapEntries = useMemo<TapEntry[]>(() => {
    return tapEntries.filter(
      (e) =>
        (effectiveNode === 'all' || e.nodeId === effectiveNode) &&
        (effectivePort === 'all' || e.port === effectivePort),
    );
  }, [tapEntries, effectiveNode, effectivePort]);

  // 뷰 탭 버튼 라벨을 필터로부터 자동 도출한다(실효 값 기준).
  //   all/all -> "전체", node/all -> {라벨}, all/port -> *:{port}, node/port -> {라벨}:{port}.
  const viewLabel = useCallback(
    (node: string, port: string): string => {
      const nodePart = node === 'all' ? null : (nodeLabelById.get(node) ?? node);
      if (node === 'all' && port === 'all') return t('editor.debug.viewAll');
      if (node !== 'all' && port === 'all')
        return nodePart ?? t('editor.debug.viewAll');
      if (node === 'all' && port !== 'all') return `*:${port}`;
      return `${nodePart}:${port}`;
    },
    [nodeLabelById, t],
  );

  // --- 뷰 추가/삭제/필터 갱신 (불변 업데이트, 인메모리 전용) ---

  const addTapView = useCallback(() => {
    const id = genViewId();
    setTapViews((prev) => [...prev, { id, node: 'all', port: 'all' }]);
    setActiveTapView(id);
  }, [genViewId]);

  const removeTapView = useCallback((id: string) => {
    setTapViews((prev) => {
      if (prev.length <= 1) return prev; // 최소 1개 유지.
      const next = prev.filter((v) => v.id !== id);
      // 활성 뷰를 지웠다면 남은 첫 뷰를 활성으로.
      setActiveTapView((cur) => (cur === id ? (next[0]?.id ?? cur) : cur));
      return next;
    });
  }, []);

  // 활성 뷰의 필터만 불변 갱신한다(다른 뷰는 그대로).
  const updateActiveView = useCallback(
    (patch: Partial<Pick<TapView, 'node' | 'port'>>) => {
      setTapViews((prev) =>
        prev.map((v) => (v.id === activeView.id ? { ...v, ...patch } : v)),
      );
    },
    [activeView.id],
  );

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
          <span>{t('editor.debug.tapTab')}</span>
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
            aria-label={t('editor.debug.clearAria')}
          >
            <Trash2 className="h-3.5 w-3.5 text-gray-500" />
          </button>
        )}
        <button
          type="button"
          onClick={() => setIsOpen((v) => !v)}
          className="rounded p-1 hover:bg-gray-200 dark:hover:bg-gray-700"
          aria-label={isOpen ? t('editor.debug.collapse') : t('editor.debug.expand')}
        >
          {isOpen ? (
            <ChevronDown className="h-3.5 w-3.5 text-gray-500" />
          ) : (
            <ChevronUp className="h-3.5 w-3.5 text-gray-500" />
          )}
        </button>
      </div>

      {/* 메시지 로그 영역 */}
      {isOpen && (
        <>
          {/* 상단 드래그 핸들 — 위로 끌면 확대, 아래로 끌면 축소, min 이하로 끌면 접힘.
              더블클릭 시 기본 높이(192px)로 리셋. isOpen 일 때만 렌더된다. */}
          <div
            role="separator"
            aria-orientation="horizontal"
            aria-label={t('editor.debug.resizeHandle')}
            data-testid="debug-resize-handle"
            onPointerDown={onHandlePointerDown}
            onPointerMove={onHandlePointerMove}
            onPointerUp={onHandlePointerUp}
            onDoubleClick={onHandleDoubleClick}
            className="h-1.5 shrink-0 cursor-row-resize touch-none border-t border-(--color-border-default) bg-gray-100 hover:bg-sky-500/40 dark:bg-gray-800 dark:hover:bg-sky-500/40"
          />
          <div
            ref={scrollRef}
            onScroll={handleScroll}
            data-testid="debug-log-area"
            style={{ height: logHeight }}
            className="overflow-y-auto bg-gray-950 font-mono text-xs"
          >
          {tab === 'debug' ? (
            entries.length === 0 ? (
              <div className="flex h-full items-center justify-center px-4 text-center text-gray-500">
                {t('editor.debug.debugEmpty')}
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
              {t('editor.debug.tapEmpty')}
            </div>
          ) : (
            <>
              {/* 뷰 컨트롤 — 고정 높이 로그 영역 내부 sticky 헤더라
                  탭 전환 시 패널 높이가 바뀌지 않는다(레이아웃 점프 방지).
                  Row 1: 뷰 탭(추가/삭제), Row 2: 활성 뷰의 노드/포트 필터. */}
              <div className="sticky top-0 z-10 bg-gray-900">
                {/* Row 1: 뷰 탭 + 추가 버튼 */}
                <div className="flex items-center gap-1 overflow-x-auto border-b border-gray-800 px-2 py-1">
                  {tapViews.map((v) => {
                    const isActive = v.id === activeView.id;
                    // 라벨도 실효 필터 기준으로 도출한다(사라진 옵션 → all 폴백, stale 방지).
                    const vNode =
                      v.node === 'all' || tapNodeOptions.some((o) => o.id === v.node)
                        ? v.node
                        : 'all';
                    const vPort =
                      v.port === 'all' || tapPortOptions.includes(v.port) ? v.port : 'all';
                    const label = viewLabel(vNode, vPort);
                    return (
                      <div
                        key={v.id}
                        className={
                          'flex shrink-0 items-center rounded ' +
                          (isActive ? 'bg-sky-500/20 text-sky-300' : 'text-gray-400')
                        }
                      >
                        <button
                          type="button"
                          onClick={() => setActiveTapView(v.id)}
                          title={label}
                          className={
                            'max-w-[12rem] truncate px-1.5 py-0.5 ' +
                            (isActive ? '' : 'rounded hover:bg-gray-800')
                          }
                        >
                          {label}
                        </button>
                        {tapViews.length > 1 && (
                          <button
                            type="button"
                            onClick={() => removeTapView(v.id)}
                            title={t('editor.debug.removeViewTitle')}
                            aria-label={t('editor.debug.removeView').replace(
                              '{label}',
                              label,
                            )}
                            className="rounded px-1 py-0.5 text-gray-400 hover:bg-gray-800 hover:text-gray-200"
                          >
                            <X className="h-3 w-3" />
                          </button>
                        )}
                      </div>
                    );
                  })}
                  <button
                    type="button"
                    onClick={addTapView}
                    title={t('editor.debug.addView')}
                    aria-label={t('editor.debug.addView')}
                    className="shrink-0 rounded px-1.5 py-0.5 text-gray-400 hover:bg-gray-800 hover:text-gray-200"
                  >
                    <Plus className="h-3.5 w-3.5" />
                  </button>
                </div>
                {/* Row 2: 활성 뷰의 노드/포트 필터 (서로 독립) */}
                <div className="flex items-center gap-2 border-b border-gray-800 px-2 py-1">
                  <label className="flex items-center gap-1 text-gray-400">
                    <span className="text-[10px]">{t('editor.debug.node')}</span>
                    <select
                      aria-label={t('editor.debug.nodeFilter')}
                      value={effectiveNode}
                      onChange={(e) => updateActiveView({ node: e.target.value })}
                      className="rounded border border-gray-700 bg-gray-800 px-1 py-0.5 text-gray-200"
                    >
                      <option value="all">{t('editor.debug.filterAll')}</option>
                      {tapNodeOptions.map((o) => (
                        <option key={o.id} value={o.id}>
                          {o.label}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label className="flex items-center gap-1 text-gray-400">
                    <span className="text-[10px]">{t('editor.debug.port')}</span>
                    <select
                      aria-label={t('editor.debug.portFilter')}
                      value={effectivePort}
                      onChange={(e) => updateActiveView({ port: e.target.value })}
                      className="rounded border border-gray-700 bg-gray-800 px-1 py-0.5 text-gray-200"
                    >
                      <option value="all">{t('editor.debug.filterAll')}</option>
                      {tapPortOptions.map((port) => (
                        <option key={port} value={port}>
                          {port}
                        </option>
                      ))}
                    </select>
                  </label>
                </div>
              </div>
              {visibleTapEntries.length === 0 ? (
                <div className="flex items-center justify-center px-4 py-6 text-center text-gray-500">
                  {t('editor.debug.noOutput')}
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
                      {formatTapRecord(entry.message)}
                    </td>
                  </tr>
                ))}
                  </tbody>
                </table>
              )}
            </>
          )}
          </div>
        </>
      )}
    </div>
  );
}
