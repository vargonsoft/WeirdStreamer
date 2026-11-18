package metadata

import "github.com/weirdstreamer/server/internal/config"

// The metadata tables keep folders and files by client path (database/keys.go); this package
// works with disk paths, turned into client paths (config.ClientPath) only to read and write them.

// setFolderKey records that a folder (a disk path in lib) stands for one title. A folder outside
// every library (one configured since) has no client path, and nothing is recorded.
func (s *Service) setFolderKey(lib config.MediaPath, folder, kind, key string) error {
	client, ok := config.ClientPath(folder)
	if !ok {
		return nil
	}
	return s.db.SetFolderKey(lib.ID, client, kind, key)
}

// folderKeys returns the lookup keys recorded for folders (disk paths in lib), by disk path.
func (s *Service) folderKeys(lib config.MediaPath, folders []string) (map[string]string, error) {
	byClient := map[string]string{}
	var clients []string
	for _, f := range folders {
		if c, ok := config.ClientPath(f); ok {
			byClient[c] = f
			clients = append(clients, c)
		}
	}
	keys, err := s.db.FolderKeys(lib.ID, clients)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(keys))
	for c, k := range keys {
		out[byClient[c]] = k
	}
	return out, nil
}

// clientPath is a disk path's client path, or "" for one outside every library.
func clientPath(path string) string {
	c, _ := config.ClientPath(path)
	return c
}
