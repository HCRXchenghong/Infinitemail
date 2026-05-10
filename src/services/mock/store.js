import { createSeedState } from "./seed";

const STORAGE_KEY = "yuexiang-post-office.mock.v1";

function clone(value) {
  return JSON.parse(JSON.stringify(value));
}

export function readMockState() {
  if (typeof window === "undefined") {
    return createSeedState();
  }

  const raw = window.localStorage.getItem(STORAGE_KEY);
  if (!raw) {
    const seed = createSeedState();
    writeMockState(seed);
    return seed;
  }

  try {
    return JSON.parse(raw);
  } catch {
    const seed = createSeedState();
    writeMockState(seed);
    return seed;
  }
}

export function writeMockState(state) {
  if (typeof window !== "undefined") {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
  }
}

export function updateMockState(updater) {
  const current = readMockState();
  const draft = clone(current);
  const next = updater(draft) || draft;
  writeMockState(next);
  return next;
}

export function resetMockState() {
  const seed = createSeedState();
  writeMockState(seed);
  return seed;
}
