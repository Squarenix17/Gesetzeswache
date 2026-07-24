package fixtures

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"runtime"
)

// Dir returns the directory containing this package's fixture files.
func Dir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("fixtures: cannot determine directory")
	}
	return filepath.Dir(file)
}

// MustRead loads a fixture file from Dir() and panics on error.
func MustRead(name string) []byte {
	data, err := os.ReadFile(filepath.Join(Dir(), name))
	if err != nil {
		panic(err)
	}
	return data
}

// MustZipXML builds an in-memory zip archive containing a single XML entry.
func MustZipXML(entryName string, xml []byte) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(entryName)
	if err != nil {
		panic(err)
	}
	if _, err := w.Write(xml); err != nil {
		panic(err)
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
