export interface FoucScriptOptions {
  storageKey?: string;
  attributeName?: string;
  strategy?: 'data-attribute' | 'class' | 'both';
}

/**
 * Returns an IIFE-shaped JS string that synchronously applies the persisted
 * color scheme to <html> before React mounts. Inject into <head> via:
 *
 *   <script dangerouslySetInnerHTML={{ __html: getColorSchemeFoucScript() }} />
 */
export function getColorSchemeFoucScript(options: FoucScriptOptions = {}): string {
  const storageKey = options.storageKey ?? 'color-scheme';
  const attributeName = options.attributeName ?? 'data-theme';
  const strategy = options.strategy ?? 'class';

  const k = JSON.stringify(storageKey);
  const a = JSON.stringify(attributeName);
  const s = JSON.stringify(strategy);

  return `(function(){try{var k=${k},a=${a},s=${s};var stored=null;try{stored=(localStorage.getItem(k)||'').toLowerCase();}catch(e){}var scheme;if(stored==='light'||stored==='dark'){scheme=stored;}else{scheme=(window.matchMedia&&window.matchMedia('(prefers-color-scheme: dark)').matches)?'dark':'light';}var el=document.documentElement;if(s==='data-attribute'||s==='both'){el.setAttribute(a,scheme);}if(s==='class'||s==='both'){el.classList.remove('light','dark');el.classList.add(scheme);}}catch(e){}})();`;
}
