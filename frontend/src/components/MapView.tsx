import { useEffect, useRef } from 'react';
import { select } from 'd3-selection';
import { zoom, zoomIdentity, type D3ZoomEvent } from 'd3-zoom';
import type { Bounds, Resource, Sink, Transport } from '../types';
import { toSvgY } from '../coords';

// World-space tile layout, matching the original CustomSvg.elm constants
// exactly: a 5x5 grid of 32000-unit tiles with its corner at (0, -160300).
const MAP_CORNER_X = 0;
const MAP_CORNER_Y = -160300;
const MAP_TILE_SIZE = 32000;
const MAP_GRID_SIZE = 5;

interface MapViewProps {
  bounds: Bounds;
  resources: Resource[];
  sinks: Sink[];
  transports: Transport[];
}

export default function MapView({ bounds, resources, sinks, transports }: MapViewProps) {
  const svgRef = useRef<SVGSVGElement | null>(null);
  const zoomGroupRef = useRef<SVGGElement | null>(null);

  useEffect(() => {
    if (!svgRef.current || !zoomGroupRef.current) {
      return;
    }
    const svgSelection = select(svgRef.current);
    const zoomGroupSelection = select(zoomGroupRef.current);
    const zoomBehavior = zoom<SVGSVGElement, unknown>()
      .scaleExtent([0.05, 20])
      .on('zoom', (event: D3ZoomEvent<SVGSVGElement, unknown>) => {
        zoomGroupSelection.attr('transform', event.transform.toString());
      });
    svgSelection.call(zoomBehavior);
    // Center roughly on the world bounds on first mount.
    const initialScale = 0.3;
    svgSelection.call(
      zoomBehavior.transform,
      zoomIdentity
        .translate(400, 300)
        .scale(initialScale)
        .translate(-(bounds.xmin + bounds.xmax) / 2, -(bounds.ymin + bounds.ymax) / 2),
    );
    return () => {
      svgSelection.on('.zoom', null);
    };
    // Only set up zoom once on mount -- bounds don't change after init.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const tiles = [];
  for (let x = 0; x < MAP_GRID_SIZE; x++) {
    for (let y = 0; y < MAP_GRID_SIZE; y++) {
      tiles.push(
        <image
          key={`${x}_${y}`}
          href={`/${x}_${y}.png`}
          x={MAP_CORNER_X + x * MAP_TILE_SIZE}
          y={MAP_CORNER_Y + y * MAP_TILE_SIZE}
          width={MAP_TILE_SIZE}
          height={MAP_TILE_SIZE}
        />,
      );
    }
  }

  return (
    <svg ref={svgRef} width="100%" height="100%" style={{ display: 'block', background: '#222' }}>
      <g ref={zoomGroupRef}>
        <g>{tiles}</g>
        <g>
          {resources
            .filter((r) => !r.active)
            .map((r, i) => (
              <circle
                key={`inactive-${i}`}
                cx={r.location.x}
                cy={toSvgY(bounds, r.location.y)}
                r={180}
                fill="lightgrey"
              />
            ))}
        </g>
        <g>
          {resources
            .filter((r) => r.active)
            .map((r, i) => (
              <circle
                key={`active-${i}`}
                cx={r.location.x}
                cy={toSvgY(bounds, r.location.y)}
                r={180}
                fill={r.profitability > 0 ? 'blue' : 'purple'}
              />
            ))}
        </g>
        <g>
          {transports.map((t, i) => (
            <line
              key={i}
              x1={t.origin.x}
              y1={toSvgY(bounds, t.origin.y)}
              x2={t.destination.x}
              y2={toSvgY(bounds, t.destination.y)}
              stroke="black"
              strokeWidth={200}
            />
          ))}
        </g>
        <g>
          {sinks.map((s, i) => (
            <text
              key={i}
              x={s.location.x}
              y={toSvgY(bounds, s.location.y) + 1300}
              textAnchor="middle"
              dominantBaseline="middle"
              fontSize={800}
            >
              {s.label}
            </text>
          ))}
        </g>
        <g>
          {sinks.map((s, i) => (
            <circle key={i} cx={s.location.x} cy={toSvgY(bounds, s.location.y)} r={180} fill="orange" />
          ))}
        </g>
      </g>
    </svg>
  );
}
