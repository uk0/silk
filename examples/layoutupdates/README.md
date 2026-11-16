# Event-driven layout updates

```sh
go run ./examples/layoutupdates
```

Use **Long status / Short status**, **Toggle font**, **Toggle detail** and
**Toggle spacing**, then resize the window. Wrapped text, its nested group and
following controls recompute their preferred geometry without a polling timer
or a manual `Layout()` in each action. Use a reasonably tall window: this
example deliberately does not add automatic scrolling for undersized clients.

Custom widgets call `widget.InvalidateSizeHints()` after changing preferred-size
inputs. `widget.RequestLayout()` also queues the widget's own layout. Both are
UI-thread-only; marshal worker results through `gui.Post`. Existing explicit
`InvalidateLayout()` / `InvalidateParentLayout()` retain their immediate normal
path; reentrant invalidation is deferred instead of recursing into the same
layout. Ancestors are coalesced on the next UI turn, not synchronously inside
ordinary text/font setters.

Hidden/unmeasured hierarchies retain dirty state without active queued work.
Show/resize/reparent admits current geometry; owner-local requests are revoked
on hide/detach. A batch yields after at most 64 entries or its soft 4 ms budget;
one custom `Layout` callback itself cannot be preempted. This is not a guarantee
of constant-time layout, atomic whole-tree reflow, full DPI support, or arbitrary
user callback reentrancy safety.
