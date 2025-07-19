package parser

import (
	"os"
)

// FileList returns a list of all files from the provided rootDir.
// Optionally pass an exclusion list to skip certain file names.
func FileList(rootDir string, exclusionList ...string) ([]string, error) {
	// Convert exclusionList slice to a map for fast lookup
	exclusions := make(map[string]struct{}, len(exclusionList))
	for _, name := range exclusionList {
		exclusions[name] = struct{}{}
	}

	fileList := make([]string, 0)

	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			if _, skip := exclusions[entry.Name()]; !skip {
				fileList = append(fileList, entry.Name())
			}
		}
	}

	return fileList, nil
}
