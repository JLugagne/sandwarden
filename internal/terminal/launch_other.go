//go:build !darwin

package terminal

func bundleLaunch(bin string, argv []string) (string, []string) { return bin, argv }
