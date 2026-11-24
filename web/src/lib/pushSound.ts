// The optional sound for a notification while reet is open in front: off by default, chosen per device. A short, quiet
// two-note tone made with Web Audio, so there is no audio file. Browsers let a page play sound only after a press on it;
// the card's Try it is that press.

const KEY = 'reet.push.sound'

export function soundOn(): boolean {
  try { return localStorage.getItem(KEY) === '1' } catch { return false }
}

export function setSound(on: boolean): void {
  try { if (on) localStorage.setItem(KEY, '1'); else localStorage.removeItem(KEY) } catch { /* storage unavailable: stays off */ }
}

let audio: AudioContext | undefined

/** Two rising notes, about 0.4 seconds in all, at a low volume. Never throws. */
export function chime(): void {
  try {
    audio ??= new AudioContext()
    if (audio.state === 'suspended') void audio.resume().catch(() => { /* not allowed yet: silent */ })
    const start = audio.currentTime + 0.02
    ;[660, 880].forEach((frequency, i) => {
      const at = start + i * 0.16, tone = audio!.createOscillator(), level = audio!.createGain()
      tone.type = 'sine'
      tone.frequency.value = frequency
      level.gain.setValueAtTime(0.0001, at)
      level.gain.exponentialRampToValueAtTime(0.05, at + 0.02)
      level.gain.exponentialRampToValueAtTime(0.0001, at + 0.22)
      tone.connect(level).connect(audio!.destination)
      tone.start(at)
      tone.stop(at + 0.24)
    })
  } catch { /* no audio in this browser */ }
}
