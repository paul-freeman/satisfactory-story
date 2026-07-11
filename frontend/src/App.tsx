import { usePolling } from './hooks/usePolling';
import MapView from './components/MapView';
import NavLeft from './components/NavLeft';
import * as api from './api';

function App() {
  const { state, applyState } = usePolling();

  if (!state) {
    return <div>Loading...</div>;
  }

  return (
    <div style={{ width: '100vw', height: '100vh', display: 'flex' }}>
      <NavLeft
        tick={state.tick}
        running={state.running}
        onRun={() => api.run().then(() => api.getState().then(applyState))}
        onStop={() => api.stop().then(applyState)}
        onTick={() => api.tick().then(applyState)}
        onReset={() => api.reset().then(applyState)}
      />
      <div style={{ flex: 1 }}>
        <MapView
          bounds={state.bounds}
          resources={state.resources}
          sinks={state.sinks}
          transports={state.transports}
          factories={state.factories}
        />
      </div>
    </div>
  );
}

export default App
