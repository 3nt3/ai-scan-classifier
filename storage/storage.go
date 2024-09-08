package storage

type Classification struct {
	Title       string `json:"title"`
	Category    string `json:"category"`
	Explanation string `json:"explanation"`
    FileName    string `json:"filename"`
}

type StorageProvider interface {
    StoreFile([]byte, Classification) (string, error)
}


