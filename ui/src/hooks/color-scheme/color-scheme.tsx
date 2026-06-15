import {
  createContext,
  type PropsWithChildren,
  useContext,
  useEffect,
  useState,
  useSyncExternalStore,
} from 'react';

export type ColorScheme = 'light' | 'dark';
export type UserSpecifiedColorScheme = 'light' | 'dark' | 'system';

const DEFAULT_STORAGE_KEY = 'color-scheme';
const DEFAULT_ATTRIBUTE_NAME = 'data-theme';
const DEFAULT_FALLBACK: ColorScheme = 'light';
const DARK_SCHEME_MEDIA_QUERY = '(prefers-color-scheme: dark)';

const SUPPORTED_USER_VALUES = new Set<string>(['light', 'dark', 'system']);

export function getBrowserPreferredColorScheme(): ColorScheme {
  return readSystemScheme() ?? DEFAULT_FALLBACK;
}

function readSystemScheme(): ColorScheme | null {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return null;
  }
  return window.matchMedia(DARK_SCHEME_MEDIA_QUERY).matches ? 'dark' : 'light';
}

export interface ColorSchemeResolver {
  getCustomizedColorScheme(): Promise<UserSpecifiedColorScheme | null>;
  setCustomizedColorScheme(colorScheme: UserSpecifiedColorScheme | null): Promise<void>;
  /**
   * Optional. Notify the store when the persisted value may have changed
   * outside of `setCustomizedColorScheme` — e.g. another tab writing to the
   * same `localStorage` key. The store re-reads via `getCustomizedColorScheme`.
   * Returns an unsubscribe function the store calls on dispose.
   */
  subscribe?(callback: () => void): () => void;
}

export interface LocalStorageColorSchemeResolverOptions {
  storageKey?: string;
  storage?: Storage;
}

export class LocalStorageColorSchemeResolver implements ColorSchemeResolver {
  private readonly storageKey: string;
  private readonly storage: Storage | null;

  constructor(options: LocalStorageColorSchemeResolverOptions = {}) {
    this.storageKey = options.storageKey ?? DEFAULT_STORAGE_KEY;
    this.storage =
      options.storage ??
      (typeof globalThis !== 'undefined' && 'localStorage' in globalThis
        ? ((globalThis as { localStorage?: Storage }).localStorage ?? null)
        : null);
  }

  async getCustomizedColorScheme(): Promise<UserSpecifiedColorScheme | null> {
    if (!this.storage) return 'system';
    const raw = (this.storage.getItem(this.storageKey) ?? '').toLowerCase();
    return SUPPORTED_USER_VALUES.has(raw) ? (raw as UserSpecifiedColorScheme) : 'system';
  }

  async setCustomizedColorScheme(colorScheme: UserSpecifiedColorScheme | null): Promise<void> {
    if (!this.storage) return;
    if (colorScheme == null || colorScheme === 'system') {
      this.storage.removeItem(this.storageKey);
    } else {
      this.storage.setItem(this.storageKey, colorScheme);
    }
  }

  subscribe(callback: () => void): () => void {
    if (typeof window === 'undefined') return () => {};
    const handler = (event: StorageEvent) => {
      if (event.key === this.storageKey || event.key === null) {
        callback();
      }
    };
    window.addEventListener('storage', handler);
    return () => {
      window.removeEventListener('storage', handler);
    };
  }
}

export type ColorSchemeStrategy =
  | 'data-attribute'
  | 'class'
  | 'both'
  | ((scheme: ColorScheme) => void);

export interface ColorSchemeStoreOptions {
  resolver?: ColorSchemeResolver;
  strategy?: ColorSchemeStrategy;
  target?: HTMLElement;
  attributeName?: string;
}

export interface ColorSchemeContextValue {
  /** Resolved color scheme. Non-null from first render via the system query. */
  colorScheme: ColorScheme | null;
  /** True only on first render, until the persisted user choice is resolved. */
  isLoading: boolean;
  /** The user's choice: 'light', 'dark', or 'system'. */
  userSpecifiedColorScheme: UserSpecifiedColorScheme;
  /** The OS-level preferred scheme, or null if unavailable (SSR). */
  systemColorScheme: ColorScheme | null;
  /** Persist a new user choice. Awaitable. Passing null is treated as 'system'. */
  setColorScheme: (value: UserSpecifiedColorScheme | null) => Promise<void>;
}

interface Snapshot {
  colorScheme: ColorScheme | null;
  isLoading: boolean;
  userSpecifiedColorScheme: UserSpecifiedColorScheme;
  systemColorScheme: ColorScheme | null;
}

function applyToDom(
  scheme: ColorScheme,
  strategy: ColorSchemeStrategy,
  target: HTMLElement,
  attributeName: string
): void {
  if (typeof strategy === 'function') {
    strategy(scheme);
    return;
  }
  if (strategy === 'data-attribute' || strategy === 'both') {
    target.setAttribute(attributeName, scheme);
  }
  if (strategy === 'class' || strategy === 'both') {
    target.classList.remove('light', 'dark');
    target.classList.add(scheme);
  }
}

class ColorSchemeStore {
  private readonly resolver: ColorSchemeResolver | null;
  private readonly strategy: ColorSchemeStrategy;
  private readonly target: HTMLElement | undefined;
  private readonly attributeName: string;

  private snapshot: Snapshot;
  private readonly subscribers = new Set<() => void>();
  private resolverUnsubscribe: (() => void) | null = null;
  private mqlCleanup: (() => void) | null = null;
  private warnedMissingSystem = false;
  private started = false;

  constructor(options: ColorSchemeStoreOptions = {}) {
    this.resolver =
      options.resolver ??
      (typeof window !== 'undefined' ? new LocalStorageColorSchemeResolver() : null);
    this.strategy = options.strategy ?? 'class';
    this.target = options.target;
    this.attributeName = options.attributeName ?? DEFAULT_ATTRIBUTE_NAME;

    const system = readSystemScheme();
    this.snapshot = {
      colorScheme: system ?? DEFAULT_FALLBACK,
      isLoading: true,
      userSpecifiedColorScheme: 'system',
      systemColorScheme: system,
    };
  }

  start(): void {
    if (this.started) return;
    this.started = true;
    if (this.snapshot.systemColorScheme == null && !this.warnedMissingSystem) {
      this.warnedMissingSystem = true;
      console.error(`Unable to determine system color scheme, defaulting to '${DEFAULT_FALLBACK}'`);
    }
    this.applyDom();
    void this.load();
    this.resolverUnsubscribe = this.resolver?.subscribe?.(() => void this.load()) ?? null;
    this.mqlCleanup = this.attachMatchMediaListener();
  }

  dispose(): void {
    this.resolverUnsubscribe?.();
    this.resolverUnsubscribe = null;
    this.mqlCleanup?.();
    this.mqlCleanup = null;
    this.subscribers.clear();
    this.started = false;
  }

  subscribe = (cb: () => void): (() => void) => {
    this.subscribers.add(cb);
    return () => {
      this.subscribers.delete(cb);
    };
  };

  getSnapshot = (): Snapshot => this.snapshot;

  setColorScheme = async (value: UserSpecifiedColorScheme | null): Promise<void> => {
    const next = value ?? 'system';
    this.update({ userSpecifiedColorScheme: next });
    try {
      await this.resolver?.setCustomizedColorScheme(value);
    } catch (error) {
      console.error('Failed to persist color scheme', error);
    }
  };

  private async load(): Promise<void> {
    try {
      const stored = await this.resolver?.getCustomizedColorScheme();
      this.update({
        userSpecifiedColorScheme: stored ?? 'system',
        isLoading: false,
      });
    } catch (error) {
      console.error('Failed to load user-specified color scheme', error);
      this.update({ isLoading: false });
    }
  }

  private attachMatchMediaListener(): (() => void) | null {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
      return null;
    }
    const mql = window.matchMedia(DARK_SCHEME_MEDIA_QUERY);
    const handle = (event: MediaQueryListEvent) => {
      this.update({ systemColorScheme: event.matches ? 'dark' : 'light' });
    };
    mql.addEventListener('change', handle);
    return () => mql.removeEventListener('change', handle);
  }

  private update(patch: Partial<Snapshot>): void {
    const merged = { ...this.snapshot, ...patch };
    const resolved =
      merged.userSpecifiedColorScheme === 'system'
        ? (merged.systemColorScheme ?? DEFAULT_FALLBACK)
        : merged.userSpecifiedColorScheme;
    const next: Snapshot = { ...merged, colorScheme: resolved };
    if (
      next.colorScheme === this.snapshot.colorScheme &&
      next.isLoading === this.snapshot.isLoading &&
      next.userSpecifiedColorScheme === this.snapshot.userSpecifiedColorScheme &&
      next.systemColorScheme === this.snapshot.systemColorScheme
    ) {
      return;
    }
    const colorChanged = next.colorScheme !== this.snapshot.colorScheme;
    this.snapshot = next;
    if (colorChanged) this.applyDom();
    this.subscribers.forEach((cb) => cb());
  }

  private applyDom(): void {
    if (typeof document === 'undefined') return;
    if (this.snapshot.colorScheme == null) return;
    const el = this.target ?? document.documentElement;
    applyToDom(this.snapshot.colorScheme, this.strategy, el, this.attributeName);
  }
}

let defaultStore: ColorSchemeStore | null = null;
let defaultStoreOptions: ColorSchemeStoreOptions = {};

/**
 * Configure the default color-scheme store used by `useColorScheme()` calls
 * that are NOT wrapped in a `ColorSchemeProvider`. Call this exactly once at
 * app startup, before any hook invocation. Calls after the default store has
 * been initialized are ignored with a warning.
 */
export function configureColorScheme(options: ColorSchemeStoreOptions): void {
  if (defaultStore) {
    console.warn(
      'configureColorScheme: the default store has already been initialized; new options were ignored. Call this once before any useColorScheme() invocation, or use ColorSchemeProvider for scoped configuration.'
    );
    return;
  }
  defaultStoreOptions = options;
}

function getDefaultStore(): ColorSchemeStore {
  if (!defaultStore) {
    defaultStore = new ColorSchemeStore(defaultStoreOptions);
    defaultStore.start();
  }
  return defaultStore;
}

/** @internal — for tests only. Disposes and resets the module-level singleton. */
export function _resetDefaultColorSchemeStore(): void {
  defaultStore?.dispose();
  defaultStore = null;
  defaultStoreOptions = {};
}

const ColorSchemeContext = createContext<ColorSchemeStore | null>(null);
ColorSchemeContext.displayName = 'ColorSchemeContext';

export interface ColorSchemeProviderProps {
  colorSchemeResolver?: ColorSchemeResolver;
  /** How the resolved scheme is applied to the DOM. Default: "class". */
  strategy?: ColorSchemeStrategy;
  /** DOM target for "data-attribute"/"class"/"both" strategies. Default: <html>. */
  target?: HTMLElement;
  /** Attribute name used by "data-attribute" and "both". Default: "data-theme". */
  attributeName?: string;
}

export function ColorSchemeProvider({
  children,
  colorSchemeResolver,
  strategy,
  target,
  attributeName,
}: PropsWithChildren<ColorSchemeProviderProps>) {
  const [store] = useState(
    () =>
      new ColorSchemeStore({
        resolver: colorSchemeResolver,
        strategy,
        target,
        attributeName,
      })
  );

  useEffect(() => {
    store.start();
    return () => store.dispose();
  }, [store]);

  return <ColorSchemeContext.Provider value={store}>{children}</ColorSchemeContext.Provider>;
}

export function useColorScheme(): ColorSchemeContextValue {
  const ctxStore = useContext(ColorSchemeContext);
  const store = ctxStore ?? getDefaultStore();
  const snapshot = useSyncExternalStore(store.subscribe, store.getSnapshot, store.getSnapshot);
  return {
    colorScheme: snapshot.colorScheme,
    isLoading: snapshot.isLoading,
    userSpecifiedColorScheme: snapshot.userSpecifiedColorScheme,
    systemColorScheme: snapshot.systemColorScheme,
    setColorScheme: store.setColorScheme,
  };
}
