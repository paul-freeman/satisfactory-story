import { useEffect, useState } from 'react';
import { usePolling } from './hooks/usePolling';
import MapView from './components/MapView';
import NavLeft from './components/NavLeft';
import NavRight from './components/NavRight';
import * as api from './api';
import type { Recipe } from './types';

function App() {
  const { state, applyState } = usePolling();
  const [recipes, setRecipes] = useState<Recipe[]>([]);

  useEffect(() => {
    api.getRecipes().then(setRecipes);
  }, []);

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
      <NavRight
        state={state}
        recipes={recipes}
        onSetRecipe={(name, active) => api.setRecipe(name, active).then(setRecipes)}
      />
    </div>
  );
}

export default App
