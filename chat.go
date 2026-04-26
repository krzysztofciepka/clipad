package main

// chatModeT, chatTurn, citation are filled in by Phase 5 tasks. Stubs here
// so model.go compiles in Phase 3.

type chatModeT int

const (
	chatModeInput chatModeT = iota
	chatModeView
)

type chatTurn struct {
	Role      string // "user" | "assistant"
	Content   string
	Citations []citation
}

type citation struct {
	Path      string
	StartLine int
	EndLine   int
}
