export interface Toast {
  id: number;
  text: string;
  kind: 'ok' | 'error' | 'info';
}

export const toasts = $state<{ list: Toast[] }>({ list: [] });
let next = 1;

export function toast(text: string, kind: Toast['kind'] = 'ok', ms = 3500) {
  const id = next++;
  toasts.list = [...toasts.list, { id, text, kind }];
  setTimeout(() => {
    toasts.list = toasts.list.filter((t) => t.id !== id);
  }, ms);
}
