package main

// searchResult is one ranked hit from a vault semantic search. Filled in by
// Task 24; the type exists here so model.go compiles.
type searchResult struct {
	Path      string
	StartLine int
	EndLine   int
	Score     float32
	Snippet   string
}
