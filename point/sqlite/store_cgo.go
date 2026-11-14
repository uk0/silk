//go:build cgo

package sqlite

// StoreAvailable reports whether SQLite storage is compiled in.
func StoreAvailable() bool { return true }
