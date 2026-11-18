//go:build !unix

package agentaccounts

// openNoWait is nothing where there are no named pipes to wait on.
const openNoWait = 0
