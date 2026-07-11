import { scaleDiverging } from 'd3-scale';
import { interpolateRdYlGn } from 'd3-scale-chromatic';

// Builds a factory cash -> color function, domain-fit to the given
// population's min/max/zero each time it's called, so the gradient stays
// meaningful as the economy's scale changes over a run rather than
// saturating to a single color against a fixed threshold. Diverges at 0
// (the insolvency boundary): red below, green above.
export function cashColorScale(cashValues: number[]): (cash: number) => string {
  if (cashValues.length === 0) {
    return () => 'grey';
  }
  const min = Math.min(...cashValues, -1);
  const max = Math.max(...cashValues, 1);
  const scale = scaleDiverging(interpolateRdYlGn).domain([min, 0, max]);
  return (cash: number) => scale(cash);
}
