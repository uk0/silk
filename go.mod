module github.com/uk0/silk

go 1.25.0

require (
	github.com/go-gl/gl v0.0.0-20260331235117-4566fea9a276
	github.com/go-gl/glfw/v3.3/glfw v0.0.0-20260406072232-3ac4aa2bb164
	github.com/robinson/gos7 v0.0.0-20260622162611-2d6806f80c8b
	github.com/simonvetter/modbus v1.6.4
	github.com/traefik/yaegi v0.16.1
	golang.org/x/image v0.39.0
	golang.org/x/net v0.52.0
)

require (
	github.com/goburrow/serial v0.1.0 // indirect
	golang.org/x/text v0.36.0 // indirect
)

replace mod/map => ./mod/map
