package core

type IPersistSaver interface {
	Write(key string, v interface{}) error

	Enter(key string) bool
	Leave()
}

type IPersistLoader interface {
	Read(key string, ptr interface{}) error

	Enter(key string) bool
	Leave()
}

type IPersist interface {
	OnPersistSave(p IPersistSaver)
	OnPersistLoad(p IPersistLoader)
}

type IPsLoaded interface {
	OnPersistLoaded() error
}
