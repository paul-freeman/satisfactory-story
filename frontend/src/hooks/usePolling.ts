import { useCallback, useEffect, useRef, useState } from 'react';
import { getState } from '../api';
import type { State } from '../types';

// Matches the original Elm app's sleepAndPoll interval exactly.
const POLL_INTERVAL_MS = 50;

export function usePolling() {
  const [state, setState] = useState<State | null>(null);
  const [error, setError] = useState<string | null>(null);
  const timerRef = useRef<number | null>(null);
  // Tracks whether this component has unmounted. If a fetch resolves after unmount,
  // we check this flag to avoid updating state or scheduling timers on a dead component.
  const cancelledRef = useRef(false);

  const applyState = useCallback((newState: State) => {
    if (cancelledRef.current) {
      return;
    }
    setState(newState);
    setError(null);
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    if (newState.running) {
      timerRef.current = window.setTimeout(() => {
        if (cancelledRef.current) {
          return;
        }
        getState().then(applyState).catch(handleError);
      }, POLL_INTERVAL_MS);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleError = useCallback((err: unknown) => {
    if (cancelledRef.current) {
      return;
    }
    setError(err instanceof Error ? err.message : String(err));
  }, []);

  useEffect(() => {
    cancelledRef.current = false;
    getState().then(applyState).catch(handleError);
    return () => {
      cancelledRef.current = true;
      if (timerRef.current !== null) {
        window.clearTimeout(timerRef.current);
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return { state, error, applyState, setError: handleError };
}
