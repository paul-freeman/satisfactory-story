export interface Location {
  x: number;
  y: number;
}

export interface Bounds {
  xmin: number;
  xmax: number;
  ymin: number;
  ymax: number;
}

export interface Resource {
  location: Location;
  recipe: string;
  product: string;
  profitability: number;
  active: boolean;
}

export interface Factory {
  location: Location;
  recipe: string;
  products: string[];
  profitability: number;
  cash: number;
}

export interface Sink {
  location: Location;
  label: string;
}

export interface Transport {
  origin: Location;
  destination: Location;
  rate: number;
}

export interface Shortage {
  product: string;
  amount: number;
}

export interface State {
  resources: Resource[];
  factories: Factory[];
  sinks: Sink[];
  transports: Transport[];
  shortages: Shortage[];
  tick: number;
  running: boolean;
  bounds: Bounds;
}

export interface Product {
  name: string;
  rate: number;
}

export interface Recipe {
  name: string;
  inputs: Product[];
  outputs: Product[];
  active: boolean;
}
