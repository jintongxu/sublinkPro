import PropTypes from 'prop-types';
import { createContext, useContext, useState, useCallback, useMemo, useEffect, useRef } from 'react';
import { getTask, stopTask as stopTaskApi } from 'api/tasks';

// ==============================|| TASK PROGRESS CONTEXT ||============================== //

const TaskProgressContext = createContext(null);

// Auto-cleanup completed tasks after this duration (ms)
const COMPLETED_TASK_CLEANUP_DELAY = 3000;

// ==============================|| TASK PROGRESS PROVIDER ||============================== //

export function TaskProgressProvider({ children }) {
  // Map of taskId -> taskData
  const [tasks, setTasks] = useState(new Map());

  // Track tasks that are being stopped
  const [stoppingTasks, setStoppingTasks] = useState(new Set());

  // Callbacks to invoke when any task completes
  const onCompleteCallbacksRef = useRef(new Set());
  const terminalTasksRef = useRef(new Set());

  // Update or add a task
  const updateTask = useCallback((taskId, taskData) => {
    setTasks((prev) => {
      const incomingTerminal = ['completed', 'error', 'cancelled'].includes(taskData.status);
      if (terminalTasksRef.current.has(taskId) && !incomingTerminal) return prev;
      const newTasks = new Map(prev);
      const existing = newTasks.get(taskId) || {};
      newTasks.set(taskId, {
        ...existing,
        ...taskData,
        // Preserve existing startTime unless new one is provided
        startTime: taskData.startTime || existing.startTime,
        lastUpdate: Date.now()
      });
      return newTasks;
    });
  }, []);

  // Remove a task
  const removeTask = useCallback((taskId) => {
    setTasks((prev) => {
      const newTasks = new Map(prev);
      newTasks.delete(taskId);
      return newTasks;
    });
    // Also remove from stopping set
    setStoppingTasks((prev) => {
      const newSet = new Set(prev);
      newSet.delete(taskId);
      return newSet;
    });
  }, []);

  // Clear all tasks
  const clearAll = useCallback(() => {
    setTasks(new Map());
    setStoppingTasks(new Set());
  }, []);

  // Stop a task
  const stopTask = useCallback(
    async (taskId) => {
      // Mark as stopping
      setStoppingTasks((prev) => new Set(prev).add(taskId));

      try {
        await stopTaskApi(taskId);
        // The actual status update will come via SSE
        // Update local state optimistically
        updateTask(taskId, { status: 'cancelling' });
      } catch (error) {
        console.error('Failed to stop task:', error);
        // Remove from stopping set on error
        setStoppingTasks((prev) => {
          const newSet = new Set(prev);
          newSet.delete(taskId);
          return newSet;
        });
        throw error;
      }
    },
    [updateTask]
  );

  // Check if a task is being stopped
  const isTaskStopping = useCallback(
    (taskId) => {
      return stoppingTasks.has(taskId);
    },
    [stoppingTasks]
  );

  // Handle incoming progress event from SSE
  const handleProgressEvent = useCallback(
    (data) => {
      const { taskId, taskType, taskName, status, current, total, currentItem, result, message, startTime, traffic } = data;

      if (!taskId) return;

      const isTerminal = status === 'completed' || status === 'error' || status === 'cancelled';
      if (terminalTasksRef.current.has(taskId)) return;
      if (isTerminal) terminalTasksRef.current.add(taskId);

      if (isTerminal) {
        // Update task with completed/cancelled status, then schedule removal
        updateTask(taskId, {
          taskId,
          taskType,
          taskName,
          status,
          current,
          total,
          currentItem,
          result,
          message,
          traffic
        });

        // Notify all registered callbacks that a task has completed
        onCompleteCallbacksRef.current.forEach((callback) => {
          try {
            callback({ taskId, taskType, taskName, status, result });
          } catch (e) {
            console.error('TaskProgressContext onComplete callback error:', e);
          }
        });

        // Auto-remove completed tasks after delay
        setTimeout(() => {
          removeTask(taskId);
        }, COMPLETED_TASK_CLEANUP_DELAY);
      } else {
        // Update task progress
        updateTask(taskId, {
          taskId,
          taskType,
          taskName,
          status,
          current,
          total,
          currentItem,
          result,
          message,
          // Always pass startTime from event (every progress event now includes it)
          startTime,
          traffic
        });
      }
    },
    [updateTask, removeTask]
  );

  // Listen for task_progress CustomEvent from window (dispatched by AuthContext)
  useEffect(() => {
    const handleWindowEvent = (event) => {
      handleProgressEvent(event.detail);
    };
    window.addEventListener('task_progress', handleWindowEvent);
    return () => {
      window.removeEventListener('task_progress', handleWindowEvent);
    };
  }, [handleProgressEvent]);

  // Convert tasks Map to array for easier rendering
  const taskList = useMemo(() => Array.from(tasks.values()), [tasks]);

  // Check if there are any active tasks
  const hasActiveTasks = useMemo(() => taskList.length > 0, [taskList]);
  const activeTaskIds = useMemo(
    () => taskList.filter((task) => !['completed', 'error', 'cancelled'].includes(task.status)).map((task) => task.taskId).sort(),
    [taskList]
  );
  const activeTaskKey = activeTaskIds.join(',');

  // SSE is primary; periodic reads replay terminal state across navigation or SSE reconnects.
  useEffect(() => {
    if (!activeTaskKey) return undefined;
    let disposed = false;
    const taskIds = activeTaskKey.split(',').filter(Boolean);
    const reconcile = async () => {
      await Promise.all(
        taskIds.map(async (taskId) => {
          try {
            const response = await getTask(taskId);
            const task = response.data;
            if (disposed || !task || !['completed', 'error', 'cancelled'].includes(task.status)) return;
            handleProgressEvent({
              taskId: task.id,
              taskType: task.type,
              taskName: task.name,
              status: task.status,
              current: task.progress,
              total: task.total,
              currentItem: task.currentItem,
              result: task.result ? JSON.parse(task.result) : null,
              message: task.message,
              startTime: task.startedAt ? new Date(task.startedAt).getTime() : Date.now()
            });
          } catch (error) {
            if (!disposed) console.error('Failed to reconcile task:', error);
          }
        })
      );
    };
    const timer = window.setInterval(reconcile, 5000);
    reconcile();
    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
  }, [activeTaskKey, handleProgressEvent]);

  // Register a callback to be invoked when any task completes
  const registerOnComplete = useCallback((callback) => {
    onCompleteCallbacksRef.current.add(callback);
  }, []);

  // Unregister a previously registered callback
  const unregisterOnComplete = useCallback((callback) => {
    onCompleteCallbacksRef.current.delete(callback);
  }, []);

  const value = useMemo(
    () => ({
      tasks,
      taskList,
      hasActiveTasks,
      updateTask,
      removeTask,
      clearAll,
      handleProgressEvent,
      registerOnComplete,
      unregisterOnComplete,
      stopTask,
      isTaskStopping,
      stoppingTasks
    }),
    [
      tasks,
      taskList,
      hasActiveTasks,
      updateTask,
      removeTask,
      clearAll,
      handleProgressEvent,
      registerOnComplete,
      unregisterOnComplete,
      stopTask,
      isTaskStopping,
      stoppingTasks
    ]
  );

  return <TaskProgressContext.Provider value={value}>{children}</TaskProgressContext.Provider>;
}

TaskProgressProvider.propTypes = { children: PropTypes.node };

// ==============================|| useTaskProgress Hook ||============================== //

export function useTaskProgress() {
  const context = useContext(TaskProgressContext);
  if (!context) {
    throw new Error('useTaskProgress 必须在 TaskProgressProvider 内部使用');
  }
  return context;
}

export default TaskProgressContext;
