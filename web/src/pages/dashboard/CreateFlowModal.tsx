// 새 플로우 생성 모달.
// 이름(필수)과 설명(선택)을 입력받아 플로우를 생성하고
// 성공 시 에디터 페이지로 이동한다.

import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router';
import { X } from 'lucide-react';

import { useCreateFlow } from '@/hooks';

interface CreateFlowModalProps {
  open: boolean;
  onClose: () => void;
}

/** 플로우 생성 모달 오버레이 */
export default function CreateFlowModal({ open, onClose }: CreateFlowModalProps) {
  const navigate = useNavigate();
  const createFlow = useCreateFlow();
  const nameRef = useRef<HTMLInputElement>(null);

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');

  // 모달이 열리면 이름 입력 필드에 포커스
  useEffect(() => {
    if (open) {
      setName('');
      setDescription('');
      createFlow.reset();
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

  /** 폼 제출: 플로우 생성 후 에디터로 이동 */
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmedName = name.trim();
    if (!trimmedName) return;

    try {
      const result = await createFlow.mutateAsync({
        name: trimmedName,
        description: description.trim() || undefined,
        definition: {},
      });
      onClose();
      navigate(`/editor/${result.id}`);
    } catch {
      // 에러는 mutation 상태에서 표시
    }
  };

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={handleBackdropClick}
      role="dialog"
      aria-modal="true"
      aria-labelledby="create-flow-title"
    >
      <div className="mx-4 w-full max-w-md rounded-lg bg-(--color-bg-surface) p-6 shadow-xl">
        {/* 헤더 */}
        <div className="mb-4 flex items-center justify-between">
          <h2
            id="create-flow-title"
            className="text-lg font-semibold text-(--color-text-primary)"
          >
            새 플로우 만들기
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary)"
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
              htmlFor="flow-name"
              className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
            >
              이름 <span className="text-red-500">*</span>
            </label>
            <input
              ref={nameRef}
              id="flow-name"
              type="text"
              required
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="플로우 이름을 입력하세요"
              className="w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-2 text-sm text-(--color-text-primary) placeholder-(--color-text-muted) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            />
          </div>

          {/* 설명 입력 */}
          <div>
            <label
              htmlFor="flow-description"
              className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
            >
              설명
            </label>
            <textarea
              id="flow-description"
              rows={3}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="플로우에 대한 설명 (선택)"
              className="w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-2 text-sm text-(--color-text-primary) placeholder-(--color-text-muted) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            />
          </div>

          {/* 에러 메시지 */}
          {createFlow.isError && (
            <p className="text-sm text-red-600 dark:text-red-400">
              플로우 생성에 실패했습니다. 다시 시도해주세요.
            </p>
          )}

          {/* 버튼 영역 */}
          <div className="flex justify-end gap-2">
            <button
              type="button"
              onClick={onClose}
              disabled={createFlow.isPending}
              className="rounded-md border border-(--color-border-strong) px-4 py-2 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated) disabled:opacity-50"
            >
              취소
            </button>
            <button
              type="submit"
              disabled={createFlow.isPending || !name.trim()}
              className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
            >
              {createFlow.isPending ? '생성 중...' : '생성'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
