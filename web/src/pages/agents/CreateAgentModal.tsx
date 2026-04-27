// 새 에이전트 생성 모달.
// 이름(필수), 타입(필수), 타입별 설정 폼을 입력받아 에이전트를 생성한다.

import { useCallback, useEffect, useRef, useState } from 'react';
import { X } from 'lucide-react';

import { useCreateAgent } from '@/hooks/useAgent';
import { AGENT_TYPES, getAgentConfigDefaults, getAgentConfigSchema } from '@/config/agentSchemas';
import { DynamicForm } from '@/components/property/DynamicForm';

interface CreateAgentModalProps {
  open: boolean;
  onClose: () => void;
}

/** 에이전트 생성 모달 오버레이 */
export default function CreateAgentModal({ open, onClose }: CreateAgentModalProps) {
  const createAgent = useCreateAgent();
  const nameRef = useRef<HTMLInputElement>(null);

  const [name, setName] = useState('');
  const [type, setType] = useState<string>(AGENT_TYPES[0].value);
  const [config, setConfig] = useState<Record<string, unknown>>(() =>
    getAgentConfigDefaults(AGENT_TYPES[0].value),
  );

  const schema = getAgentConfigSchema(type);

  // 모달이 열리면 이름 입력 필드에 포커스
  useEffect(() => {
    if (open) {
      const defaultType = AGENT_TYPES[0].value;
      setName('');
      setType(defaultType);
      setConfig(getAgentConfigDefaults(defaultType));
      createAgent.reset();
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

  /** 타입 변경 시 설정 초기화 */
  const handleTypeChange = useCallback((newType: string) => {
    setType(newType);
    setConfig(getAgentConfigDefaults(newType));
  }, []);

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

    // 빈 값 제거 (default와 동일하거나 빈 문자열인 필드)
    const cleanConfig: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(config)) {
      if (v !== '' && v != null) {
        cleanConfig[k] = v;
      }
    }

    try {
      await createAgent.mutateAsync({
        name: trimmedName,
        type,
        config: Object.keys(cleanConfig).length > 0 ? cleanConfig : undefined,
      });
      onClose();
    } catch {
      // 에러는 mutation 상태에서 표시
    }
  };

  if (!open) return null;

  const inputClass =
    'w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-2 text-sm text-(--color-text-primary) placeholder-gray-400 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500';

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={handleBackdropClick}
      role="dialog"
      aria-modal="true"
      aria-labelledby="create-agent-title"
    >
      <div className="mx-4 w-full max-w-lg max-h-[80vh] overflow-y-auto rounded-lg bg-(--color-bg-surface) p-6 shadow-xl">
        {/* 헤더 */}
        <div className="mb-4 flex items-center justify-between">
          <h2
            id="create-agent-title"
            className="text-lg font-semibold text-(--color-text-primary)"
          >
            새 에이전트 만들기
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary)"
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
              className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
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
              className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
            >
              타입 <span className="text-red-500">*</span>
            </label>
            <select
              id="agent-type"
              required
              value={type}
              onChange={(e) => handleTypeChange(e.target.value)}
              className={inputClass}
            >
              {AGENT_TYPES.map((t) => (
                <option key={t.value} value={t.value}>
                  {t.label}
                </option>
              ))}
            </select>
          </div>

          {/* 타입별 설정 폼 */}
          {schema && (
            <div>
              <p className="mb-2 text-sm font-medium text-(--color-text-secondary)">
                설정
              </p>
              <div className="rounded-md border border-(--color-border-default) p-3">
                <DynamicForm
                  nodeId={`create-${type}`}
                  data={config}
                  schema={schema}
                  onChange={setConfig}
                />
              </div>
            </div>
          )}

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
              className="rounded-md border border-(--color-border-strong) px-4 py-2 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated) disabled:opacity-50"
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
