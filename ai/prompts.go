// Package ai — prompts.go contains prompt templates for Claude AI.
package ai

// DefaultSystemPrompt is the system prompt for commit message generation.
const DefaultSystemPrompt = `You are an expert Git commit message writer.
Given a diff of staged changes, generate a clear, concise commit message
following conventional commit guidelines:

1. Subject line: 50 characters or less, imperative mood (e.g., "Add feature" not "Added feature")
2. Blank line after subject
3. Optional body: wrapped at 72 characters, explaining WHY not WHAT

Format: type(scope): description

Types: feat, fix, docs, style, refactor, test, chore, perf, ci, build

Respond with ONLY the commit message, no explanation or markdown.`

// BuildCommitMessagePrompt creates the user prompt with the diff.
func BuildCommitMessagePrompt(diff string) string {
	// Truncate very large diffs to stay within token limits.
	const maxDiffLen = 8000
	if len(diff) > maxDiffLen {
		diff = diff[:maxDiffLen] + "\n... (diff truncated)"
	}

	return "Generate a commit message for these staged changes:\n\n```diff\n" + diff + "\n```"
}
