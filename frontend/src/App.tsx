import { usePolling } from './hooks/usePolling';
import MapView from './components/MapView';

function App() {
  const { state } = usePolling();

  if (!state) {
    return <div>Loading...</div>;
  }

  return (
    <div style={{ width: '100vw', height: '100vh' }}>
      <MapView
        bounds={state.bounds}
        resources={state.resources}
        sinks={state.sinks}
        transports={state.transports}
      />
    </div>
  );
}

export default App
