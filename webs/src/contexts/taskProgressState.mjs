export const isTerminalTask = (status) => ['completed', 'error', 'cancelled'].includes(status);

export function normalizeTask(task) {
  let result = task.result;
  if (typeof result === 'string') {
    try {
      result = JSON.parse(result);
    } catch {
      result = null;
    }
  }
  return {
    ...task,
    taskId: task.taskId ?? task.id,
    taskType: task.taskType ?? task.type,
    taskName: task.taskName ?? task.name,
    current: task.current ?? task.progress,
    startTime: task.startTime || (task.startedAt ? new Date(task.startedAt).getTime() : undefined),
    result
  };
}

// Bounded replay protection; retain recent terminal IDs even after dismissing cards.
export function acceptTask(terminalIds, task, limit = 1000) {
  if (!task.taskId || terminalIds.has(task.taskId)) return false;
  if (isTerminalTask(task.status)) {
    terminalIds.add(task.taskId);
    if (terminalIds.size > limit) terminalIds.delete(terminalIds.values().next().value);
  }
  return true;
}

// Recursive timeout rather than interval: slow requests cannot overlap.
export function startTaskReconciliation({ loadRunning, loadTask, activeIds, receive, schedule = setTimeout, unschedule = clearTimeout }) {
  let disposed = false;
  let timer;
  const reconcile = async () => {
    const ids = activeIds();
    try {
      const response = await loadRunning();
      if (!disposed) (response.data || []).forEach(receive);
    } catch {
      /* Retry after transient disconnects. */
    }
    for (const id of ids) {
      if (disposed) break;
      try {
        const response = await loadTask(id);
        if (!disposed && response.data) receive(response.data);
      } catch {
        /* A missing task must not stop reconciliation of others. */
      }
    }
    if (!disposed) timer = schedule(reconcile, 5000);
  };
  void reconcile();
  return () => {
    disposed = true;
    unschedule(timer);
  };
}
