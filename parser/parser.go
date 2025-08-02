package parser

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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

// CBZExists checks if ch<num>.cbz already exists in current directory.
// Returns true if exists, false otherwise. Returns error if unexpected.
func CBZExists(chapterNumber int) (bool, error) {
	var cbzName string
	if chapterNumber < 10 {
		cbzName = fmt.Sprintf("ch%02d.cbz", chapterNumber)
	} else {
		cbzName = fmt.Sprintf("ch%d.cbz", chapterNumber)
	}
	cbzPath := filepath.Join(".", cbzName)

	_, err := os.Stat(cbzPath)
	if err == nil {
		// File exists
		return true, nil
	}
	if os.IsNotExist(err) {
		// File does not exist
		return false, nil
	}
	// Other unexpected error
	return false, err
}

// Returns only chapter URLs that haven't been downloaded (no .cbz file present).
func FilterUndownloadedChapters(chapterStrs []string) []string {
	re := regexp.MustCompile(`chapter-([\d.]+)`)
	var filtered []string

	for _, str := range chapterStrs {
		match := re.FindStringSubmatch(str)
		if len(match) < 2 {
			continue
		}

		raw := match[1]
		cbzName := ""

		if strings.Contains(raw, ".") {
			parts := strings.SplitN(raw, ".", 2)
			intPart, err := strconv.Atoi(parts[0])
			if err != nil {
				continue
			}
			cbzName = fmt.Sprintf("ch%03d.%s.cbz", intPart, parts[1])
		} else {
			num, err := strconv.Atoi(raw)
			if err != nil {
				continue
			}
			cbzName = fmt.Sprintf("ch%03d.cbz", num)
		}

		if _, err := os.Stat(cbzName); os.IsNotExist(err) {
			filtered = append(filtered, str)
		} else {
			log.Printf("[MAIN] Skipping chapter %s: CBZ already exists", raw)
		}
	}

	return filtered
}

// generic func to create CBZ file from a dir path and providing the chapter/manga name
func CreateCBZ(dirPath string, outputFilename string) error {
	outFile, err := os.Create(outputFilename)
	if err != nil {
		return fmt.Errorf("unable to create output CBZ file: %v", err)
	}
	defer outFile.Close()

	zipWriter := zip.NewWriter(outFile)
	defer zipWriter.Close()

	err = filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}

		fw, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(fw, file)
		return err
	})

	if err != nil {
		return fmt.Errorf("error creating CBZ archive: %v", err)
	}

	return nil
}
