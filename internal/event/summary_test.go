package event

import (
	"encoding/json"
	"testing"
)

func TestShouldSummarizeToolCalls(t *testing.T) {
	cases := []struct {
		name, tool, input, wantActivity, wantFile string
	}{
		{"bash prefers description", "Bash", `{"command":"mvn -q verify","description":"Run tests"}`, "Bash: Run tests", ""},
		{"bash first line of command", "Bash", `{"command":"pio run -t upload\necho done"}`, "Bash: pio run -t upload", ""},
		{"edit uses basename", "Edit", `{"file_path":"/a/b/AuthResource.java"}`, "Edit AuthResource.java", "/a/b/AuthResource.java"},
		{"write uses basename", "Write", `{"file_path":"/x/main.go","content":"..."}`, "Write main.go", "/x/main.go"},
		{"multiedit", "MultiEdit", `{"file_path":"/x/a.ts"}`, "Edit a.ts", "/x/a.ts"},
		{"notebook", "NotebookEdit", `{"notebook_path":"/x/n.ipynb"}`, "Edit n.ipynb", "/x/n.ipynb"},
		{"read is not an edit", "Read", `{"file_path":"/x/pom.xml"}`, "Read pom.xml", ""},
		{"grep", "Grep", `{"pattern":"TODO"}`, "Grep TODO", ""},
		{"glob", "Glob", `{"pattern":"**/*.go"}`, "Glob **/*.go", ""},
		{"webfetch host", "WebFetch", `{"url":"https://zellij.dev/documentation/x"}`, "Fetch zellij.dev", ""},
		{"websearch", "WebSearch", `{"query":"ghostty applescript"}`, "Search ghostty applescript", ""},
		{"agent", "Agent", `{"description":"Find callers"}`, "Agent: Find callers", ""},
		{"task", "Task", `{"description":"Find callers"}`, "Agent: Find callers", ""},
		{"ask user question", "AskUserQuestion", `{"questions":[{"question":"Which version?"}]}`, "Which version?", ""},
		{"mcp tool", "mcp__claude_ai_Slack__slack_send_message", `{}`, "claude_ai_Slack: slack_send_message", ""},
		{"unknown tool", "Frobnicate", `{}`, "Frobnicate", ""},
		{"broken input", "Bash", `not json`, "Bash", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// when
			act, file := Summarize(c.tool, json.RawMessage(c.input))

			// then
			if act != c.wantActivity {
				t.Errorf("activity = %q, want %q", act, c.wantActivity)
			}
			if file != c.wantFile {
				t.Errorf("file = %q, want %q", file, c.wantFile)
			}
		})
	}
}

func TestShouldTruncateLongActivity(t *testing.T) {
	// given
	long := `{"command":"` + string(make([]byte, 0)) + "echo " + repeat("x", 200) + `"}`

	// when
	act, _ := Summarize("Bash", json.RawMessage(long))

	// then
	if n := len([]rune(act)); n > MaxActivity {
		t.Fatalf("len = %d, want <= %d", n, MaxActivity)
	}
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
