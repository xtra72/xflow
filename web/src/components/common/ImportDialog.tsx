// 가져오기 대화 상자.
// 파일 선택(드래그 앤 드롭 포함)으로 JSON/YAML 파일을 파싱하고,
// 미리보기 후 플로우 또는 에이전트를 일괄 생성한다.

import { useCallback, useEffect, useRef, useState } from 'react';
import { AlertCircle, Check, FileJson, Loader2, Upload, X } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import {
  parseImportFile,
  validateFlowImport,
  validateAgentImport,
  type ImportItem,
} from '@/lib/utils/importParser';
import { createFlow } from '@/services/api/flowService';
import { createAgent } from '@/services/api/agentService';

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

  // 상태 관리
  const [items, setItems] = useState<ImportItem[]>([]);
  const [errors, setErrors] = useState<string[]>([]);
  const [isDragOver, setIsDragOver] = useState(false);
  const [isImporting, setIsImporting] = useState(false);
  const [importError, setImportError] = useState<string | null>(null);
  const [fileName, setFileName] = useState<string | null>(null);

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

    try {
      const data = await parseImportFile(file);
      const result = type === 'flow'
        ? validateFlowImport(data)
        : validateAgentImport(data);

      setErrors(result.errors);
      setItems(result.items);
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

  /** 가져오기 실행 */
  const handleImport = async () => {
    setIsImporting(true);
    setImportError(null);

    try {
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
      <div className="mx-4 flex w-full max-w-lg flex-col rounded-lg bg-white shadow-xl dark:bg-gray-800">
        {/* 헤더 */}
        <div className="flex items-center justify-between border-b border-gray-200 px-6 py-4 dark:border-gray-700">
          <h2
            id="import-dialog-title"
            className="text-lg font-semibold text-gray-900 dark:text-white"
          >
            {typeLabel} 가져오기
          </h2>
          <button
            type="button"
            onClick={onClose}
            disabled={isImporting}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 disabled:opacity-50 dark:hover:bg-gray-700 dark:hover:text-gray-300"
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
                : 'border-gray-300 bg-gray-50 hover:border-gray-400 dark:border-gray-600 dark:bg-gray-700/50 dark:hover:border-gray-500',
            )}
          >
            <Upload className="h-8 w-8 text-gray-400 dark:text-gray-500" />
            <div>
              <p className="text-sm font-medium text-gray-700 dark:text-gray-300">
                파일을 드래그하거나 클릭하여 선택
              </p>
              <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
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
            <div className="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-400">
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
              <p className="text-sm font-medium text-gray-700 dark:text-gray-300">
                미리보기 ({items.length}건)
              </p>
              <div className="max-h-48 space-y-2 overflow-y-auto">
                {items.map((item, index) => (
                  <div
                    key={index}
                    className="flex items-center gap-3 rounded-md border border-gray-200 bg-white p-3 dark:border-gray-600 dark:bg-gray-700"
                  >
                    <Check className="h-4 w-4 shrink-0 text-green-500" />
                    <div className="min-w-0 flex-1">
                      <input
                        type="text"
                        value={item.editedName}
                        onChange={(e) => handleNameChange(index, e.target.value)}
                        className="w-full rounded border border-gray-200 bg-transparent px-2 py-1 text-sm text-gray-900 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-600 dark:text-white dark:focus:border-blue-400"
                      />
                      {item.type && (
                        <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                          타입: {item.type}
                        </p>
                      )}
                      {item.description && (
                        <p className="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">
                          {item.description}
                        </p>
                      )}
                    </div>
                  </div>
                ))}
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
        <div className="flex justify-end gap-2 border-t border-gray-200 px-6 py-4 dark:border-gray-700">
          <button
            type="button"
            onClick={onClose}
            disabled={isImporting}
            className="rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 disabled:opacity-50 dark:border-gray-600 dark:text-gray-300 dark:hover:bg-gray-700"
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
