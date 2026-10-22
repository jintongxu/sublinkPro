import PropTypes from 'prop-types';
import { createContext, useContext, useState, useCallback, useMemo, useEffect, useRef } from 'react';
import { getTask, getRunningTasks, stopTask as stopTaskApi } from 'api/tasks';
import { useAuth } from './AuthContext';
import { normalizeTask, acceptTask, startTaskReconciliation } from './taskProgressState.mjs';

// ==============================|| TASK PROGRESS CONTEXT ||============================== //

const TaskProgressContext = createContext(null);

// Auto-cleanup completed tasks after this duration (ms)
const COMPLETED_TASK_CLEANUP_DELAY = 3000;

// ==============================|| TASK PROGRESS PROVIDER ||============================== //

export function TaskProgressProvider({ children }) {
  const { isAuthenticated } = useAuth();
  const tasksRef = useRef(new Map());
  const cleanupTimers = useRef(new Set());
  // Map of taskId -> taskData
  const [tasks, setTasks] = useState(new Map());

  // Track tasks that are being stopped
  const [stoppingTasks, setStoppingTasks] = useState(new Set());

  // Callbacks to invoke when any task completes
  const onCompleteCallbacksRef = useRef(new Set());
  const terminalTasksRef = useRef(new Set());

  // Update or add a task
  const updateTask = useCallback((taskId, taskData) => {
    const incomingTerminal = ['completed', 'error', 'cancelled'].includes(taskData.status);
    if (terminalTasksRef.current.has(taskId) && !incomingTerminal) return;
    const next = new Map(tasksRef.current);
    const existing = next.get(taskId) || {};
    next.set(taskId, {
      ...existing,
      ...taskData,
      taskId,
      startTime: taskData.startTime || existing.startTime,
      lastUpdate: Date.now()
    });
    tasksRef.current = next;
    setTasks(next);
  }, []);

  // Remove a task
  const removeTask = useCallback((taskId) => {
    const next = new Map(tasksRef.current);
    next.delete(taskId);
    tasksRef.current = next;
    setTasks(next);
    // Also remove from stopping set
    setStoppingTasks((prev) => {
      const newSet = new Set(prev);
      newSet.delete(taskId);
      return newSet;
    });
  }, []);

  // Clear all tasks
  const clearAll = useCallback(() => {
    tasksRef.current = new Map();
    terminalTasksRef.current.clear();
    cleanupTimers.current.forEach(clearTimeout);
    cleanupTimers.current.clear();
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
      const normalized = normalizeTask(data);
      const { taskId, taskType, taskName, status, current, total, currentItem, result, message, startTime, traffic } = normalized;

      if (!taskId) return;

      const isTerminal = status === 'completed' || status === 'error' || status === 'cancelled';
      if (!acceptTask(terminalTasksRef.current, normalized)) return;

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
        const timer = setTimeout(() => {
          cleanupTimers.current.delete(timer);
          removeTask(taskId);
        }, COMPLETED_TASK_CLEANUP_DELAY);
        cleanupTimers.current.add(timer);
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
      if (isAuthenticated) handleProgressEvent(event.detail);
    };
    window.addEventListener('task_progress', handleWindowEvent);
    return () => {
      window.removeEventListener('task_progress', handleWindowEvent);
    };
  }, [handleProgressEvent, isAuthenticated]);

  // Convert tasks Map to array for easier rendering
  const taskList = useMemo(() => Array.from(tasks.values()), [tasks]);

  // Check if there are any active tasks
  const hasActiveTasks = useMemo(() => taskList.length > 0, [taskList]);
  // Authentication owns hydration. One loop survives task changes and reconciles
  // missed SSE events, reconnects and tasks started on other pages/clients.
  useEffect(() => {
    if (!isAuthenticated) {
      clearAll();
      return undefined;
    }
    const timers = cleanupTimers.current;
    const stop = startTaskReconciliation({
      loadRunning: getRunningTasks,
      loadTask: getTask,
      activeIds: () =>
        Array.from(tasksRef.current.values())
          .filter((task) => !['completed', 'error', 'cancelled'].includes(task.status))
          .map((task) => task.taskId),
      receive: handleProgressEvent
    });
    return () => {
      stop();
      timers.forEach(clearTimeout);
      timers.clear();
    };
  }, [isAuthenticated, handleProgressEvent, clearAll]);

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
