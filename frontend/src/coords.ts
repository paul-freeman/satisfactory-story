import type { Bounds } from './types';

// The backend's y grows opposite to SVG's -- every marker position needs
// this flip against the world bounds (not the pixel viewport, since
// markers are drawn in the same world-coordinate <g> that d3-zoom
// transforms as a whole). Matches the original Elm app's exact formula.
export function toSvgY(bounds: Bounds, y: number): number {
  return bounds.ymin + bounds.ymax - y;
}
