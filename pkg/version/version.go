package version

import (
	"runtime/debug"
)

func AppName() string {
	return "prometheus-pulsar-remote-write"
}

func AppVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		return info.Main.Version
	}
	return "unknown"
}
