// processlibrary assembles inert reusable equipment and points without a GUI or driver.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/hmi"
)

func main() {
	// os.Exit bypasses defers; close the framework run marker on failures too.
	code := runCLI()
	if code != 0 {
		os.Exit(code)
	}
}

func runCLI() int {
	defer core.Close()
	flags := flag.NewFlagSet("processlibrary", flag.ContinueOnError)
	out := flags.String("out", "", "output directory (default: a fresh temporary directory)")
	if err := flags.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if err := run(*out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
func run(out string) error {
	var err error
	if out == "" {
		out, err = os.MkdirTemp("", "silk-device-library-*")
	} else {
		err = os.MkdirAll(out, 0755)
	}
	if err != nil {
		return err
	}
	lib := hmi.BuiltinDeviceLibrary()
	p := hmi.Project{Version: hmi.Version, Title: "Reusable equipment demo", Width: 2000, Height: 1000}
	var first hmi.DeviceInstance
	p, first, err = hmi.PlaceDeviceTemplate(p, lib.Devices[0], hmi.DevicePlacement{ID: "pump1", X: 20, Y: 20})
	if err != nil {
		return err
	}
	p, _, err = hmi.PlaceDeviceTemplate(p, lib.Devices[1], hmi.DevicePlacement{ID: "heat1", X: 900, Y: 50})
	if err != nil {
		return err
	}
	captured, err := hmi.CaptureDeviceTemplate(p, first.Nodes, "captured-pump", "Captured pump skid")
	if err != nil {
		return err
	}
	lib.Devices = append(lib.Devices, captured)
	p, _, err = hmi.PlaceDeviceTemplate(p, captured, hmi.DevicePlacement{ID: "pump2", X: 20, Y: 500, Points: map[string]string{"pump1.RUN": "pump2.RUN", "pump1.LEVEL": "pump2.LEVEL", "pump1.PRESSURE": "pump2.PRESSURE"}})
	if err != nil {
		return err
	}
	if err = hmi.SaveDeviceLibrary(context.Background(), filepath.Join(out, "devices.silkhmilibrary"), lib, false); err != nil {
		return err
	}
	// Atomically publish the project without replacing an existing destination.
	path := filepath.Join(out, "plant.silkhmi")
	tmp, err := os.CreateTemp(out, ".plant-stage-*")
	if err != nil {
		return err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	if err = hmi.Save(tmp.Name(), p); err != nil {
		return err
	}
	if err = os.Link(tmp.Name(), path); err != nil {
		return err
	}
	fmt.Printf("%s\n%d nodes / %d pipes / %d points; no sources started\n", out, len(p.Nodes), len(p.Links), len(p.Points))
	return nil
}
