package utils

import (
	"os"
	"strings"
)

func RewriteSubpath(subpath string) error {
	// Read ./dist/index.html
	htmlBytes, err := os.ReadFile("./dist/index.html")
	if err != nil {
		return err
	}

	// Replace all occurrences of "/assets/" with the subpath
	htmlString := string(htmlBytes)
	if !strings.Contains(htmlString, subpath) {
		htmlString = strings.ReplaceAll(htmlString, "/assets/", subpath+"assets/")
	}

	// Overwrite the file
	err = os.WriteFile("./dist/index.html", []byte(htmlString), 0644)

	return err
}
