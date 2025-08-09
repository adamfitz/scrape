package parser

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	_ "image/gif" // register GIF decoder
	_ "image/png" // register PNG decoder

	"github.com/chai2010/webp"
	_ "golang.org/x/image/webp" // register WEBP decoder, add module
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

// func exist to extract the keys from a map, sort ascending order and return string slice
func SortKeys(inputMap map[string]string) ([]string, error) {

	var sortedList []string

	for key := range inputMap {
		sortedList = append(sortedList, key)
	}

	sort.Strings(sortedList)

	return sortedList, nil
}

// DownloadAndConvertToJPG downloads an image from imageURL,
// converts to JPG if needed, and saves it inside targetDir.
// Returns error if any.
func DownloadAndConvertToJPG(imageURL, targetDir string) error {
	resp, err := http.Get(imageURL)
	if err != nil {
		return fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("bad response status: %s", resp.Status)
	}

	imgBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read image data: %w", err)
	}

	format, err := DetectImageFormat(imgBytes)
	if err != nil {
		return fmt.Errorf("failed to detect image format: %w", err)
	}

	base := filepath.Base(imageURL)
	ext := strings.ToLower(filepath.Ext(base))
	name := strings.TrimSuffix(base, ext)

	// pad the image filename to 3 digits
	padedFileName := padFileName(name + ".jpg")

	// join teh padded dir / filename back together
	outputFile := filepath.Join(targetDir, padedFileName)

	// If already JPEG, just save raw bytes directly
	if format == "jpeg" {
		err = os.WriteFile(outputFile, imgBytes, 0644)
		if err != nil {
			return fmt.Errorf("failed to save jpeg image: %w", err)
		}
		return nil
	}

	// Decode image according to detected format
	var img image.Image

	switch format {
	case "png", "gif":
		img, _, err = image.Decode(bytes.NewReader(imgBytes))
		if err != nil {
			return fmt.Errorf("failed to decode image: %w", err)
		}
	case "webp":
		img, err = webp.Decode(bytes.NewReader(imgBytes))
		if err != nil {
			return fmt.Errorf("failed to decode webp image: %w", err)
		}
	default:
		return fmt.Errorf("unsupported image format: %s", format)
	}

	// Convert and save as JPG
	outFile, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	opts := jpeg.Options{Quality: 90}
	err = jpeg.Encode(outFile, img, &opts)
	if err != nil {
		return fmt.Errorf("failed to encode jpeg: %w", err)
	}

	return nil
}

// detectImageFormat reads the magic bytes and returns the image format string (like "jpeg", "png", "webp")
func DetectImageFormat(data []byte) (string, error) {
	if len(data) < 12 {
		return "", errors.New("data too short to determine format")
	}

	if bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}) {
		return "jpeg", nil
	}
	if bytes.HasPrefix(data, []byte{0x89, 0x50, 0x4E, 0x47}) {
		return "png", nil
	}
	if bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")) {
		return "gif", nil
	}
	if bytes.HasPrefix(data, []byte("RIFF")) && bytes.HasPrefix(data[8:], []byte("WEBP")) {
		return "webp", nil
	}

	return "", errors.New("unknown image format")
}

// func to pad the filename to 3 digits, the inputFileName must be a filename.ext and the filename must be string
// representation of a digit.
// the input filename will be an integer.jpg (or with some image extenstion), note the input fiel name must have an
// extension
func padFileName(inputFileName string) string {
	var outputFileName string

	if strings.Contains(inputFileName, ".") {
		// split the filename on the . to separate the extension while padding
		parts := strings.SplitN(inputFileName, ".", 2)

		// convert the fielname string to an integer
		fileNamePart, err := strconv.Atoi(parts[0])
		if err != nil {
			log.Printf("padFileName() - error when converting filename integer %v", err)
		}
		// pad the resulting integer
		padded := fmt.Sprintf("%03d", fileNamePart)

		// craete the final filename
		outputFileName = padded + "." + parts[1]

	} else {
		log.Fatal("padFileName() - inputFilename must contain an extension eg: filename.ext")
	}

	return outputFileName
}

// check if the chapter file already exists in the current directory
// Returns true if exists, false otherwise. Returns error if unexpected.
// avoid re-downloading chapters
func FileExists(fileName string) (bool, error) {

	filePath := filepath.Join(".", fileName)

	_, err := os.Stat(filePath)
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

// create cbz file from source directory that ONLY contains image files
// imput sourceDir is scanned and sorted to add files to cbz in order
// note it is expected that the soureDir is the temp dir that ONLY contains image files
func CreateCbzFromDir(sourceDir, zipName string) error {
	// Read all directory entries
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	// Collect all file names (skip directories)
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}

	// Sort files alphabetically for ordered inclusion
	sort.Strings(files)

	// Create output cbz (zip) file
	zipFile, err := os.Create(zipName)
	if err != nil {
		return fmt.Errorf("parser.CreateCbzFromDir() - failed to create cbz file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Add each file to the zip archive
	for _, file := range files {
		filePath := filepath.Join(sourceDir, file)

		err := func() error {
			f, err := os.Open(filePath)
			if err != nil {
				return err
			}
			defer f.Close()

			w, err := zipWriter.Create(file)
			if err != nil {
				return err
			}

			_, err = io.Copy(w, f)
			return err
		}()
		if err != nil {
			return fmt.Errorf("error adding %s to cbz: %w", filePath, err)
		}
	}

	return nil
}

// filters out any non *.cbz file from the list
func FilterCBZFiles(files []string) []string {
	var filtered []string
	for _, f := range files {
		if strings.EqualFold(filepath.Ext(f), ".cbz") {
			filtered = append(filtered, f)
		}
	}
	return filtered
}
