package router

import "fmt"

type Request struct {
	Model    string    `json:"model"`
	Prompt   any       `json:"prompt"`
	Messages []Message `json:"messages"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (r Request) PrefixText(limit int) string {
	var out string
	if len(r.Messages) > 0 {
		for _, msg := range r.Messages {
			out += msg.Content
			if len(out) >= limit {
				return out[:limit]
			}
		}
		return out
	}

	switch prompt := r.Prompt.(type) {
	case string:
		out = prompt
	case []any:
		for _, part := range prompt {
			out += fmt.Sprint(part)
			if len(out) >= limit {
				return out[:limit]
			}
		}
	case nil:
		out = ""
	default:
		out = fmt.Sprint(prompt)
	}
	if len(out) > limit {
		return out[:limit]
	}
	return out
}
