package assets

import (
	"embed"
	"io/fs"
)

//go:embed dist
var embeddedAssets embed.FS

// GetEmbeddedAssets returns the embedded frontend assets filesystem
// with the "dist" prefix stripped. Returns nil if assets are not embedded.
func GetEmbeddedAssets() fs.FS {
	// Check if the dist directory exists in the embedded filesystem
	if _, err := embeddedAssets.ReadDir("dist"); err != nil {
		// Assets not embedded (likely development build)
		return nil
	}
	
	assets, err := fs.Sub(embeddedAssets, "dist")
	if err != nil {
		// This shouldn't happen if ReadDir succeeded above
		return nil
	}
	return assets
}

// HasEmbeddedAssets returns true if frontend assets are embedded
func HasEmbeddedAssets() bool {
	_, err := embeddedAssets.ReadDir("dist")
	return err == nil
}