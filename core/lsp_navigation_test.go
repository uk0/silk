package core

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestSemanticNavigationWireShapes(t *testing.T) {
	const link = `[{"targetUri":"file:///target.go","targetRange":{"start":{"line":2,"character":0},"end":{"line":9,"character":0}},"targetSelectionRange":{"start":{"line":3,"character":7},"end":{"line":3,"character":13}}}]`
	for _, method := range []string{"definition", "typeDefinition", "implementation"} {
		t.Run(method, func(t *testing.T) {
			c, in, out := newPipedClient(t)
			defer c.Close()
			done := runFakeServer(t, in, out, func(m string, _ *json.RawMessage, p json.RawMessage) *fakeReply {
				if m != "textDocument/"+method {
					t.Errorf("method=%s", m)
				}
				var params struct {
					Position LSPPosition `json:"position"`
				}
				if err := json.Unmarshal(p, &params); err != nil {
					t.Error(err)
				}
				if params.Position.Line != 4 || params.Position.Character != 12 {
					t.Errorf("lost UTF16 request: %s", p)
				}
				return &fakeReply{Result: json.RawMessage(link)}
			})
			var got []LSPLocation
			var err error
			switch method {
			case "definition":
				got, err = c.Definition("file:///source.go", 4, 12)
			case "typeDefinition":
				got, err = c.TypeDefinition("file:///source.go", 4, 12)
			case "implementation":
				got, err = c.Implementation("file:///source.go", 4, 12)
			}
			if err != nil || len(got) != 1 || got[0].URI != "file:///target.go" || got[0].Range.Start.Line != 3 || got[0].Range.Start.Character != 7 {
				t.Fatalf("navigation=%+v %v", got, err)
			}
			in.Close()
			out.Close()
			<-done
		})
	}
}

func TestNavigationNullInvalidAndCancellation(t *testing.T) {
	for _, raw := range []string{"", "null", " \nnull\t", "[]"} {
		locs, err := decodeLocationsResult(json.RawMessage(raw), "definition")
		if err != nil || len(locs) != 0 {
			t.Fatalf("%q: %v %v", raw, locs, err)
		}
	}
	for _, raw := range []string{"{}", "[null]", "[{}]", "42", "{bad"} {
		if _, err := decodeLocationsResult(json.RawMessage(raw), "definition"); err == nil {
			t.Fatalf("accepted invalid target %s", raw)
		}
	}
	c, buf := advOfflineClient()
	c.serverCaps = json.RawMessage(`{"typeDefinitionProvider":false}`)
	if locs, err := c.TypeDefinition("file:///a.go", 0, 0); err != nil || len(locs) != 0 || buf.Len() != 0 {
		t.Fatal("unsupported type definition sent a request")
	}
	c.serverCaps = nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.DefinitionContext(ctx, "file:///a.go", 0, 0); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
}
