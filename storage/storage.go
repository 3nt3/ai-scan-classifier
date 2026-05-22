package storage

type Classification struct {
	Title       string `json:"title"`
	Category    string `json:"category"`
	Explanation string `json:"explanation"`
	FileName    string `json:"filename"`
	Business    bool   `json:"business"`
}

type StorageProvider interface {
	StoreFile([]byte, Classification) (string, error)
}
