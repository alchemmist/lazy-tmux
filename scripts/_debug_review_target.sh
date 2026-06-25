#!/bin/sh
# Throwaway target so the Claude review has something obvious to comment on.
# Deliberately sloppy: unquoted variable in rm, no error handling.
clean() {
    dir=$1
    rm -rf $dir/*
}
clean "$1"
