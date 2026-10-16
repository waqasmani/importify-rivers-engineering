package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

func init() { register(workflowAnalyzer{}) }

// WorkflowStep is one observed step of a business process.
type WorkflowStep struct {
	Actor    string   `json:"actor"`
	Action   string   `json:"action"`
	System   string   `json:"system"`
	Data     []string `json:"data"`
	Outcome  string   `json:"outcome"`
	Decision string   `json:"decision"`
}

// StepsFromMetadata reads structured steps stored on a workflow source, falling back to parsing
// "Actor: action -> outcome" lines from the text.
func StepsFromMetadata(meta map[string]any, text string) []WorkflowStep {
	var steps []WorkflowStep
	if raw, ok := meta["steps"]; ok {
		b, _ := json.Marshal(raw)
		_ = json.Unmarshal(b, &steps)
	}
	if len(steps) > 0 {
		return steps
	}
	lineRe := regexp.MustCompile(`^\s*(?:\d+[.)]\s*)?(?:([^:]{1,60}):\s*)?(.+?)(?:\s*(?:->|→)\s*(.+))?$`)
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		m := lineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		steps = append(steps, WorkflowStep{Actor: strings.TrimSpace(m[1]), Action: strings.TrimSpace(m[2]), Outcome: strings.TrimSpace(m[3])})
	}
	return steps
}

var mermaidUnsafe = strings.NewReplacer(`"`, "'", "[", "(", "]", ")", "{", "(", "}", ")", "<", "‹", ">", "›", "|", "/", "\n", " ", "#", "no.", ";", ",")

// MermaidLabel makes text safe inside a quoted Mermaid node label.
func MermaidLabel(s string) string { return clip(mermaidUnsafe.Replace(strings.TrimSpace(s)), 120) }

// WorkflowMermaid renders steps as a top-down flowchart with decision branches.
func WorkflowMermaid(steps []WorkflowStep) string {
	var b strings.Builder
	b.WriteString("flowchart TD\n")
	b.WriteString("  start([Start])\n")
	prev := "start"
	for i, s := range steps {
		id := fmt.Sprintf("s%d", i+1)
		label := s.Action
		if s.Actor != "" {
			label = s.Actor + ": " + s.Action
		}
		if s.System != "" {
			label += " (" + s.System + ")"
		}
		if s.Decision != "" {
			fmt.Fprintf(&b, "  %s{\"%s\"}\n", id, MermaidLabel(s.Decision))
			fmt.Fprintf(&b, "  %s --> %s\n", prev, id)
			fmt.Fprintf(&b, "  %s_do[\"%s\"]\n", id, MermaidLabel(label))
			fmt.Fprintf(&b, "  %s -- yes --> %s_do\n", id, id)
			prev = id + "_do"
			continue
		}
		fmt.Fprintf(&b, "  %s[\"%s\"]\n", id, MermaidLabel(label))
		fmt.Fprintf(&b, "  %s --> %s\n", prev, id)
		prev = id
	}
	b.WriteString("  finish([End])\n")
	fmt.Fprintf(&b, "  %s --> finish\n", prev)
	return b.String()
}

// SequenceMermaid renders steps between actors and systems as a sequence diagram.
func SequenceMermaid(steps []WorkflowStep) string {
	var b strings.Builder
	b.WriteString("sequenceDiagram\n")
	for _, s := range steps {
		actor := firstNonEmpty(s.Actor, "User")
		target := firstNonEmpty(s.System, "System")
		fmt.Fprintf(&b, "  %s->>%s: %s\n", mermaidID(actor), mermaidID(target), MermaidLabel(s.Action))
		if s.Outcome != "" {
			fmt.Fprintf(&b, "  %s-->>%s: %s\n", mermaidID(target), mermaidID(actor), MermaidLabel(s.Outcome))
		}
	}
	return b.String()
}

var idRe = regexp.MustCompile(`[^A-Za-z0-9_]`)

func mermaidID(s string) string {
	id := idRe.ReplaceAllString(strings.TrimSpace(s), "_")
	if id == "" || (id[0] >= '0' && id[0] <= '9') {
		id = "n_" + id
	}
	return id
}

type workflowAnalyzer struct{}

func (workflowAnalyzer) Name() string { return "workflow" }

func (workflowAnalyzer) Analyze(ctx context.Context, in Input) (Result, error) {
	b := newBuilder()
	steps := StepsFromMetadata(in.Metadata, in.Text)
	if len(steps) == 0 {
		b.unknown("workflow_step", "No workflow steps were provided", "Describe the observed process as steps (actor, action, system, data, outcome).")
		return Result{Findings: b.out, Stats: map[string]any{"steps": 0}}, nil
	}
	actors, systems, entities := map[string]int{}, map[string]int{}, map[string]int{}
	for i, s := range steps {
		ev := fmt.Sprintf("Actor: %s\nAction: %s", firstNonEmpty(s.Actor, "(not stated)"), s.Action)
		if s.System != "" {
			ev += "\nSystem: " + s.System
		}
		if len(s.Data) > 0 {
			ev += "\nData: " + strings.Join(s.Data, ", ")
		}
		if s.Outcome != "" {
			ev += "\nOutcome: " + s.Outcome
		}
		if s.Decision != "" {
			ev += "\nDecision: " + s.Decision
		}
		b.verified("workflow_step", fmt.Sprintf("Step %d: %s", i+1, clip(s.Action, 200)), "Recorded by the analyst as an observed step.",
			ev, fmt.Sprintf("step %d", i+1), map[string]any{"index": i + 1, "step": s})
		if s.Actor != "" {
			actors[s.Actor]++
		}
		if s.System != "" {
			systems[s.System]++
		}
		for _, d := range s.Data {
			if d = strings.TrimSpace(d); d != "" {
				entities[d]++
			}
		}
		if s.Actor != "" && s.Action != "" {
			req := fmt.Sprintf("The system must allow %s to %s.", strings.ToLower(s.Actor), lowerFirst(strings.TrimSuffix(s.Action, ".")))
			b.inferred("requirement", clip(req, 300), "Derived from workflow step "+fmt.Sprint(i+1)+".", ev, fmt.Sprintf("step %d", i+1),
				"Assumes the observed step should be supported by the reconstructed system.", 0.7, map[string]any{"step": i + 1})
		}
	}
	if len(actors) > 0 {
		b.verified("actor", "Actors ("+plural(len(actors), "actor")+")", "", strings.Join(sortedKeys(actors), "\n"), "steps", map[string]any{"actors": sortedKeys(actors)})
	}
	if len(systems) > 0 {
		b.verified("component", "Systems involved ("+plural(len(systems), "system")+")", "", strings.Join(sortedKeys(systems), "\n"), "steps", map[string]any{"systems": sortedKeys(systems)})
	}
	for _, e := range sortedKeys(entities) {
		b.inferred("entity", "Entity: "+e, fmt.Sprintf("Data item used in %d step(s).", entities[e]), e, "steps",
			"Data named in a step is assumed to be a persistent entity; it may only be a transient value.", 0.6, map[string]any{"name": e})
	}
	b.verified("diagram", "Workflow diagram", "Mermaid flowchart generated from the recorded steps.", WorkflowMermaid(steps), "steps",
		map[string]any{"mermaid": WorkflowMermaid(steps), "sequence": SequenceMermaid(steps)})
	return Result{Findings: b.out, Stats: map[string]any{"steps": len(steps), "actors": len(actors), "entities": len(entities)}}, nil
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
