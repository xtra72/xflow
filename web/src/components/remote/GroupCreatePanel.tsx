// GroupCreatePanel 은 그룹 관리 화면 우측의 "신규 그룹 생성" 패널이다.
//
// 파생 그룹 모델(빈 그룹 미영속)이므로, 새 그룹명 + 멤버 노드 다중선택 → 일괄 배정으로
// 그룹을 생성한다. 생성 성공 시 onCreated(newName) 으로 상위가 새 그룹을 선택하게 한다.

import { useState } from 'react';
import { FolderPlus, Loader2 } from 'lucide-react';

import { useSetNodeGroup } from '@/hooks/useRemote';
import { useTranslation } from '@/lib/i18n';
import type { ManagedNode } from '@/types/remote';
import { useUIStore } from '@/stores/uiStore';

interface GroupCreatePanelProps {
  nodes: ManagedNode[];
  onCreated: (groupName: string) => void;
}

export function GroupCreatePanel({
  nodes,
  onCreated,
}: GroupCreatePanelProps): React.JSX.Element {
  const { t } = useTranslation();
  const addNotification = useUIStore((s) => s.addNotification);
  const setNodeGroup = useSetNodeGroup();

  const [name, setName] = useState('');
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [creating, setCreating] = useState(false);

  const toggle = (id: string): void => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const handleCreate = async (): Promise<void> => {
    const groupName = name.trim();
    if (groupName === '' || selected.size === 0) return;
    setCreating(true);
    try {
      await Promise.all(
        Array.from(selected).map((instanceID) =>
          setNodeGroup.mutateAsync({ instanceID, groupName }),
        ),
      );
      addNotification({ type: 'success', message: t('remote.group.toast.created') });
      setName('');
      setSelected(new Set());
      onCreated(groupName);
    } catch {
      addNotification({ type: 'error', message: t('remote.group.toast.opFailed') });
    } finally {
      setCreating(false);
    }
  };

  return (
    <section
      className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-4"
      data-testid="group-create"
    >
      <div className="mb-3 flex items-center gap-2">
        <FolderPlus className="h-4 w-4 text-(--color-text-muted)" aria-hidden="true" />
        <h2 className="text-sm font-semibold text-(--color-text-primary)">
          {t('remote.group.manage.create')}
        </h2>
      </div>

      <div className="mb-3 flex flex-wrap items-center gap-2">
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder={t('remote.group.manage.createPlaceholder')}
          data-testid="create-name"
          className="w-48 rounded border border-(--color-border-strong) bg-(--color-bg-primary) px-2 py-1 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none"
        />
        <button
          type="button"
          onClick={() => void handleCreate()}
          disabled={creating || name.trim() === '' || selected.size === 0}
          data-testid="create-submit"
          className="inline-flex items-center gap-1 rounded bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
        >
          {creating && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
          {t('remote.group.manage.createButton')} ({selected.size})
        </button>
      </div>

      <p className="mb-1 text-xs font-medium text-(--color-text-muted)">
        {t('remote.group.manage.selectNodes')}
      </p>
      {nodes.length === 0 ? (
        <p className="text-xs text-(--color-text-muted)" data-testid="create-nonodes">
          {t('remote.group.manage.noNodes')}
        </p>
      ) : (
        <ul className="max-h-72 overflow-y-auto rounded border border-(--color-border-default)">
          {nodes.map((n) => (
            <li key={n.instance_id}>
              <label className="flex cursor-pointer items-center gap-2 px-2 py-1 text-xs hover:bg-(--color-bg-elevated)">
                <input
                  type="checkbox"
                  checked={selected.has(n.instance_id)}
                  onChange={() => toggle(n.instance_id)}
                  data-testid={`create-node-${n.instance_id}`}
                  className="h-3.5 w-3.5 rounded border-(--color-border-strong) text-blue-600"
                />
                <span className="flex-1 truncate text-(--color-text-primary)">
                  {n.hostname || n.instance_id}
                </span>
                <span className="shrink-0 text-(--color-text-muted)">
                  {n.group_name || t('remote.group.manage.ungrouped')}
                </span>
              </label>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
