package main

import (
	"fmt"
	"os"

	"github.com/wI2L/jsondiff"
)

func main() {
	vuln1, _ := os.ReadFile("test-data/nginx-1.json")
	vuln2, _ := os.ReadFile("test-data/nginx-2.json")
	patch, _ := jsondiff.CompareJSON(vuln1, vuln2)
	fmt.Println(patch)
}
