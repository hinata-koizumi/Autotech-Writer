package sanitizer

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var (
	reComment = regexp.MustCompile(`(?m)(^|[^\\])%.*?$`)
	reBib     = regexp.MustCompile(`(?s)\\begin\{thebibliography\}.*?\\end\{thebibliography\}`)
	reAck     = regexp.MustCompile(`(?s)\\section\*?\{Acknowledgments?\}.*?(\n\\section|$)`)
	reImport  = regexp.MustCompile(`(?m)\\(?:input|include)\s*\{([^}]+)\}`)
	reNewCmd  = regexp.MustCompile(`\\newcommand\*?(?:\{?\\[a-zA-Z]+\}?|\\[a-zA-Z]+)(?:\[\d+\])?\{([^}]+)\}`)
	reMacro   = regexp.MustCompile(`\\newcommand\*?\{?(\\[a-zA-Z]+)\}?`)
	reDef     = regexp.MustCompile(`\\def(\\[a-zA-Z]+)\{([^}]+)\}`)
)

// ProcessArxivLatex fetches and processes LaTeX source for a given arXiv entry.
func ProcessArxivLatex(ctx context.Context, client *http.Client, baseURL, entryID string, fetchFunc func(context.Context, *http.Client, string) ([]byte, error)) (string, error) {
	parts := strings.Split(entryID, "/")
	id := parts[len(parts)-1]

	// Use a 30s timeout for the entire fetch and process operation
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	url := baseURL + "/e-print/" + id
	body, err := fetchFunc(ctx, client, url)
	if err != nil {
		return "", err
	}

	gzReader, err := gzip.NewReader(io.NopCloser(strings.NewReader(string(body))))
	if err != nil {
		return "", err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	files := make(map[string]string)
	var mainFile string

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		if strings.HasSuffix(header.Name, ".tex") || strings.HasSuffix(header.Name, ".ltx") || strings.HasSuffix(header.Name, ".bbl") {
			content, err := io.ReadAll(tarReader)
			if err != nil {
				continue
			}
			text := string(content)
			files[header.Name] = text

			if strings.Contains(text, "\\documentclass") {
				if mainFile == "" || strings.Contains(text, "\\begin{document}") {
					mainFile = header.Name
				}
			}
		}
	}

	if len(files) == 0 {
		return "", fmt.Errorf("no LaTeX files found in e-print")
	}
	if mainFile == "" {
		mainFile = findFallbackMainFile(files)
	}

	tex := resolveImports(files[mainFile], files, 0)
	tex = expandMacros(tex)

	if tex == "" {
		return "", nil
	}

	// Post-processing cleanup
	tex = reComment.ReplaceAllString(tex, "$1")
	tex = reBib.ReplaceAllString(tex, "")
	tex = reAck.ReplaceAllString(tex, "$1")

	return tex, nil
}

func findFallbackMainFile(files map[string]string) string {
	var maxLen int
	var mainFile string
	for name, content := range files {
		if len(content) > maxLen {
			maxLen = len(content)
			mainFile = name
		}
	}
	return mainFile
}

func resolveImports(content string, files map[string]string, depth int) string {
	if depth > 10 {
		return content
	}

	return reImport.ReplaceAllStringFunc(content, func(match string) string {
		sub := reImport.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		target := sub[1]
		if !strings.HasSuffix(target, ".tex") {
			target += ".tex"
		}

		importedContent, exists := files[target]
		if !exists {
			return match
		}
		return resolveImports(importedContent, files, depth+1)
	})
}

func expandMacros(content string) string {
	macros := make(map[string]string)

	matches := reNewCmd.FindAllString(content, -1)
	for _, match := range matches {
		nameSub := reMacro.FindStringSubmatch(match)
		valSub := reNewCmd.FindStringSubmatch(match)
		if len(nameSub) > 1 && len(valSub) > 1 {
			macros[nameSub[1]] = valSub[1]
		}
	}

	defMatches := reDef.FindAllStringSubmatch(content, -1)
	for _, sub := range defMatches {
		if len(sub) == 3 {
			macros[sub[1]] = sub[2]
		}
	}

	res := content
	for name, val := range macros {
		// Go's regexp doesn't support lookahead. Use capturing group for the follower.
		reCall := regexp.MustCompile(regexp.QuoteMeta(name) + `(?:\{\})?([^a-zA-Z]|$)`)
		res = reCall.ReplaceAllString(res, val+"$1")
	}

	return res
}
