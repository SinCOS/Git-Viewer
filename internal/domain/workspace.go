package domain

import "encoding/json"

type WorkspaceFolder struct {
	Path string `json:"path"`
	Name string `json:"name,omitempty"`
}
type WorkspaceDocument struct {
	Folders    []WorkspaceFolder `json:"folders"`
	Settings   json.RawMessage   `json:"settings,omitempty"`
	Extensions json.RawMessage   `json:"extensions,omitempty"`
	Launch     json.RawMessage   `json:"launch,omitempty"`
	Tasks      json.RawMessage   `json:"tasks,omitempty"`
}
type WorkspaceFolderView struct {
	Path         string   `json:"path"`
	Name         string   `json:"name,omitempty"`
	ResolvedPath string   `json:"resolvedPath"`
	RepositoryID string   `json:"repositoryId,omitempty"`
	Diagnostics  []string `json:"diagnostics"`
}
type WorkspaceView struct {
	ID       string                `json:"id"`
	FilePath string                `json:"filePath"`
	Revision string                `json:"revision"`
	Folders  []WorkspaceFolderView `json:"folders"`
}
