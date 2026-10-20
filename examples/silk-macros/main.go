package main

//go:generate go run ../../cmd/silkgen -macro -o sensor_gen.go sensor.silkgo

import "fmt"

func main() {
	s := Sensor{}
	s.SetPressure(42.5)
	s.SetEnabled(true)
	fmt.Printf("pressure=%.1f enabled=%t\n", s.Pressure, s.Enabled)
}
