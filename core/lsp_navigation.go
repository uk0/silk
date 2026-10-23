package core

import "context"

// TypeDefinition resolves the declared type of the symbol under the caret.
// Coordinates are zero-based UTF-16. Unsupported servers return no locations.
func (c *LSPClient) TypeDefinition(uri string, line, character int) ([]LSPLocation, error) {
	return c.TypeDefinitionContext(context.Background(), uri, line, character)
}

// TypeDefinitionContext supports bounded/cancellable IDE navigation requests.
func (c *LSPClient) TypeDefinitionContext(ctx context.Context, uri string, line, character int) ([]LSPLocation, error) {
	raw, ok, err := c.requestOptionalContext(ctx, "textDocument/typeDefinition", "typeDefinitionProvider",
		textDocumentPositionParams(uri, line, character))
	if err != nil || !ok {
		return nil, err
	}
	return decodeLocationsResult(raw, "type definition")
}
