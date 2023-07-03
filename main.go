/*
Command har-extractor provides a streaming HAR file parser, which can extract
and write response content to disk. It preserves directory structure.

Usage:

	$ har-extractor -o /path/to/output <harfiles...>

Options:

	-allowed-hosts string
	      Comma-separated list of hosts to allow (e.g. "example.com,example.org")
	-dry-run
	      Enable dry run mode
	-o string
	      Output directory (short) (default ".")
	-output string
	      Output directory (default ".")
	-r    Remove query string from file path (short)
	-remove-query-string
	      Remove query string from file path
	-verbose
	      Show processing file path
*/
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Content struct {
	Size        int    `json:"size"`
	MimeType    string `json:"mimeType"`
	Text        string `json:"text"`
	Compression int    `json:"compression"`
	Encoding    string `json:"encoding"`
}

type Response struct {
	Status  int     `json:"status"`
	Content Content `json:"content"`
}

type Request struct {
	Method string `json:"method"`
	URL    string `json:"url"`
}

type Entry struct {
	Request  Request  `json:"request"`
	Response Response `json:"response"`
}

func safeFileName(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' {
			return '-'
		}
		return r
	}, s)
}

func processHar(reader io.Reader, rootDir string, removeQueryString bool, dryRun bool, verbose bool, hostAllowlist map[string]bool) (int, error) {
	var count int
	decoder := json.NewDecoder(reader)

	// Read until the "entries" key
	for {
		token, err := decoder.Token()
		if err != nil {
			return count, err
		}

		if key, ok := token.(string); ok && key == "entries" {
			// Break the loop if the key is "entries"
			break
		}
	}

	// Expect the next token to be the opening bracket [
	if _, err := decoder.Token(); err != nil {
		return count, err
	}

	for decoder.More() {
		var entry Entry
		if err := decoder.Decode(&entry); err != nil {
			return count, err
		}

		if err := processEntry(entry, rootDir, removeQueryString, dryRun, verbose, hostAllowlist); err != nil {
			return count, err
		}

		count++
	}

	// Expect the next token to be the closing bracket ]
	if _, err := decoder.Token(); err != nil {
		return count, err
	}

	return count, nil
}

func processEntry(entry Entry, rootDir string, removeQueryString bool, dryRun bool, verbose bool, hostAllowlist map[string]bool) error {
	parsedUrl, err := url.Parse(entry.Request.URL)
	if err != nil {
		return err
	}

	if len(hostAllowlist) > 0 {
		if !hostAllowlist[parsedUrl.Host] {
			return nil
		}
	}

	if removeQueryString {
		parsedUrl.RawQuery = ""
	}

	dirPath := filepath.Join(rootDir, parsedUrl.Host, filepath.Dir(parsedUrl.Path))

	if !dryRun {
		err = os.MkdirAll(dirPath, os.ModePerm)
		if err != nil {
			return err
		}
	}

	filePath := filepath.Join(dirPath, safeFileName(parsedUrl.Path))
	if verbose {
		fmt.Println("Processing: ", filePath)
	}

	if dryRun {
		return nil
	}

	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// handle base64 encoding
	if entry.Response.Content.Encoding == "base64" {
		var data []byte
		data, err = base64.StdEncoding.DecodeString(entry.Response.Content.Text)
		if err != nil {
			return err
		}
		_, err = file.Write(data)
	} else {
		_, err = file.WriteString(entry.Response.Content.Text)
	}

	if err == nil {
		err = file.Close()
	}

	return err
}

func main() {
	var output string
	var removeQueryString bool
	var dryRun bool
	var verbose bool
	var hostAllowlistStr string

	flag.StringVar(&output, "output", ".", "Output directory")
	flag.StringVar(&output, "o", ".", "Output directory (short)")
	flag.BoolVar(&removeQueryString, "remove-query-string", false, "Remove query string from file path")
	flag.BoolVar(&removeQueryString, "r", false, "Remove query string from file path (short)")
	flag.BoolVar(&dryRun, "dry-run", false, "Enable dry run mode")
	flag.BoolVar(&verbose, "verbose", false, "Show processing file path")
	flag.StringVar(&hostAllowlistStr, "allowed-hosts", "", "Comma-separated list of hosts to allow (e.g. \"example.com,example.org\")")

	flag.Parse()

	if flag.NArg() == 0 {
		fmt.Println("Please provide at least one HAR file to process")
		os.Exit(1)
	}

	hostAllowlist := make(map[string]bool)
	if hostAllowlistStr != "" {
		for _, host := range strings.Split(hostAllowlistStr, ",") {
			hostAllowlist[host] = true
		}
	}

	for _, harFilePath := range flag.Args() {
		file, err := os.Open(harFilePath)
		if err != nil {
			fmt.Println("Failed to open HAR file:", err)
			continue
		}

		var count int
		count, err = processHar(bufio.NewReader(file), output, removeQueryString, dryRun, verbose, hostAllowlist)
		_ = file.Close()
		if err != nil {
			fmt.Printf("Failed to process HAR file (%d entries processed): %s\n", count, err)
			continue
		}

		fmt.Printf("Successfully processed HAR file (%d entries processed): %s\n", count, harFilePath)
	}
}
