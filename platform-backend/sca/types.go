package sca

type Dependency struct {
	Name      string
	Version   string
	Language  string
	FilePath  string
	Locations []string // For deep scan, where it was found (e.g., inside which jar)
}

type ScannerPlugin interface {
	Name() string
	Scan(dir string) ([]Dependency, error)
}
