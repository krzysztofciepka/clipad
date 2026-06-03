package main

type agentMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	ToolCalls  []agentToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type agentToolCall struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Function agentToolFunction `json:"function"`
}

type agentToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type agentTool struct {
	Type     string        `json:"type"`
	Function agentToolSpec `json:"function"`
}

type agentToolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// agentTools returns the tool schema advertised to the model.
func agentTools() []agentTool {
	return []agentTool{
		{Type: "function", Function: agentToolSpec{
			Name:        "bash",
			Description: "Run a bash command in the notes vault (cwd is the vault root). Use for managing files: cd, ls, mv, cp, cat, sed, awk, etc. Confined to the vault.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"cmd": map[string]any{
						"type":        "string",
						"description": "The bash command to run.",
					},
				},
				"required": []string{"cmd"},
			},
		}},
		{Type: "function", Function: agentToolSpec{
			Name:        "search_vault",
			Description: "Semantic search over the user's notes. Use to answer questions about note content. Returns numbered excerpts with file paths and line ranges to cite.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The search query.",
					},
					"k": map[string]any{
						"type":        "integer",
						"description": "Number of excerpts to return (default 8).",
					},
				},
				"required": []string{"query"},
			},
		}},
	}
}
