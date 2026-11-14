package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/hmiapp"
	"net/http"
	"os"
)

func main() {
	path := flag.String("project", "", ".silkhmi file to load (no I/O sources auto-started)")
	db := flag.String("db", "", "optional SQLite point store")
	flag.Parse()
	c := hmiapp.Config{ProjectPath: *path, DatabasePath: *db}
	if raw := os.Getenv("SILK_HMI_HTTP_HEADERS"); raw != "" {
		var headers map[string]http.Header
		if e := json.Unmarshal([]byte(raw), &headers); e != nil {
			fmt.Fprintln(os.Stderr, "invalid runtime header environment")
			os.Exit(1)
		}
		c.RequestHeaders = func(id string) http.Header { return headers[id].Clone() }
	}
	if *path != "" {
		p, e := hmi.Load(*path)
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		b, _ := p.JSON()
		c.ProjectJSON = string(b)
	}
	if e := hmiapp.Run(c); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
