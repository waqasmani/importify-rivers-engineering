package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

func init() {
	register(openAPIAnalyzer{})
	register(apiExamplesAnalyzer{})
}

type openAPIAnalyzer struct{}

func (openAPIAnalyzer) Name() string { return "openapi" }

// ParseStructured decodes JSON or YAML into generic maps with string keys.
func ParseStructured(text string) (map[string]any, error) {
	t := strings.TrimSpace(text)
	var v any
	if strings.HasPrefix(t, "{") {
		if err := json.Unmarshal([]byte(t), &v); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
	} else {
		if err := yaml.Unmarshal([]byte(t), &v); err != nil {
			return nil, fmt.Errorf("invalid YAML: %w", err)
		}
		v = normalizeYAML(v)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("the document is not an object")
	}
	return m, nil
}

func normalizeYAML(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			t[k] = normalizeYAML(x)
		}
		return t
	case map[any]any:
		out := map[string]any{}
		for k, x := range t {
			out[fmt.Sprint(k)] = normalizeYAML(x)
		}
		return out
	case []any:
		for i, x := range t {
			t[i] = normalizeYAML(x)
		}
	}
	return v
}

var httpMethods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}

func mapOf(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func refName(v any) string {
	m := mapOf(v)
	if r, ok := m["$ref"].(string); ok {
		return r[strings.LastIndex(r, "/")+1:]
	}
	if t, ok := m["type"].(string); ok {
		if t == "array" {
			return "array of " + refName(m["items"])
		}
		return t
	}
	return ""
}

func (openAPIAnalyzer) Analyze(ctx context.Context, in Input) (Result, error) {
	b := newBuilder()
	doc, err := ParseStructured(in.Text)
	if err != nil {
		return Result{}, err
	}
	version := str(doc["openapi"])
	if version == "" {
		version = str(doc["swagger"])
	}
	if version == "" {
		return Result{}, fmt.Errorf("the document has no openapi or swagger version field")
	}
	info := mapOf(doc["info"])
	b.verified("api", "API: "+firstNonEmpty(str(info["title"]), "untitled")+" "+str(info["version"]),
		clip(str(info["description"]), 4000), "openapi/swagger "+version, "info", map[string]any{"spec_version": version, "title": str(info["title"]), "version": str(info["version"])})

	var servers []string
	for _, s := range anySlice(doc["servers"]) {
		servers = append(servers, str(mapOf(s)["url"]))
	}
	if h := str(doc["host"]); h != "" {
		servers = append(servers, h+str(doc["basePath"]))
	}
	if len(servers) > 0 {
		b.verified("api", "Servers", "", strings.Join(servers, "\n"), "servers", map[string]any{"servers": servers})
	}

	// Security schemes.
	comps := mapOf(doc["components"])
	schemes := mapOf(comps["securitySchemes"])
	if schemes == nil {
		schemes = mapOf(doc["securityDefinitions"])
	}
	for _, name := range sortedKeys(schemes) {
		s := mapOf(schemes[name])
		desc := strings.TrimSpace(str(s["type"]) + " " + str(s["scheme"]) + " " + str(s["in"]) + " " + str(s["name"]))
		b.verified("auth", "Security scheme: "+name, clip(str(s["description"]), 1000), desc, "components.securitySchemes."+name, map[string]any{"scheme": s})
	}
	globalSec := len(anySlice(doc["security"])) > 0

	// Endpoints.
	paths := mapOf(doc["paths"])
	count := 0
	for _, p := range sortedKeys(paths) {
		item := mapOf(paths[p])
		for _, m := range httpMethods {
			op := mapOf(item[m])
			if op == nil {
				continue
			}
			count++
			var params []string
			for _, prm := range append(anySlice(item["parameters"]), anySlice(op["parameters"])...) {
				pm := mapOf(prm)
				if r := str(pm["$ref"]); r != "" {
					params = append(params, r[strings.LastIndex(r, "/")+1:]+" (ref)")
					continue
				}
				req := ""
				if pm["required"] == true {
					req = ", required"
				}
				params = append(params, fmt.Sprintf("%s (%s%s)", str(pm["name"]), str(pm["in"]), req))
			}
			reqBody := ""
			if rb := mapOf(op["requestBody"]); rb != nil {
				for ct, mt := range mapOf(rb["content"]) {
					reqBody = ct + ": " + refName(mapOf(mt)["schema"])
					break
				}
			}
			var responses []string
			for _, code := range sortedKeys(mapOf(op["responses"])) {
				resp := mapOf(mapOf(op["responses"])[code])
				schema := ""
				for _, mt := range mapOf(resp["content"]) {
					schema = refName(mapOf(mt)["schema"])
					break
				}
				if schema == "" {
					schema = refName(resp["schema"])
				}
				responses = append(responses, strings.TrimSpace(code+" "+schema))
			}
			secured := globalSec || len(anySlice(op["security"])) > 0
			if sec, ok := op["security"].([]any); ok && len(sec) == 0 {
				secured = false
			}
			var ev []string
			if len(params) > 0 {
				ev = append(ev, "Parameters: "+strings.Join(params, ", "))
			}
			if reqBody != "" {
				ev = append(ev, "Request body: "+reqBody)
			}
			ev = append(ev, "Responses: "+strings.Join(responses, "; "))
			title := strings.ToUpper(m) + " " + p
			b.verified("endpoint", title, firstNonEmpty(str(op["summary"]), clip(str(op["description"]), 500)), strings.Join(ev, "\n"),
				"paths."+p+"."+m, map[string]any{"method": strings.ToUpper(m), "path": p, "operation_id": str(op["operationId"]),
					"tags": anySlice(op["tags"]), "parameters": params, "request_body": reqBody, "responses": responses, "secured": secured,
					"summary": str(op["summary"])})
		}
	}
	// Schemas.
	schemas := mapOf(comps["schemas"])
	if schemas == nil {
		schemas = mapOf(doc["definitions"])
	}
	for _, name := range sortedKeys(schemas) {
		sc := mapOf(schemas[name])
		props := mapOf(sc["properties"])
		required := map[string]bool{}
		for _, r := range anySlice(sc["required"]) {
			required[str(r)] = true
		}
		var fields []map[string]any
		var lines []string
		for _, pn := range sortedKeys(props) {
			t := refName(props[pn])
			fields = append(fields, map[string]any{"name": pn, "type": t, "required": required[pn]})
			req := ""
			if required[pn] {
				req = " (required)"
			}
			lines = append(lines, pn+": "+t+req)
		}
		b.verified("schema", "Schema: "+name, clip(str(sc["description"]), 1000), strings.Join(lines, "\n"), "components.schemas."+name,
			map[string]any{"name": name, "fields": fields})
	}
	if count == 0 {
		b.unknown("endpoint", "The document declares no operations", "The paths object is empty.")
	}
	b.unknown("backend", "Implementation behind the API is not described",
		"An API description documents the contract, not the server implementation, data storage or business rules beyond what descriptions state.")
	return Result{Findings: b.out, Stats: map[string]any{"operations": count, "schemas": len(schemas)}}, nil
}

func anySlice(v any) []any {
	s, _ := v.([]any)
	return s
}

// ---- recorded request/response examples ----

type apiExamplesAnalyzer struct{}

func (apiExamplesAnalyzer) Name() string { return "api_examples" }

type apiExample struct {
	Method       string `json:"method"`
	URL          string `json:"url"`
	Status       int    `json:"status"`
	RequestBody  any    `json:"request_body"`
	ResponseBody any    `json:"response_body"`
}

// sensitiveHeaders are dropped from recorded traffic before anything is stored.
var sensitiveHeaders = map[string]bool{"authorization": true, "cookie": true, "set-cookie": true, "x-api-key": true,
	"proxy-authorization": true, "x-csrf-token": true, "x-auth-token": true}

// ParseExamples accepts a JSON array of examples or a HAR log.
func ParseExamples(text string) ([]apiExample, error) {
	var raw any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, fmt.Errorf("examples must be JSON (an array of examples or a HAR file): %w", err)
	}
	var out []apiExample
	if m := mapOf(raw); m != nil && m["log"] != nil {
		for _, e := range anySlice(mapOf(m["log"])["entries"]) {
			em := mapOf(e)
			req, resp := mapOf(em["request"]), mapOf(em["response"])
			ex := apiExample{Method: strings.ToUpper(str(req["method"])), URL: str(req["url"])}
			if st, ok := resp["status"].(float64); ok {
				ex.Status = int(st)
			}
			if pd := mapOf(req["postData"]); pd != nil {
				ex.RequestBody = decodeMaybeJSON(str(pd["text"]))
			}
			if c := mapOf(resp["content"]); c != nil && strings.Contains(str(c["mimeType"]), "json") {
				ex.ResponseBody = decodeMaybeJSON(str(c["text"]))
			}
			out = append(out, ex)
		}
		return out, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("expected an array of examples or a HAR file")
	}
	for _, e := range arr {
		em := mapOf(e)
		ex := apiExample{Method: strings.ToUpper(str(em["method"])), URL: str(em["url"]), RequestBody: em["request_body"], ResponseBody: em["response_body"]}
		if st, ok := em["status"].(float64); ok {
			ex.Status = int(st)
		}
		out = append(out, ex)
	}
	return out, nil
}

func decodeMaybeJSON(s string) any {
	var v any
	if json.Unmarshal([]byte(s), &v) == nil {
		return v
	}
	return nil
}

// TemplatePath replaces identifier-like segments with placeholders: /users/42 -> /users/{id}.
func TemplatePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		if s == "" {
			continue
		}
		digits := strings.Trim(s, "0123456789") == ""
		uuidLike := len(s) == 36 && strings.Count(s, "-") == 4
		hexLike := len(s) >= 16 && strings.Trim(strings.ToLower(s), "0123456789abcdef") == ""
		if digits || uuidLike || hexLike {
			parts[i] = "{id}"
		}
	}
	return strings.Join(parts, "/")
}

// shapeOf describes JSON values as types without retaining the values themselves.
func shapeOf(v any, depth int) any {
	if depth > 4 {
		return "…"
	}
	switch t := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for _, k := range sortedKeys(t) {
			if len(out) >= 40 {
				break
			}
			out[k] = shapeOf(t[k], depth+1)
		}
		return out
	case []any:
		if len(t) == 0 {
			return []any{}
		}
		return []any{shapeOf(t[0], depth+1)}
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case nil:
		return "null"
	}
	return "unknown"
}

func (apiExamplesAnalyzer) Analyze(ctx context.Context, in Input) (Result, error) {
	b := newBuilder()
	exs, err := ParseExamples(in.Text)
	if err != nil {
		return Result{}, err
	}
	type ep struct {
		method, path, host string
		statuses           map[int]bool
		req, resp          any
		count              int
	}
	eps := map[string]*ep{}
	for _, ex := range exs {
		u := ex.URL
		host := ""
		if i := strings.Index(u, "://"); i >= 0 {
			rest := u[i+3:]
			if j := strings.IndexByte(rest, '/'); j >= 0 {
				host, u = rest[:j], rest[j:]
			} else {
				host, u = rest, "/"
			}
		}
		if j := strings.IndexByte(u, '?'); j >= 0 {
			u = u[:j]
		}
		p := TemplatePath(u)
		key := ex.Method + " " + p
		e := eps[key]
		if e == nil {
			e = &ep{method: ex.Method, path: p, host: host, statuses: map[int]bool{}}
			eps[key] = e
		}
		e.count++
		if ex.Status > 0 {
			e.statuses[ex.Status] = true
		}
		if ex.RequestBody != nil && e.req == nil {
			e.req = shapeOf(ex.RequestBody, 0)
		}
		if ex.ResponseBody != nil && e.resp == nil {
			e.resp = shapeOf(ex.ResponseBody, 0)
		}
	}
	keys := sortedKeys(eps)
	for _, k := range keys {
		e := eps[k]
		var codes []int
		for c := range e.statuses {
			codes = append(codes, c)
		}
		sort.Ints(codes)
		var ev []string
		ev = append(ev, fmt.Sprintf("Observed %d time(s); status codes %v", e.count, codes))
		if e.req != nil {
			ev = append(ev, "Request shape: "+clip(mustJSON(e.req), 1500))
		}
		if e.resp != nil {
			ev = append(ev, "Response shape: "+clip(mustJSON(e.resp), 2500))
		}
		b.verified("endpoint", k, "Observed in recorded, authorized traffic. Only the structure of bodies is kept.", strings.Join(ev, "\n"), e.host+e.path,
			map[string]any{"method": e.method, "path": e.path, "host": e.host, "statuses": codes, "request_shape": e.req, "response_shape": e.resp})
		if strings.Contains(e.path, "{id}") {
			b.inferred("endpoint", "Path parameter in "+k, "Identifier-like segments were replaced with {id}.", e.path, e.host+e.path,
				"Numeric, UUID and long hexadecimal segments are assumed to be identifiers.", 0.75, nil)
		}
	}
	b.unknown("auth", "Credential details were removed", "Authorization headers and cookies are stripped from recorded traffic, so authentication requirements are inferred only from status codes.")
	b.unknown("endpoint", "Unobserved endpoints are unknown", "Only requests present in the recording are documented; other operations may exist.")
	return Result{Findings: b.out, Stats: map[string]any{"examples": len(exs), "endpoints": len(keys)}}, nil
}
