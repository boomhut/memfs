package memfs

type Option interface {
	setOption(*fsOption)
}

type fsOption struct {
	openHook      func(path string, existingContent []byte, origErr error) ([]byte, error)
	maxStorage    int64
	encryptionKey []byte
}

type openHookOption struct {
	hook func(string, []byte, error) ([]byte, error)
}

func (o *openHookOption) setOption(fsOpt *fsOption) {
	fsOpt.openHook = o.hook
}

// WithOpenHook returns an Option that sets a hook function to be called
// when opening files in the MemFS.
//
// The hook function takes three parameters:
//   - path: the path of the file being opened
//   - content: the original content of the file (may be nil if the file doesn't exist)
//   - origError: the original error returned when trying to open the file (may be nil)
//
// The hook function returns:
//   - []byte: the new content of the file
//   - error: any error that occurred during the hook's execution
func WithOpenHook(f func(string, []byte, error) ([]byte, error)) Option {
	return &openHookOption{
		hook: f,
	}
}

type maxStorageOption struct {
	size int64
}

func (o *maxStorageOption) setOption(fsOpt *fsOption) {
	fsOpt.maxStorage = o.size
}

// WithMaxStorage returns an Option that sets the maximum storage space (in bytes) for the MemFS instance.
// If the total size of all files in the MemFS exceeds this limit, an error will be returned when trying to write new files.
func WithMaxStorage(size int64) Option {
	return &maxStorageOption{
		size: size,
	}
}

type encryptionOption struct {
	key []byte
}

func (o *encryptionOption) setOption(fsOpt *fsOption) {
	fsOpt.encryptionKey = o.key
}

// WithEncryption returns an Option that enables encryption at rest for all file data stored in the MemFS.
// The encryption key can be of any length and will be hashed to 32 bytes for AES-256-GCM encryption.
// All file contents will be automatically encrypted when written and decrypted when read.
//
// Example:
//
//	key := []byte("my-secret-encryption-key")
//	fs := memfs.New(memfs.WithEncryption(key))
//
// Note: The encryption key is not stored with the filesystem. You must provide the same key
// when loading a saved encrypted filesystem, otherwise decryption will fail.
func WithEncryption(key []byte) Option {
	return &encryptionOption{
		key: key,
	}
}
