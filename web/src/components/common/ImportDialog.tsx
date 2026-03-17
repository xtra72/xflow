// 가져오기 대화 상자.
// 파일 선택(드래그 앤 드롭 포함)으로 JSON/YAML 파일을 파싱하고,
// 미리보기 후 플로우 또는 에이전트를 일괄 생성한다.
// 플로우 가져오기 시 참조된 에이전트가 서버에 없으면 자동 생성 옵션을 제공한다.

import { useCallback, useEffect, useRef, useState } from 'react';
import { AlertCircle, AlertTriangle, Check, FileJson, Loader2, Upload, X } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import {
  parseImportFile,
  validateFlowImport,
  validateAgentImport,
  type ImportItem,
  type RequiredAgent,
} from '@/lib/utils/importParser';
import { createFlow } from '@/services/api/flowService';
import { createAgent, getAgents } from '@/services/api/agentService';

interface ImportDialogProps {
  open: boolean;
  onClose: () => void;
  type: 'flow' | 'agent';
  onImportSuccess: () => void;
}

/**
 * 가져오기 모달.
 * 파일 업로드 -> 파싱 -> 미리보기 -> 생성 플로우를 제공한다.
 */
export default function ImportDialog({ open, onClose, type, onImportSuccess }: ImportDialogProps) {
  const fileInputRef = useRef<HTMLInputElement>(null);
  const agentFileInputRef = useRef<HTMLInputElement>(null);

  // 상태 관리
  const [items, setItems] = useState<ImportItem[]>([]);
  const [errors, setErrors] = useState<string[]>([]);
  const [isDragOver, setIsDragOver] = useState(false);
  const [isImporting, setIsImporting] = useState(false);
  const [importError, setImportError] = useState<string | null>(null);
  const [fileName, setFileName] = useState<string | null>(null);

  // 누락 에이전트 상태 (플로우 가져오기 전용)
  const [missingAgents, setMissingAgents] = useState<RequiredAgent[]>([]);
  const [selectedAgentIndices, setSelectedAgentIndices] = useState<Set<number>>(new Set());
  const [isAgentDragOver, setIsAgentDragOver] = useState(false);

  const typeLabel = type === 'flow' ? '플로우' : '에이전트';

  // 모달이 열릴 때 상태 초기화
  useEffect(() => {
    if (open) {
      setItems([]);
      setErrors([]);
      setIsDragOver(false);
      setIsImporting(false);
      setImportError(null);
      setFileName(null);
      setMissingAgents([]);
      setSelectedAgentIndices(new Set());
      setIsAgentDragOver(false);
    }
  }, [open]);

  // Escape 키로 닫기
  useEffect(() => {
    if (!open) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !isImporting) onClose();
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [open, onClose, isImporting]);

  /** 배경 클릭 시 모달 닫기 */
  const handleBackdropClick = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (e.target === e.currentTarget && !isImporting) onClose();
    },
    [onClose, isImporting],
  );

  /** 파일 처리 로직 */
  const processFile = async (file: File) => {
    setErrors([]);
    setItems([]);
    setImportError(null);
    setFileName(file.name);
    setMissingAgents([]);
    setSelectedAgentIndices(new Set());

    try {
      const data = await parseImportFile(file);
      const result = type === 'flow'
        ? validateFlowImport(data)
        : validateAgentImport(data);

      setErrors(result.errors);
      setItems(result.items);

      // 플로우 가져오기 시 누락 에이전트 확인
      if (type === 'flow' && result.items.length > 0) {
        const allRequired = result.items.flatMap(item => item.requiredAgents ?? []);
        if (allRequired.length > 0) {
          try {
            const { data: existingAgents } = await getAgents({ page: 1, size: 100 });
            const existingNames = new Set(existingAgents.map(a => a.name));

            // 이름 기준 중복 제거
            const uniqueMap = new Map<string, RequiredAgent>();
            for (const agent of allRequired) {
              if (!existingNames.has(agent.name) && !uniqueMap.has(agent.name)) {
                uniqueMap.set(agent.name, agent);
              }
            }

            const missing = Array.from(uniqueMap.values());
            setMissingAgents(missing);
            // type 정보가 있는 에이전트는 기본 선택
            setSelectedAgentIndices(
              new Set(missing.map((_, i) => i).filter(i => !!missing[i]?.type)),
            );
          } catch {
            // 에이전트 조회 실패 시 무시하고 계속 진행
          }
        }
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : '파일 파싱에 실패했습니다.';
      setErrors([message]);
    }
  };

  /** 파일 선택 핸들러 */
  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) processFile(file);
    // 같은 파일 재선택 허용
    e.target.value = '';
  };

  /** 드래그 앤 드롭 핸들러 */
  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragOver(true);
  };

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragOver(false);
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragOver(false);
    const file = e.dataTransfer.files[0];
    if (file) processFile(file);
  };

  /** 항목 이름 편집 핸들러 */
  const handleNameChange = (index: number, newName: string) => {
    setItems((prev) =>
      prev.map((item, i) => (i === index ? { ...item, editedName: newName } : item)),
    );
  };

  /** 누락 에이전트 선택 토글 */
  const toggleAgentSelection = (index: number) => {
    setSelectedAgentIndices((prev) => {
      const next = new Set(prev);
      if (next.has(index)) next.delete(index);
      else next.add(index);
      return next;
    });
  };

  /** 에이전트 파일로 누락 에이전트 보완 */
  const processAgentFiles = async (files: FileList) => {
    // 파싱 결과 수집
    const parsed: RequiredAgent[] = [];
    for (const file of Array.from(files)) {
      try {
        const data = await parseImportFile(file);
        const result = validateAgentImport(data);
        for (const item of result.items) {
          parsed.push({ name: item.name, type: item.type, config: item.config });
        }
      } catch {
        // 개별 파일 파싱 실패 시 무시
      }
    }
    if (parsed.length === 0) return;

    // immutable 상태 업데이트
    setMissingAgents(prev => {
      const updated = [...prev];
      const newIndices: number[] = [];

      for (const agent of parsed) {
        const idx = updated.findIndex(a => a.name === agent.name);
        if (idx >= 0) {
          updated[idx] = agent;
          newIndices.push(idx);
        } else {
          newIndices.push(updated.length);
          updated.push(agent);
        }
      }

      // 매칭된 에이전트 자동 선택
      setSelectedAgentIndices(prev => {
        const next = new Set(prev);
        for (const idx of newIndices) next.add(idx);
        return next;
      });

      return updated;
    });
  };

  /** 가져오기 실행 */
  const handleImport = async () => {
    setIsImporting(true);
    setImportError(null);

    try {
      // 1단계: 선택된 누락 에이전트를 먼저 생성한다
      if (type === 'flow' && missingAgents.length > 0) {
        for (const index of selectedAgentIndices) {
          const agent = missingAgents[index];
          if (agent?.type) {
            await createAgent({
              name: agent.name,
              type: agent.type,
              config: agent.config,
            });
          }
        }
      }

      // 2단계: 플로우/에이전트 생성
      for (const item of items) {
        const name = item.editedName.trim() || item.name;

        if (type === 'flow') {
          await createFlow({
            name,
            description: item.description,
            definition: (item.definition as Record<string, unknown>) ?? {},
          });
        } else {
          await createAgent({
            name,
            type: item.type ?? '',
            config: item.config,
          });
        }
      }

      onImportSuccess();
      onClose();
    } catch (err) {
      const message = err instanceof Error ? err.message : '가져오기 중 오류가 발생했습니다.';
      setImportError(message);
    } finally {
      setIsImporting(false);
    }
  };

  /** 유효한 항목이 있고 에러가 없으면 확인 가능 */
  const canImport = items.length > 0 && errors.length === 0 && !isImporting;

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={handleBackdropClick}
      role="dialog"
      aria-modal="true"
      aria-labelledby="import-dialog-title"
    >
      <div className="mx-4 flex w-full max-w-lg flex-col rounded-lg bg-(--color-bg-surface) shadow-xl">
        {/* 헤더 */}
        <div className="flex items-center justify-between border-b border-(--color-border-default) px-6 py-4">
          <h2
            id="import-dialog-title"
            className="text-lg font-semibold text-(--color-text-primary)"
          >
            {typeLabel} 가져오기
          </h2>
          <button
            type="button"
            onClick={onClose}
            disabled={isImporting}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 disabled:opacity-50 dark:hover:text-gray-300"
            aria-label="닫기"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* 본문 */}
        <div className="space-y-4 px-6 py-4">
          {/* 파일 드롭 영역 */}
          <div
            onDragOver={handleDragOver}
            onDragLeave={handleDragLeave}
            onDrop={handleDrop}
            onClick={() => fileInputRef.current?.click()}
            className={cn(
              'flex cursor-pointer flex-col items-center gap-2 rounded-lg border-2 border-dashed px-6 py-8 text-center transition-colors',
              isDragOver
                ? 'border-blue-400 bg-blue-50 dark:border-blue-500 dark:bg-blue-900/20'
                : 'border-(--color-border-strong) bg-gray-50 hover:border-gray-400 dark:bg-gray-700/50 dark:hover:border-gray-500',
            )}
          >
            <Upload className="h-8 w-8 text-(--color-text-muted)" />
            <div>
              <p className="text-sm font-medium text-(--color-text-secondary)">
                파일을 드래그하거나 클릭하여 선택
              </p>
              <p className="mt-1 text-xs text-(--color-text-muted)">
                .json, .yaml, .yml 파일 지원
              </p>
            </div>
            <input
              ref={fileInputRef}
              type="file"
              accept=".json,.yaml,.yml"
              onChange={handleFileChange}
              className="hidden"
            />
          </div>

          {/* 선택된 파일 이름 */}
          {fileName && (
            <div className="flex items-center gap-2 text-sm text-(--color-text-muted)">
              <FileJson className="h-4 w-4" />
              <span>{fileName}</span>
            </div>
          )}

          {/* 유효성 오류 표시 */}
          {errors.length > 0 && (
            <div className="rounded-md border border-red-200 bg-red-50 p-3 dark:border-red-800 dark:bg-red-900/20">
              <div className="flex items-start gap-2">
                <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-red-600 dark:text-red-400" />
                <div className="space-y-1">
                  {errors.map((err, i) => (
                    <p key={i} className="text-sm text-red-700 dark:text-red-300">
                      {err}
                    </p>
                  ))}
                </div>
              </div>
            </div>
          )}

          {/* 미리보기 목록 */}
          {items.length > 0 && (
            <div className="space-y-2">
              <p className="text-sm font-medium text-(--color-text-secondary)">
                미리보기 ({items.length}건)
              </p>
              <div className="max-h-48 space-y-2 overflow-y-auto">
                {items.map((item, index) => (
                  <div
                    key={index}
                    className="flex items-center gap-3 rounded-md border border-(--color-border-default) bg-(--color-bg-surface) p-3"
                  >
                    <Check className="h-4 w-4 shrink-0 text-green-500" />
                    <div className="min-w-0 flex-1">
                      <input
                        type="text"
                        value={item.editedName}
                        onChange={(e) => handleNameChange(index, e.target.value)}
                        className="w-full rounded border border-(--color-border-default) bg-transparent px-2 py-1 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:focus:border-blue-400"
                      />
                      {item.type && (
                        <p className="mt-1 text-xs text-(--color-text-muted)">
                          타입: {item.type}
                        </p>
                      )}
                      {item.description && (
                        <p className="mt-1 truncate text-xs text-(--color-text-muted)">
                          {item.description}
                        </p>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* 누락된 에이전트 목록 (플로우 가져오기 전용) */}
          {type === 'flow' && missingAgents.length > 0 && (
            <div className="space-y-2">
              <div className="flex items-center gap-1.5">
                <AlertTriangle className="h-4 w-4 text-amber-500" />
                <p className="text-sm font-medium text-amber-700 dark:text-amber-400">
                  누락된 에이전트 ({missingAgents.length}건)
                </p>
              </div>
              <p className="text-xs text-(--color-text-muted)">
                플로우에서 참조하지만 서버에 없는 에이전트입니다. 선택한 항목을 자동 생성합니다.
              </p>
              <div className="max-h-32 space-y-1.5 overflow-y-auto">
                {missingAgents.map((agent, index) => (
                  <label
                    key={agent.name}
                    className={cn(
                      'flex cursor-pointer items-center gap-2.5 rounded-md border p-2.5 transition-colors',
                      agent.type
                        ? 'border-amber-200 bg-amber-50 hover:bg-amber-100 dark:border-amber-800 dark:bg-amber-900/20 dark:hover:bg-amber-900/30'
                        : 'cursor-not-allowed border-(--color-border-default) bg-gray-50 dark:bg-gray-700/50',
                    )}
                  >
                    <input
                      type="checkbox"
                      checked={selectedAgentIndices.has(index)}
                      onChange={() => toggleAgentSelection(index)}
                      disabled={!agent.type}
                      className="rounded border-amber-300 text-amber-600 focus:ring-amber-500"
                    />
                    <span className="text-sm text-(--color-text-primary)">{agent.name}</span>
                    {agent.type ? (
                      <span className="text-xs text-(--color-text-muted)">
                        ({agent.type})
                      </span>
                    ) : (
                      <span className="text-xs text-red-500 dark:text-red-400">
                        (타입 정보 없음 - 수동 생성 필요)
                      </span>
                    )}
                  </label>
                ))}
              </div>
              {/* 에이전트 파일로 추가 */}
              <div
                onDragEnter={(e) => { e.preventDefault(); e.stopPropagation(); setIsAgentDragOver(true); }}
                onDragOver={(e) => { e.preventDefault(); e.stopPropagation(); }}
                onDragLeave={(e) => { e.preventDefault(); e.stopPropagation(); setIsAgentDragOver(false); }}
                onDrop={(e) => {
                  e.preventDefault();
                  e.stopPropagation();
                  setIsAgentDragOver(false);
                  if (e.dataTransfer.files.length > 0) processAgentFiles(e.dataTransfer.files);
                }}
                onClick={() => agentFileInputRef.current?.click()}
                className={cn(
                  'flex cursor-pointer items-center justify-center gap-1.5 rounded-md border-2 border-dashed px-3 py-3 text-xs transition-colors',
                  isAgentDragOver
                    ? 'border-amber-400 bg-amber-100 text-amber-700 dark:border-amber-500 dark:bg-amber-900/30 dark:text-amber-300'
                    : 'border-amber-300 text-amber-600 hover:bg-amber-50 dark:border-amber-700 dark:text-amber-400 dark:hover:bg-amber-900/20',
                )}
              >
                <Upload className="h-3.5 w-3.5" />
                에이전트 파일 드래그 또는 클릭하여 추가
                <input
                  ref={agentFileInputRef}
                  type="file"
                  accept=".json,.yaml,.yml"
                  multiple
                  onChange={(e) => {
                    if (e.target.files && e.target.files.length > 0) processAgentFiles(e.target.files);
                    e.target.value = '';
                  }}
                  className="hidden"
                />
              </div>
            </div>
          )}

          {/* 가져오기 오류 */}
          {importError && (
            <div className="rounded-md border border-red-200 bg-red-50 p-3 dark:border-red-800 dark:bg-red-900/20">
              <div className="flex items-start gap-2">
                <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-red-600 dark:text-red-400" />
                <p className="text-sm text-red-700 dark:text-red-300">{importError}</p>
              </div>
            </div>
          )}
        </div>

        {/* 푸터 */}
        <div className="flex justify-end gap-2 border-t border-(--color-border-default) px-6 py-4">
          <button
            type="button"
            onClick={onClose}
            disabled={isImporting}
            className="rounded-md border border-(--color-border-strong) px-4 py-2 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated) disabled:opacity-50"
          >
            취소
          </button>
          <button
            type="button"
            onClick={handleImport}
            disabled={!canImport}
            className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            {isImporting ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                가져오는 중...
              </>
            ) : (
              <>
                <Upload className="h-4 w-4" />
                가져오기 ({items.length}건)
              </>
            )}
          </button>
        </div>
      </div>
    </div>
  );
}
