//go:build !networkdebug

package buildmode

// A compile-time constant: diagnostic branches are absent from production code.
const NetworkDebug = false
