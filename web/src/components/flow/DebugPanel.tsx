// output 노드의 editor 출력을 실시간으로 표시하는 디버그 패널.
// WebSocket debug.message 이벤트를 수신하여 메시지 로그를 렌더링한다.

import { useCallback, useEffect, useRef, useState } from 'react';
import { ChevronDown, ChevronUp, Trash2, Terminal } from 'lucide-react';
import { createWSClient, type WSClient } from '@/services/ws/wsClient';
import { WS_MESSAGE_TYPES } from '@/services/ws/wsHandlers';

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

export function DebugPanel() {
  const [entries, setEntries] = useState<DebugEntry[]>([]);
  const [isOpen, setIsOpen] = useState(false);
  const [autoScroll, setAutoScroll] = useState(true);
  const scrollRef = useRef<HTMLDivElement>(null);
  const idRef = useRef(0);
  const wsRef = useRef<WSClient | null>(null);

  const handleDebugMessage = useCallback((data: unknown) => {
    const msg = data as DebugMessage;
    const entry: DebugEntry = {
      id: ++idRef.current,
      nodeId: msg.node_id,
      message: msg.message,
      time: new Date(msg.timestamp).toLocaleTimeString('ko-KR', {
        hour12: false,
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        fractionalSecondDigits: 3,
      }),
    };
    setEntries((prev) => {
      const next = [...prev, entry];
      return next.length > MAX_ENTRIES ? next.slice(-MAX_ENTRIES) : next;
    });
    // 새 메시지가 오면 패널을 자동으로 연다
    setIsOpen(true);
  }, []);

  useEffect(() => {
    const client = createWSClient();
    wsRef.current = client;
    client.on(WS_MESSAGE_TYPES.DEBUG_MESSAGE, handleDebugMessage);
    client.connect();

    return () => {
      client.off(WS_MESSAGE_TYPES.DEBUG_MESSAGE, handleDebugMessage);
      client.disconnect();
    };
  }, [handleDebugMessage]);

  // 자동 스크롤
  useEffect(() => {
    if (autoScroll && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [entries, autoScroll]);

  const handleScroll = useCallback(() => {
    if (!scrollRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = scrollRef.current;
    setAutoScroll(scrollHeight - scrollTop - clientHeight < 40);
  }, []);

  const clearEntries = useCallback(() => {
    setEntries([]);
  }, []);

  const entryCount = entries.length;

  return (
    <div className="border-t border-(--color-border-default) bg-white dark:bg-gray-950">
      {/* 헤더 바 */}
      <button
        type="button"
        onClick={() => setIsOpen((v) => !v)}
        className="flex w-full items-center gap-2 px-3 py-1.5 text-xs font-medium text-gray-600 hover:bg-gray-50 dark:text-gray-400 dark:hover:bg-gray-900/50"
      >
        <Terminal className="h-3.5 w-3.5" />
        <span>Debug Output</span>
        {entryCount > 0 && (
          <span className="rounded-full bg-blue-100 px-1.5 py-0.5 text-[10px] font-semibold text-blue-700 dark:bg-blue-900/40 dark:text-blue-300">
            {entryCount}
          </span>
        )}
        <span className="flex-1" />
        {isOpen && (
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              clearEntries();
            }}
            className="rounded p-0.5 hover:bg-gray-200 dark:hover:bg-gray-700"
            title="Clear"
          >
            <Trash2 className="h-3.5 w-3.5" />
          </button>
        )}
        {isOpen ? (
          <ChevronDown className="h-3.5 w-3.5" />
        ) : (
          <ChevronUp className="h-3.5 w-3.5" />
        )}
      </button>

      {/* 메시지 로그 영역 */}
      {isOpen && (
        <div
          ref={scrollRef}
          onScroll={handleScroll}
          className="h-48 overflow-y-auto border-t border-(--color-border-default) bg-gray-950 font-mono text-xs"
        >
          {entries.length === 0 ? (
            <div className="flex h-full items-center justify-center text-gray-500">
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
          )}
        </div>
      )}
    </div>
  );
}
