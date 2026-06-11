//go:build !linux

package backup

import "syscall"

// weedSysProcAttr is a no-op on non-Linux platforms (Pdeathsig is Linux-only).
func weedSysProcAttr() *syscall.SysProcAttr {
	return nil
}
