# TDoc stream I/O

`LoadTDoc` / `LoadTDocFile` perform a complete document load. `LoadTDoc1` /
`LoadTDocFile1` accept a node limit, including the root; zero means unlimited.
On any reported failure, no partial document is returned. Scalar conversion
still happens in `Value`, `ReadAttr` or `Unmarshal`; parsing is not schema validation.

## Framing and compatibility

- Plain documents end at EOF or a blank line. LF, CR, CRLF and LFCR are accepted.
  Keep and reuse a `*bufio.Reader` to decode successive documents or inspect a
  prefix followed by its remaining nodes. Otherwise internal read-ahead bytes
  cannot be recovered from the original reader.
- A complete gzip load validates **one member**, including its checksum and
  trailer, even when the contained document ends at an earlier blank line.
  Consecutive members remain separate documents; reuse the buffered input to
  load the next member. The unread next member is not certified by this load.
- Limited gzip loads inspect only a prefix; they do not promise integrity of
  unread content or support resuming the decompressed suffix. Use a full load
  when validating a persisted file.
- Paired-newline detection needs one byte of lookahead. An encountered I/O
  failure is reported, including at a line-limit/blank-line boundary, rather
  than consumed and hidden as EOF. Reads from a stalled stream can block;
  this synchronous API does not add a timeout or cancellation mechanism.
- Space, tab, newline, carriage return, backslash and equals in keys round-trip
  through the writer's existing escapes. Empty/unnamed nodes, optional equals,
  raw scalar values and historical printable-character escapes remain supported.
- Invalid UTF-8, literal NUL, nonrepresentable escaped key controls, invalid
  indentation and unfinished EOF key escapes return syntax errors. Do not
  rewrite a failed input as an empty/default document. The indentation limit
  remains 128 levels; this is not a total decoded-byte or node-count limit.

## Diagnostics

| Code | Meaning |
| --- | --- |
| `SILK-DOC001` | Invalid document syntax; decoded UTF-8 line, byte column and offset are provided. |
| `SILK-DOC002` | Plain/source I/O failure, including opening a file. |
| `SILK-DOC003` | Gzip header, decompression, truncation, checksum or compressed-source failure. |

Errors include recovery hints and preserve underlying causes for `errors.Is` /
`errors.As`. File loaders add the filename. EOF on an empty stream is an error
that still matches `io.EOF`; malformed compressed members preserve
`io.ErrUnexpectedEOF` / `gzip.ErrChecksum` as applicable.

`Save` / `Save1` propagate buffered flush, short-write and gzip finalization
errors, including flushing a caller-provided buffered destination. `SaveFile`
also reports close errors, but is an **in-place**, non-atomic writer. For atomic
preferences and coalesced background saves, use the
[settings persistence API](../settings/PERSISTENCE.md).
