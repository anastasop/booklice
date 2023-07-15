# Booklice

Booklice is an indexer for pdf files. It scans directories, finds the pdfs and records in an sqlite3 database the full text of each pdf and the first page. The user can submit full text queries to the database to find related pdfs. The first page helps to check the pdf if the response snippet is not very informative.

## Example

This is a usage example. The `$` is the shell prompt. The response is a sequence of snippets, here only 2 are displayed for the shake of space. Each snippet is the pdf id, the path name when indexed and the result snippet for this pdf.
```
$ booklice add /home/anastasop/pdfs
$ booklice search golang
[894] /media/anastasop/Work/Suitcase/202303/pdf/Go at Google_ Language Design in the Service of Software Engineering - The Go Programming Language.pdf (#14)
...Language Design in the Service of
            Software Engineering
               Rob Pike
               Google, Inc.
               @rob_pike
               http://golang...

[996] /media/anastasop/Work/Suitcase/OneDrive/Code/GopherCon 2018 Lightning Talk_ go test in qemu.pdf (#79)
     go test -run=InQemu
            Lightning Talk
         Aug 2018, GopherCon
           Brad Fitzpatrick
         @bradfitz, @golang.org
  Oh, you...
$ ./booklice cover 996
# opens evince with the first page of the pdf file with id 996
```

## Installation

Booklice needs go >= 1.9 and ghostscript. If you are on a linux you already have ghostscript installed. For go check [here](http://golang.org/dl). To view pdf pages, it uses `evince` but you can select alternative viewers with the `-v` option, for example `./booklice cover -v gv 912`.

`go install --tags fts5 github.com/anastasop/booklice@latest`

Boolice has a dependency on the sqlite3 driver https://github.com/mattn/go-sqlite3 which is a cgo driver. If the installation fails then probably you should install the sqlite3 driver manually and then install booklice.

## License

Booklice is released under the GNU public license version 3.

## Bugs

- Difficult to get the defaults right. Should `list` enclose query in `%%` or expect the user to do it? Should search display the terms in bold or not? Should save the cover of each pdf by default or make this a flag?
- Not very comfortable to use directly from shell. You need to integrate it with your editor.
