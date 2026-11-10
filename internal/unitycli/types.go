package unitycli

import "time"

// Project is a project in the Hub's project database, as the CLI lists it.
type Project struct {
	Title                string `json:"title"`
	Path                 string `json:"path"`
	ContainingFolderPath string `json:"containingFolderPath"`
	Version              string `json:"version"` // editor version, e.g. 6000.6.8f1
	Architecture         string `json:"architecture"`
	Changeset            string `json:"changeset"`
	LastModified         int64  `json:"lastModified"` // milliseconds since the epoch
	IsFavorite           bool   `json:"isFavorite"`
	IsCustomEditor       bool   `json:"isCustomEditor"`
	// BuildTarget is the project's active build target as a Unity enum name,
	// such as StandaloneLinux64. It is read from the project's Library folder.
	BuildTarget    string `json:"buildTarget"`
	RenderPipeline string `json:"renderPipeline"`
	CloudEnabled   bool   `json:"cloudEnabled"`
}

// LastOpened is when the Hub last recorded the project as used.
func (p Project) LastOpened() time.Time {
	if p.LastModified <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(p.LastModified)
}

// ProjectInfo is `unity projects info`: a Project plus its contents.
type ProjectInfo struct {
	Project
	ScriptingBackend map[string]string `json:"scriptingBackend"`
	Packages         map[string]string `json:"packages"`
}

// Editor is an installed Unity Editor.
type Editor struct {
	Version      string   `json:"version"`
	Alias        string   `json:"alias"`
	Architecture string   `json:"architecture"`
	Location     string   `json:"location"` // path to the Editor executable
	ModuleNames  []string `json:"moduleNames"`
	Default      bool     `json:"default"`
}

// Module is an installable part of an Editor, such as a build platform.
type Module struct {
	ID             string `json:"id"`
	Aliases        string `json:"aliases"`
	Name           string `json:"name"`
	Category       string `json:"category"`
	Status         string `json:"status"`
	DownloadBytes  int64  `json:"downloadBytes"`
	InstalledBytes int64  `json:"installedBytes"`
	IsInstalled    bool   `json:"isInstalled"`
}

// Release is an Editor version available for download.
type Release struct {
	Version      string  `json:"version"`
	Architecture string  `json:"architecture"`
	Stream       string  `json:"stream"` // LTS, TECH, BETA, ALPHA
	LTS          bool    `json:"lts"`
	Installed    bool    `json:"installed"`
	RevisionHash *string `json:"revisionHash"`
}

// ReleasePage is one page of releases.
type ReleasePage struct {
	Items []Release
	// NextOffset is the offset to request for the following page, 0 when there
	// are no more. (The CLI calls this field "nextPage", but it is an offset.)
	NextOffset int
}

// Template is a project template.
type Template struct {
	Name        string `json:"name"` // package id, e.g. com.unity.template.urp-blank
	DisplayName string `json:"displayName"`
	Type        string `json:"type"`
	Version     string `json:"version"`
	Status      string `json:"status"`
}

// AuthInfo is the sign-in state of the CLI.
type AuthInfo struct {
	LoggedIn     bool   `json:"loggedIn"`
	SessionState string `json:"sessionState"`
	Mode         string `json:"mode"`
	User         struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"user"`
}

// InstallPlan is what `install --dry-run` reports.
type InstallPlan struct {
	AlreadyInstalled bool `json:"alreadyInstalled"`
	Editor           struct {
		Version      string `json:"version"`
		Architecture string `json:"architecture"`
		DownloadSize int64  `json:"downloadSize"`
	} `json:"editor"`
	Modules []struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		DownloadSize int64  `json:"downloadSize"`
	} `json:"modules"`
	TotalDownloadSize int64 `json:"totalDownloadSize"`
}
