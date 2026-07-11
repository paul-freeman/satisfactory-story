import type { CSSProperties } from 'react';
import type { Recipe, State } from '../types';
import ShortagePanel from './ShortagePanel';

interface NavRightProps {
  state: State;
  recipes: Recipe[];
  onSetRecipe: (name: string, active: boolean) => void;
}

const navColumnStyle: CSSProperties = {
  width: 200,
  padding: 4,
  background: '#808080',
  border: '2px solid black',
  display: 'flex',
  flexDirection: 'column',
  gap: 4,
  overflowY: 'auto',
};

const itemStyle: CSSProperties = {
  background: 'white',
  border: '1px solid black',
  padding: 4,
};

export default function NavRight({ state, recipes, onSetRecipe }: NavRightProps) {
  const alternates = recipes.filter((r) => r.name.startsWith('Alternate:')).sort((a, b) => a.name.localeCompare(b.name));
  const normal = recipes.filter((r) => !r.name.startsWith('Alternate:')).sort((a, b) => a.name.localeCompare(b.name));
  const inactiveResources = state.resources.filter((r) => !r.active).length;

  return (
    <div style={navColumnStyle}>
      <div style={itemStyle}>Resources: {state.resources.length}</div>
      <div style={itemStyle}>Factories: {state.factories.length}</div>
      <div style={itemStyle}>Sinks: {state.sinks.length}</div>
      <div style={itemStyle}>Transports: {state.transports.length}</div>
      <div style={itemStyle}>Inactive: {inactiveResources}</div>
      <ShortagePanel shortages={state.shortages} />
      {[...alternates, ...normal].map((recipe) => (
        <label key={recipe.id} style={{ ...itemStyle, display: 'flex', alignItems: 'center', gap: 4, fontSize: 11 }}>
          <input
            type="checkbox"
            checked={recipe.active}
            onChange={(e) => onSetRecipe(recipe.id, e.target.checked)}
          />
          {recipe.name}
        </label>
      ))}
    </div>
  );
}
