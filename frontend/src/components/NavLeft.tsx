import type { CSSProperties } from 'react';

interface NavLeftProps {
  tick: number;
  running: boolean;
  onRun: () => void;
  onStop: () => void;
  onTick: () => void;
  onReset: () => void;
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

const buttonStyle: CSSProperties = {
  ...itemStyle,
  width: '100%',
  background: '#c0c0c0',
  cursor: 'pointer',
};

export default function NavLeft({ tick, running, onRun, onStop, onTick, onReset }: NavLeftProps) {
  return (
    <div style={navColumnStyle}>
      <div style={itemStyle}>Tick: {tick}</div>
      {running ? (
        <button style={buttonStyle} onClick={onStop}>
          Stop
        </button>
      ) : (
        <>
          <button style={buttonStyle} onClick={onRun}>
            Run
          </button>
          <button style={buttonStyle} onClick={onTick}>
            Tick
          </button>
          <button style={buttonStyle} onClick={onReset}>
            Reset
          </button>
        </>
      )}
    </div>
  );
}
