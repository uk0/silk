package httpx

import "github.com/uk0/silk/jsonx"

// Document parses a bounded immutable JSON document. Parse once, then reuse it
// for multiple fields or a JSONView. Existing JSON/Str/Int APIs are unchanged.
// This call performs CPU work synchronously; use a worker for large responses.
func (r *Response) Document() (*jsonx.Document, error) { return r.DocumentWithOptions(jsonx.Options{}) }
func (r *Response) DocumentWithOptions(options jsonx.Options) (*jsonx.Document, error) {
	if r == nil {
		return nil, jsonx.ErrNotFound
	}
	return jsonx.ParseWithOptions(r.body, options)
}
