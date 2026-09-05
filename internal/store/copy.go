package store

import (
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/agensfield/fulla/internal/securefs"
)

func copyTree(root *os.Root, source, destination string) error {
	return fs.WalkDir(root.FS(), source, func(name string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		if err := securefs.ValidateInfo(name, info, true); err != nil {
			return err
		}
		target := destination + strings.TrimPrefix(name, source)
		if e.IsDir() {
			return root.MkdirAll(target, 0o700)
		}
		data, err := securefs.Read(root, name, 256<<20)
		if err != nil {
			return err
		}
		if err := securefs.WriteNew(root, target, data); err != nil {
			return err
		}
		return securefs.SyncDir(root, path.Dir(target))
	})
}
