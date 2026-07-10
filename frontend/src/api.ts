import type { Recipe, State } from './types';

async function getJSON<T>(url: string): Promise<T> {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`request to ${url} failed: ${response.status} ${response.statusText}`);
  }
  return response.json() as Promise<T>;
}

export function getState(): Promise<State> {
  return getJSON<State>('/state');
}

export function tick(): Promise<State> {
  return getJSON<State>('/tick');
}

export async function run(): Promise<void> {
  const response = await fetch('/run');
  if (!response.ok) {
    throw new Error(`request to /run failed: ${response.status} ${response.statusText}`);
  }
}

export function stop(): Promise<State> {
  return getJSON<State>('/stop');
}

export function reset(): Promise<State> {
  return getJSON<State>('/reset');
}

export function getRecipes(): Promise<Recipe[]> {
  return getJSON<Recipe[]>('/recipes');
}

export function setRecipe(name: string, active: boolean): Promise<Recipe[]> {
  return getJSON<Recipe[]>(`/recipe/${encodeURIComponent(name)}/${active ? '1' : '0'}`);
}
