//go:build !linux && !darwin

package service

import (
	"context"
	"runtime"
)

type unsupportedPlatform struct{ goos string }

var current platform = unsupportedPlatform{goos: runtime.GOOS}

func (p unsupportedPlatform) name() string { return p.goos }

func (p unsupportedPlatform) unitPath() (string, error) { return "", unsupportedError(p.goos) }

func (p unsupportedPlatform) install(context.Context, Runner, Spec) (Report, error) {
	return Report{}, unsupportedError(p.goos)
}

func (p unsupportedPlatform) status(context.Context, Runner) (Status, error) {
	return Status{}, unsupportedError(p.goos)
}

func (p unsupportedPlatform) uninstall(context.Context, Runner) (Report, error) {
	return Report{}, unsupportedError(p.goos)
}
