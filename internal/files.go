package internal

import (
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"strings"

	"github.com/h2non/filetype"
)

func findScannerMatches(reader io.Reader) ([][]string, int, error) {
	matchedValues := make([][]string, len(regexRules)+1)
	nameIndex := len(regexRules)
	count := 0

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		v := scanner.Text()
		count += 1

		for i, rule := range regexRules {
			if rule.Regex.MatchString(v) {
				matchedValues[i] = append(matchedValues[i], v)
			}
		}

		tokens := tokenizer.Split(strings.ToLower(v), -1)
		if anyMatches(tokens) {
			matchedValues[nameIndex] = append(matchedValues[nameIndex], v)
		}
	}

	return matchedValues, count, nil
}

// TODO make more efficient
func zipReader(file io.Reader) (io.ReaderAt, int64) {
	data, err := io.ReadAll(file)
	if err != nil {
		abort(err)
	}
	bytesFile := bytes.NewReader(data)

	return bytesFile, int64(bytesFile.Size())
}

func processZip(file io.Reader) ([][]string, int, error) {
	matchedValues := make([][]string, len(regexRules)+1)
	count := 0

	readerAt, size := zipReader(file)
	reader, err := zip.NewReader(readerAt, size)
	if err != nil {
		return nil, 0, err
	}

	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}

		fileReader, err := file.Open()
		if err != nil {
			return nil, 0, err
		}
		defer fileReader.Close()

		fileMatchedValues, fileCount, err := processFile(fileReader)
		if err != nil {
			return nil, 0, err
		}

		// TODO capture specific file in archive
		for i := range matchedValues {
			matchedValues[i] = append(matchedValues[i], fileMatchedValues[i]...)
		}
		count += fileCount
	}

	return matchedValues, count, nil
}

func processGzip(file io.Reader) ([][]string, int, error) {
	gz, err := gzip.NewReader(file)
	if err != nil {
		return nil, 0, err
	}

	return findScannerMatches(gz)
}

func processFile(file io.Reader) ([][]string, int, error) {
	reader := bufio.NewReader(file)

	// we only have to pass the file header = first 261 bytes
	head, err := reader.Peek(261)
	if err != nil && err != io.EOF {
		return nil, 0, err
	}

	kind, err := filetype.Match(head)
	if err == filetype.ErrEmptyBuffer {
		matchedValues := make([][]string, len(regexRules)+1)
		count := 0
		return matchedValues, count, nil
	} else if err != nil {
		return nil, 0, err
	}
	// fmt.Println(kind.MIME.Value)

	// skip binary
	// TODO better method of detection
	if kind.MIME.Type == "video" || kind.MIME.Value == "application/x-bzip2" {
		matchedValues := make([][]string, len(regexRules)+1)
		count := 0
		return matchedValues, count, nil
		// } else if kind.MIME.Value == "application/pdf" {
		// 	return processPdf(file)
	} else if kind.MIME.Value == "application/zip" {
		return processZip(reader)
	} else if kind.MIME.Value == "application/gzip" {
		return processGzip(reader)
	}

	return findScannerMatches(reader)
}
