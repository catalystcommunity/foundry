//go:build linux

package backup

import "syscall"

// weedSysProcAttr makes the kernel send SIGKILL to the weed child if foundry (its
// parent) dies, so a crashed/killed run never leaves an orphaned S3 server holding
// the port.
func weedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
}
