package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"strings"
	"text/template"
)

//go:embed static
var content embed.FS

var templateFuncs = template.FuncMap(map[string]any{
	"truncate": func(l int, s string) string { return s[0:min(len(s), l)] },
	"ifempty": func(dflt, s string) string {
		if s == "" {
			return dflt
		}
		return s
	},
})

var resultsTemplate = template.Must(template.New("results.tmpl").Funcs(templateFuncs).ParseFS(content, "static/results.tmpl"))

type OpenSearchResults struct {
	Query   string
	Results []SearchResult
}

type LinkResolver string

func (ls LinkResolver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.FormValue("q")

	results, err := search(query, 100)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "internal error: %v", err)
		return
	}

	repl := strings.NewReplacer("{{{", "<strong>", "}}}", "</strong>")
	for i := range results {
		if r, err := url.JoinPath(string(ls), results[i].Name); err == nil {
			results[i].URL = r
		} else {
			results[i].URL = string(ls)
		}
		results[i].Snippet = repl.Replace(results[i].Snippet)
	}

	w.Header().Set("Cache-Control", "no-cache")
	resultsTemplate.Execute(w, OpenSearchResults{query, results})
}

func startOpenSearchServer(listenAddr, announceAddr, resolveAddr string) error {
	cnt, err := fs.Sub(content, "static")
	if err != nil {
		log.Fatal(err)
	}

	if !strings.HasPrefix(resolveAddr, "http://") {
		resolveAddr = "http://" + resolveAddr
	}

	openSearchXML := strings.NewReplacer("@", announceAddr).Replace(openSearchTemplate)

	http.HandleFunc("GET /opensearch.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/opensearchdescription+xml")
		fmt.Fprintf(w, "%s", openSearchXML)
	})
	http.Handle("GET /search", LinkResolver(resolveAddr))
	http.Handle("GET /", http.FileServerFS(cnt))
	return http.ListenAndServe(listenAddr, nil)
}

const openSearchTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<OpenSearchDescription xmlns="http://a9.com/-/spec/opensearch/1.1/">
  <ShortName>booklice</ShortName>
  <Description>booklice documents search engine</Description>
  <InputEncoding>UTF-8</InputEncoding>
  <Image width="16" height="16" type="image/x-icon">/favicon.ico</Image>
  <Url type="text/html" template="http://@/search?q={searchTerms}" />
</OpenSearchDescription>
`
