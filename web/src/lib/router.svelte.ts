// Minimal history-based router.

export type Route =
  | { name: 'overview' }
  | { name: 'devices'; id?: number }
  | { name: 'activity' }
  | { name: 'security' }
  | { name: 'settings'; section: string };

function parse(path: string): Route {
  const parts = path.split('/').filter(Boolean);
  switch (parts[0]) {
    case 'devices': {
      const id = Number(parts[1]);
      return { name: 'devices', id: Number.isInteger(id) && id > 0 ? id : undefined };
    }
    case 'activity':
      return { name: 'activity' };
    case 'security':
      return { name: 'security' };
    case 'settings':
      return { name: 'settings', section: parts[1] ?? 'general' };
    default:
      return { name: 'overview' };
  }
}

export const router = $state<{ route: Route; query: URLSearchParams }>({
  route: parse(location.pathname),
  query: new URLSearchParams(location.search),
});

export function navigate(path: string, opts: { replace?: boolean } = {}) {
  const url = new URL(path, location.origin);
  if (url.pathname + url.search === location.pathname + location.search) return;
  if (opts.replace) history.replaceState(null, '', url.pathname + url.search);
  else history.pushState(null, '', url.pathname + url.search);
  router.route = parse(url.pathname);
  router.query = url.searchParams;
}

addEventListener('popstate', () => {
  router.route = parse(location.pathname);
  router.query = new URLSearchParams(location.search);
});

/** Use on <a> elements: keeps normal link semantics but routes client-side. */
export function link(node: HTMLAnchorElement) {
  const onClick = (e: MouseEvent) => {
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    const href = node.getAttribute('href');
    if (!href || !href.startsWith('/')) return;
    e.preventDefault();
    navigate(href);
  };
  node.addEventListener('click', onClick);
  return { destroy: () => node.removeEventListener('click', onClick) };
}
