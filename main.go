package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/peterbourgon/ff/v3/ffcli"
)

const (
	progName = "booklice"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("")

	rootFs := flag.NewFlagSet("rootFlags", flag.ExitOnError)
	dbName := rootFs.String("n", "main.db", "database. Created in .config. May use absolute paths like ./test.db")
	gsName := rootFs.String("e", "gs", "ghostscript executable. Must be in PATH")
	rootCmd := &ffcli.Command{
		Name:       progName,
		ShortUsage: progName + " [flags] subcommand [flags] <arguments>...",
		ShortHelp:  progName + " indexes pdf files",
		LongHelp:   progName + " indexes pdf files and builds a full text search index for their contents. Also it stores, and can display, the cover of each pdf.",
		FlagSet:    rootFs,
		Exec: func(ctx context.Context, args []string) error {
			return flag.ErrHelp
		},
	}

	addCmd := &ffcli.Command{
		Name:       "add",
		ShortUsage: "add paths...",
		ShortHelp:  "Add adds the pdfs at paths to the index",
		LongHelp:   "Add adds the pdfs at paths to the index. If path is a directory, it walks in it and adds all pdfs found.",
		Exec: func(ctx context.Context, args []string) error {
			if len(args) == 0 {
				return flag.ErrHelp
			}
			for _, path := range args {
				if err := addPath(path); err != nil {
					return fmt.Errorf("failed to add path %q: %w", path, err)
				}
			}
			return nil
		},
	}

	coverFs := flag.NewFlagSet("coverFlags", flag.ExitOnError)
	coverViewer := coverFs.String("v", "evince", "the pdf viewer to use. Must be on PATH")
	coverCmd := &ffcli.Command{
		Name:       "cover",
		ShortUsage: "cover [flags] name",
		ShortHelp:  "Show cover of pdf by id",
		LongHelp:   "Show cover of pdf by id.",
		FlagSet:    coverFs,
		Exec: func(ctx context.Context, args []string) error {
			if len(args) != 1 {
				return flag.ErrHelp
			}
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return flag.ErrHelp
			}
			if err := showCover(id, *coverViewer); err != nil {
				return fmt.Errorf("failed to display doc %d: %w", id, err)
			}
			return nil
		},
	}

	searchFs := flag.NewFlagSet("searchFlags", flag.ExitOnError)
	matchInBold := searchFs.Bool("b", true, "Show matches in bold. Needs ANSI terminal")
	docsToFetch := searchFs.Int("n", 10, "Fetch at most n documents")
	namesOnly := searchFs.Bool("t", false, "Show pdf names only")
	searchCmd := &ffcli.Command{
		Name:       "search",
		ShortUsage: "search [flags] query",
		ShortHelp:  "Search pdfs for terms",
		LongHelp:   "Search pdfs for terms. Check https://www.sqlite.org/fts5.html for query details. For each document display the id to be used with cover, the path of the file and the snippet with the term",
		FlagSet:    searchFs,
		Exec: func(ctx context.Context, args []string) error {
			if len(args) != 1 {
				return flag.ErrHelp
			}
			query := args[0]
			if err := search(query, *docsToFetch, *namesOnly, os.Stdout, *matchInBold); err != nil {
				return fmt.Errorf("failed to search for %q: %w", query, err)
			}
			return nil
		},
	}

	listFs := flag.NewFlagSet("listFlags", flag.ExitOnError)
	listCmd := &ffcli.Command{
		Name:       "list",
		ShortUsage: "list [flags] expr..",
		ShortHelp:  "List pdfs for paths matching sql like expressions",
		LongHelp:   "List pdfs for paths matching sql like expressions",
		FlagSet:    listFs,
		Exec: func(ctx context.Context, args []string) error {
			for _, expr := range args {
				if err := list(expr, os.Stdout); err != nil {
					return fmt.Errorf("failed to list for %q: %w", expr, err)
				}
			}
			return nil
		},
	}

	rootCmd.Subcommands = []*ffcli.Command{addCmd, coverCmd, searchCmd, listCmd}

	if err := rootCmd.Parse(os.Args[1:]); err != nil {
		log.Fatal(err)
	}

	if p, err := exec.LookPath(*gsName); err != nil {
		log.Fatal(err)
	} else {
		gsExe = p
	}

	dbPath, err := pathFromName(*dbName)
	if err != nil {
		log.Fatal(err)
	}

	openDatabase(dbPath)
	defer closeDatabase()

	if err := rootCmd.Run(context.Background()); err != nil {
		log.Println(err)
	}
}

// addPDF add the pdf file to the index
func addPDF(path string) error {
	if !strings.HasSuffix(path, ".pdf") && !strings.HasSuffix(path, ".PDF") {
		return nil
	}

	var (
		contents, cover                         []byte
		pages                                   int
		sig                                     string
		contentsErr, coverErr, pagesErr, sigErr error
	)

	pdf, err := newPDF(path)
	if err != nil {
		return fmt.Errorf("failed to read %q: %w", path, err)
	}
	sig, sigErr = pdf.Sig()
	if sigErr != nil {
		return sigErr
	}
	var exists int
	if err := existsStmt.QueryRow(sig).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check existence %q: %w", path, err)
	}
	if exists > 0 {
		log.Printf("Duplicate: %s", path)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		contents, contentsErr = pdf.FullText(ctx)
	}()
	go func() {
		defer wg.Done()
		cover, coverErr = pdf.Cover(ctx)
	}()
	go func() {
		defer wg.Done()
		pages, pagesErr = pdf.Pages(ctx)
	}()
	wg.Wait()

	if contentsErr != nil {
		return contentsErr
	}
	if coverErr != nil {
		return coverErr
	}
	if pagesErr != nil {
		return pagesErr
	}

	tl, err := title(bytes.NewBuffer(cover))
	if err != nil {
		log.Printf("title error %s: %v", path, err)
	}

	_, err = insertStmt.Exec(path, pages, sig, contents, tl, cover, time.Now())
	return err
}

// showCover displays the cover of pdf with id. The viewer must be on $PATH
func showCover(id int, viewer string) error {
	var res sql.RawBytes

	rows, err := coverStmt.Query(id)
	if err != nil {
		return err
	}
	defer rows.Close()

	if !rows.Next() {
		return fmt.Errorf("pdf with id %d not found", id)
	}

	if err := rows.Scan(&res); err != nil {
		return err
	}

	fout, err := os.CreateTemp("", progName+"-*.pdf")
	if err != nil {
		return err
	}
	defer fout.Close()
	defer os.Remove(fout.Name())

	if _, err := fout.Write(res); err != nil {
		return err
	}

	vpath, err := exec.LookPath(viewer)
	if err != nil {
		return err
	}
	return exec.Command(vpath, fout.Name()).Run()
}

// search queries the index for pdfs, fetches at most docsToFetch and writes snippets to w
// If w is an ANSI terminal use matchInBold to display the matched term in bold
func search(query string, docsToFetch int, namesOnly bool, w io.Writer, matchInBold bool) error {
	rows, err := searchStmt.Query(query, docsToFetch)
	if err != nil {
		return fmt.Errorf("search for %q failed: %w", query, err)
	}
	defer rows.Close()

	repl := strings.NewReplacer("{{{", "\033[1m", "}}}", "\033[0m")
	for rows.Next() {
		var (
			id      int
			title   string
			name    string
			pages   int
			snippet string
		)
		if err := rows.Scan(&id, &title, &name, &pages, &snippet); err != nil {
			return fmt.Errorf("search for %q failed, can't scan row: %w", query, err)
		}

		if namesOnly {
			fmt.Fprintf(w, "[%d] %s (#%d)\n", id, name, pages)
		} else {
			if matchInBold {
				snippet = repl.Replace(snippet)
			}
			fmt.Fprintf(w, "[%d] %s (#%d)\nTitle: %s\n%s\n\n", id, name, pages, title, snippet)
		}
	}
	if err := rows.Err(); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("search for %q failed, can't fetch rows: %w", query, err)
	}

	return nil
}

// list queries the index for pdfs with paths matching (sql like) expression
func list(expr string, w io.Writer) error {
	rows, err := listStmt.Query(expr)
	if err != nil {
		return fmt.Errorf("like for %q failed: %w", expr, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id    int
			title string
			name  string
			pages int
		)
		if err := rows.Scan(&id, &title, &name, &pages); err != nil {
			return fmt.Errorf("list for %q failed, can't scan row: %w", expr, err)
		}

		fmt.Fprintf(w, "[%d] %s (#%d)\nTitle: %s\n", id, name, pages, title)
	}
	if err := rows.Err(); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("list for %q failed, can't fetch rows: %w", expr, err)
	}

	return nil
}

// addPath adds the files at path to index. If path is a dir it is recursively scanned for pdfs.
// During scanning dirs it just logs errors and continues to add as much files as possible.
func addPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to add %q: %w", path, err)
	}
	if info.IsDir() {
		return filepath.WalkDir(path, scanFunc)
	}
	return addPDF(path)
}

func scanFunc(path string, d fs.DirEntry, err error) error {
	if err != nil {
		log.Printf("walk error %s: %v", path, err)
		return nil
	}
	if d.IsDir() {
		return nil
	}
	if err := addPDF(path); err != nil {
		log.Printf("add error %s: %v", path, err)
		return nil
	}
	return nil
}

// pathFromName returns a db path for name. If name contains a slash, it is returned as is,
// otherwise a dir with this name is created in user's config dir (see os.UserConfigDir)
func pathFromName(name string) (string, error) {
	if strings.Contains(name, string(filepath.Separator)) {
		return name, nil
	}

	cfgPath, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	if err := os.Mkdir(filepath.Join(cfgPath, progName), 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	return filepath.Join(cfgPath, progName, name), nil
}
