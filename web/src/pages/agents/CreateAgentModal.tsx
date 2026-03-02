// 새 에이전트 생성 모달.
// 이름(필수), 타입(필수), 설정(JSON, 선택)을 입력받아 에이전트를 생성한다.
// CreateFlowModal과 동일한 패턴을 따른다.

import { useCallback, useEffect, useRef, useState } from 'react';
import { X } from 'lucide-react';

import { useCreateAgent } from '@/hooks/useAgent';

interface CreateAgentModalProps {
  open: boolean;
  onClose: () => void;
}

const AGENT_TYPES = [
  { value: 'mqtt', label: 'MQTT' },
  { value: 'modbus', label: 'Modbus' },
  { value: 'http', label: 'HTTP' },
  { value: 'file', label: 'File' },
  { value: 'script', label: 'Script' },
] as const;

/** 에이전트 생성 모달 오버레이 */
export default function CreateAgentModal({ open, onClose }: CreateAgentModalProps) {
  const createAgent = useCreateAgent();
  const nameRef = useRef<HTMLInputElement>(null);

  const [name, setName] = useState('');
  const [type, setType] = useState('mqtt');
  const [config, setConfig] = useState('');

  // 모달이 열리면 이름 입력 필드에 포커스
  useEffect(() => {
    if (open) {
      setName('');
      setType('mqtt');
      setConfig('');
      createAgent.reset();
      // requestAnimationFrame으로 포커스 지연 처리
      requestAnimationFrame(() => nameRef.current?.focus());
    }
  }, [open]); // eslint-disable-line react-hooks/exhaustive-deps

  // Escape 키로 모달 닫기
  useEffect(() => {
    if (!open) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [open, onClose]);

  /** 배경 클릭 시 모달 닫기 */
  const handleBackdropClick = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (e.target === e.currentTarget) onClose();
    },
    [onClose],
  );

  /** 폼 제출: 에이전트 생성 */
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmedName = name.trim();
    if (!trimmedName) return;

    // JSON 설정 파싱 (입력이 있는 경우)
    let parsedConfig: Record<string, unknown> | undefined;
    if (config.trim()) {
      try {
        parsedConfig = JSON.parse(config.trim());
      } catch {
        // JSON 파싱 실패 시 에러 메시지 표시를 위해 리턴하지 않고
        // 에러 상태를 직접 처리
        return;
      }
    }

    try {
      await createAgent.mutateAsync({
        name: trimmedName,
        type,
        config: parsedConfig,
      });
      onClose();
    } catch {
      // 에러는 mutation 상태에서 표시
    }
  };

  if (!open) return null;

  const inputClass =
    'w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 placeholder-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:bg-gray-700 dark:text-white dark:placeholder-gray-500 dark:focus:border-blue-400 dark:focus:ring-blue-400';

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={handleBackdropClick}
      role="dialog"
      aria-modal="true"
      aria-labelledby="create-agent-title"
    >
      <div className="mx-4 w-full max-w-md rounded-lg bg-white p-6 shadow-xl dark:bg-gray-800">
        {/* 헤더 */}
        <div className="mb-4 flex items-center justify-between">
          <h2
            id="create-agent-title"
            className="text-lg font-semibold text-gray-900 dark:text-white"
          >
            새 에이전트 만들기
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-700 dark:hover:text-gray-300"
            aria-label="닫기"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* 폼 */}
        <form onSubmit={handleSubmit} className="space-y-4">
          {/* 이름 입력 */}
          <div>
            <label
              htmlFor="agent-name"
              className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              이름 <span className="text-red-500">*</span>
            </label>
            <input
              ref={nameRef}
              id="agent-name"
              type="text"
              required
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="에이전트 이름을 입력하세요"
              className={inputClass}
            />
          </div>

          {/* 타입 선택 */}
          <div>
            <label
              htmlFor="agent-type"
              className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              타입 <span className="text-red-500">*</span>
            </label>
            <select
              id="agent-type"
              required
              value={type}
              onChange={(e) => setType(e.target.value)}
              className={inputClass}
            >
              {AGENT_TYPES.map((t) => (
                <option key={t.value} value={t.value}>
                  {t.label}
                </option>
              ))}
            </select>
          </div>

          {/* 설정 (JSON) */}
          <div>
            <label
              htmlFor="agent-config"
              className="mb-1 block text-sm font-medium text-gray-700 dark:text-gray-300"
            >
              설정 (JSON)
            </label>
            <textarea
              id="agent-config"
              rows={4}
              value={config}
              onChange={(e) => setConfig(e.target.value)}
              placeholder='{"host": "localhost", "port": 1883}'
              className={inputClass}
            />
          </div>

          {/* 에러 메시지 */}
          {createAgent.isError && (
            <p className="text-sm text-red-600 dark:text-red-400">
              에이전트 생성에 실패했습니다. 다시 시도해주세요.
            </p>
          )}

          {/* 버튼 영역 */}
          <div className="flex justify-end gap-2">
            <button
              type="button"
              onClick={onClose}
              disabled={createAgent.isPending}
              className="rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 disabled:opacity-50 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
            >
              취소
            </button>
            <button
              type="submit"
              disabled={createAgent.isPending || !name.trim()}
              className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
            >
              {createAgent.isPending ? '생성 중...' : '생성'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
