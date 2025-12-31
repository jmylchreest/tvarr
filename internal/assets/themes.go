// Package assets provides embedded static assets for the tvarr application.
package assets

import (
	"io/fs"
)

// GetThemesFS returns a sub-filesystem rooted at the themes directory within the embedded frontend.
// The themes are built into the frontend's public/themes/ folder and embedded with the rest
// of the frontend static assets.
// Returns an error if the themes directory doesn't exist.
func GetThemesFS() (fs.FS, error) {
	staticFS, err := GetStaticFS()
	if err != nil {
		return nil, err
	}
	return fs.Sub(staticFS, "themes")
}

