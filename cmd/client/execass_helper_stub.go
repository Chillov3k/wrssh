//go:build !execass || !windows

package main

func runExecassHelperIfRequested() bool {
	return false
}
