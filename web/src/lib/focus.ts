/** Focus on load suits a keyboard and a wide screen. On a phone it raises the soft keyboard over the page before it has been read. */
export function canAutoFocus(): boolean {
  return typeof matchMedia === 'function' && matchMedia('(pointer: fine)').matches && matchMedia('(min-width: 700px)').matches
}
