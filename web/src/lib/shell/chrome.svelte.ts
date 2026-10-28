// Which field outside the top bar has focus and asks the phone chrome to step aside while the keyboard is up
// (keyboardChrome in terminal/sizing.ts). The launch bar's prompt sets `launch`; the session screen's terminal sets `session`;
// the shell reads them. Nothing is stored.
export const chromeFocus = $state<{ launch: boolean; session: boolean }>({ launch: false, session: false })
