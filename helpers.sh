#!/usr/bin/bash

# bash functions to use ghostscript from shell
# Usage: . ./helpers.sh

function text {
    gs -dNOPAUSE -dBATCH -dSAFER -dQUIET -sDEVICE=txtwrite -sOutputFile=- "$1"
}

function cover {
    gs -dNOPAUSE -dBATCH -dSAFER -dQUIET -sDEVICE=pdfwrite -sOutputFile=- -dFirstPage=1 -dLastPage=1 "$1"
}

function pages {
    gs -dNOPAUSE -dBATCH -dSAFER -dQUIET -dNODISPLAY --permit-file-read="$1" -c "($1) (r) file runpdfbegin pdfpagecount = quit"
}
