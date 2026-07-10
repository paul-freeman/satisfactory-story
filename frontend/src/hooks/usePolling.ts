import { useCallback, useEffect, useRef, useState } from 'react';
import { getState } from '../api';
import type { State } from '../types';

// Matches the original Elm app's sleepAndPoll interval exactly.
const POLL_INTERVAL_MS = 50;

export function usePolling() {
  const [state, setState] = useState<State | null>(null);
  const [error, setError] = useState<string | null>(null);
  const timerRef = useRef<number | null>(null);

  const applyState = useCallback((newState: State) => {
    setState(newState);
    setError(null);
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    if (newState.running) {
      timerRef.current = window.setTimeout(() => {
        getState().then(applyState).catch(handleError);
      }, POLL_INTERVAL_MS);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleError = useCallback((err: unknown) => {
    setError(err instanceof Error ? err.message : String(err));
  }, []);

  useEffect(() => {
    getState().then(applyState).catch(handleError);
    return () => {
      if (timerRef.current !== null) {
        window.clearTimeout(timerRef.current);
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return { state, error, applyState, setError: handleError };
}
