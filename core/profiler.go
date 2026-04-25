package core

import (
	"errors"
	"os"
	"runtime/pprof"
)

var cpuProfilePath string
var cpuProfileFile *os.File

func ProfilerStart() error {
	Log("ProfilerStart()")
	if cpuProfileFile != nil {
		return errors.New("profiler already started.")
	}
	cpuProfilePath = ExeFile() + ".prof"
	var err error
	cpuProfileFile, err = os.Create(cpuProfilePath)
	if err != nil {
		return err
	}
	Trace("before pprof.StartCPUProfile(cpuProfileFile)")
	pprof.StartCPUProfile(cpuProfileFile)
	Trace("after pprof.StartCPUProfile(cpuProfileFile)")

	return nil
}

func ProfilerStop() {
	Log("ProfilerStop()")
	if cpuProfileFile == nil {
		return
	}
	cpuProfileFile = nil
	pprof.StopCPUProfile()

	//	pprof.Lookup()
}
