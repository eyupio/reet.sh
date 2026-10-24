//go:build !linux && !darwin

package skillbundle

// Executable-mode observation is not yet verified on these platforms.
const readFlags = 0
const supportedModes = false
