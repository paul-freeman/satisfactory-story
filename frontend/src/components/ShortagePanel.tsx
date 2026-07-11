import { useState, type CSSProperties } from 'react';
import type { Shortage } from '../types';

interface ShortagePanelProps {
  shortages: Shortage[];
}

const itemStyle: CSSProperties = {
  background: 'white',
  border: '1px solid black',
  padding: 4,
};

export default function ShortagePanel({ shortages }: ShortagePanelProps) {
  const [open, setOpen] = useState(false);

  return (
    <div>
      <button style={{ ...itemStyle, width: '100%', cursor: 'pointer' }} onClick={() => setOpen(!open)}>
        Shortages {open ? '▲' : '▼'}
      </button>
      {open && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 4, marginTop: 4 }}>
          {shortages.length === 0 && <div style={{ ...itemStyle, fontSize: 11 }}>None recorded</div>}
          {shortages.map((s) => (
            <div key={s.product} style={{ ...itemStyle, fontSize: 11, display: 'flex', justifyContent: 'space-between' }}>
              <span>{s.product}</span>
              <span>{s.amount.toFixed(1)}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
