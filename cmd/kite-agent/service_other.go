//go:build !windows

package main

import "errors"

var errWindowsOnly = errors.New("install, uninstall and service are only supported on Windows (use the systemd unit on Linux)")

func installService([]string) error { return errWindowsOnly }
func uninstallService() error       { return errWindowsOnly }
func runService() error             { return errWindowsOnly }
