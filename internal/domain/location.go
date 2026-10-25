package domain

// SavedLocation stores connection aliases and paths, never identities or credentials.
type SavedLocation struct {
	Kind string `json:"kind"`
	Host string `json:"host,omitempty"`
	Path string `json:"path"`
}
