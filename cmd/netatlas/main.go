package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stdout, "NetAtlas")
	fmt.Fprintln(os.Stdout, "Current collector binary: dnslog")
	fmt.Fprintln(os.Stdout, "Run `dnslog help` for DNS ingestion commands.")
}
