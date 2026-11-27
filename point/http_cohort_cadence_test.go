package point

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/httpx"
)

type cadenceTransport func(*http.Request) (*http.Response, error)

func (f cadenceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type cadenceStore func(context.Context, string, core.Value) error

func (f cadenceStore) Publish(ctx context.Context, name string, sample core.Value) error {
	return f(ctx, name, sample)
}

func TestHTTPSourceKeepsHealthyMappingsAtConfiguredCadence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		const interval = 100 * time.Millisecond
		const first int64 = 9007199254740993
		tags := core.NewTagDB()
		bi := tags.GetOrCreate("BI11", core.Meta{})
		di := tags.GetOrCreate("DI11", core.Meta{})
		di.SetValue(false)

		replies := []struct {
			status int
			body   string
		}{
			{200, fmt.Sprintf(`{"data":{"counter":%d,"drive":"invalid"}}`, first)},
			{503, "temporarily unavailable"},
			{200, `{"data":`},
			{200, fmt.Sprintf(`{"data":{"counter":%d}}`, first+1)},
			{200, fmt.Sprintf(`{"data":{"counter":%d,"drive":true}}`, first+2)},
			{200, fmt.Sprintf(`{"data":{"counter":%d,"drive":true}}`, first+3)},
		}
		errStorage := errors.New("poll storage unavailable")
		var i int // one owner: the polling goroutine
		transport := cadenceTransport(func(r *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(r.Body)
			_ = r.Body.Close()
			if err != nil || string(body) != `{"device":"R-001"}` || r.Method != "POST" ||
				r.URL.Path != "/points" || r.Header.Get("Content-Type") != "application/json" {
				return nil, fmt.Errorf("unexpected poll request: %s %s", r.Method, r.URL.Path)
			}
			if i >= len(replies) {
				return nil, errors.New("unexpected extra poll")
			}
			response := replies[i]
			i++
			return &http.Response{StatusCode: response.status, Body: io.NopCloser(strings.NewReader(response.body)), Header: make(http.Header)}, nil
		})
		client, err := httpx.NewClient(httpx.Options{HTTPClient: &http.Client{Transport: transport}})
		if err != nil {
			t.Fatal(err)
		}
		store := cadenceStore(func(_ context.Context, _ string, _ core.Value) error {
			if i == 5 {
				return errStorage // valid response, but neither value may go live
			}
			return nil
		})
		src, err := NewHTTPSource(client, httpx.Request{
			URL: "https://process.example.invalid/points", Method: "POST",
			Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{"device":"R-001"}`),
		}, []Mapping{{Name: "BI11", Pointer: "/data/counter", Kind: Int}, {Name: "DI11", Pointer: "/data/drive", Kind: Bool}}, tags, store)
		if err != nil {
			t.Fatal(err)
		}
		type report struct {
			at     time.Duration
			err    error
			bi, di core.Value
		}
		ch := make(chan report, 8)
		done := make(chan error, 1)
		start := time.Now()
		go func() {
			done <- src.RunObserved(ctx, interval, true, func(err error) {
				ch <- report{time.Since(start), err, bi.Value(), di.Value()}
			})
		}()
		defer func() {
			cancel()
			if e := <-done; !errors.Is(e, context.Canceled) {
				t.Errorf("poll stop: %v", e)
			}
		}()
		advance := func(offset time.Duration) {
			if until := offset - time.Since(start); until > 0 {
				time.Sleep(until)
			}
			synctest.Wait()
		}
		poll := func(at time.Duration, wantError bool, counter int64, biQuality core.Quality, drive bool, diQuality core.Quality) report {
			t.Helper()
			advance(at + time.Nanosecond)
			select {
			case v := <-ch:
				if v.at != at || (v.err != nil) != wantError || v.bi.Raw != counter || v.bi.Quality != biQuality ||
					v.di.Raw != drive || v.di.Quality != diQuality {
					t.Fatalf("poll at %v: got %+v; BI=%+v DI=%+v", at, v, v.bi, v.di)
				}
				return v
			default:
				t.Fatalf("no poll at %v", at)
				return report{}
			}
		}
		poll(0, true, first, core.QualityGood, false, core.QualityBad)
		poll(interval, true, first, core.QualityBad, false, core.QualityBad)
		poll(3*interval, true, first, core.QualityBad, false, core.QualityBad)
		partial := poll(7*interval, true, first+1, core.QualityGood, false, core.QualityBad)
		failed := poll(8*interval, true, first+1, core.QualityGood, false, core.QualityBad)
		if !errors.Is(failed.err, errStorage) || failed.bi != partial.bi || failed.di != partial.di {
			t.Fatalf("storage failure touched live values or hid the error: %+v", failed)
		}
		poll(10*interval, false, first+3, core.QualityGood, true, core.QualityGood)
		advance(11*interval - time.Nanosecond)
		if len(ch) != 0 {
			t.Fatalf("extra poll before the next configured interval: %d", len(ch))
		}
	})
}
