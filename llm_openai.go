package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
)

// openaiStream talks to an OpenAI-compatible /chat/completions endpoint (DeepSeek, etc).
func openaiStream(ctx context.Context, c aiConf, msgs []oMsg, tools []oTool, onDelta func(string)) (oMsg, error) {
	type fn struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}
	type tc struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function fn     `json:"function"`
	}
	type msg struct {
		Role             string `json:"role"`
		Content          string `json:"content"`
		ReasoningContent string `json:"reasoning_content,omitempty"`
		ToolCalls        []tc   `json:"tool_calls,omitempty"`
		ToolCallID       string `json:"tool_call_id,omitempty"`
	}
	wire := make([]msg, 0, len(msgs))
	for _, m := range msgs {
		w := msg{Role: m.Role, Content: m.Content, ReasoningContent: m.Reasoning, ToolCallID: m.ToolCallID}
		for _, c := range m.ToolCalls {
			w.ToolCalls = append(w.ToolCalls, tc{ID: c.ID, Type: "function", Function: fn{Name: c.Function.Name, Arguments: string(c.Function.Arguments)}})
		}
		wire = append(wire, w)
	}
	req0 := map[string]any{
		"model":       c.Model,
		"messages":    wire,
		"stream":      true,
		"temperature": 0.4,
	}
	if len(tools) > 0 {
		req0["tools"] = tools
	}
	body, _ := json.Marshal(req0)
	url := strings.TrimRight(c.URL, "/") + "/chat/completions"
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if c.Key != "" {
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}
	out := oMsg{Role: "assistant"}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return out, errors.New("ai_unreachable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		var e struct {
			Error struct{ Message string } `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&e)
		if e.Error.Message == "" {
			e.Error.Message = resp.Status
		}
		return out, errors.New("ai: " + e.Error.Message)
	}

	type partial struct {
		id, name string
		args     strings.Builder
	}
	calls := map[int]*partial{}
	var text, reasoning strings.Builder
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
					ToolCalls        []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function fn     `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil || len(chunk.Choices) == 0 {
			continue
		}
		d := chunk.Choices[0].Delta
		reasoning.WriteString(d.ReasoningContent)
		if d.Content != "" {
			text.WriteString(d.Content)
			onDelta(d.Content)
		}
		for _, c := range d.ToolCalls {
			p := calls[c.Index]
			if p == nil {
				p = &partial{}
				calls[c.Index] = p
			}
			if c.ID != "" {
				p.id = c.ID
			}
			if c.Function.Name != "" {
				p.name = c.Function.Name
			}
			p.args.WriteString(c.Function.Arguments)
		}
	}
	out.Content, out.Reasoning = text.String(), reasoning.String()
	idx := make([]int, 0, len(calls))
	for i := range calls {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	for _, i := range idx {
		p := calls[i]
		var c oToolCall
		c.ID, c.Function.Name = p.id, p.name
		args := p.args.String()
		if !json.Valid([]byte(args)) {
			args = "{}"
		}
		c.Function.Arguments = json.RawMessage(args)
		out.ToolCalls = append(out.ToolCalls, c)
	}
	if err := sc.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return out, err
	}
	return out, nil
}
