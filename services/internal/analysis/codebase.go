package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func init() { register(codebaseAnalyzer{}) }

type codebaseAnalyzer struct{}

func (codebaseAnalyzer) Name() string { return "codebase" }

var languageByExt = map[string]string{
	".go": "Go", ".js": "JavaScript", ".mjs": "JavaScript", ".cjs": "JavaScript", ".jsx": "JavaScript", ".ts": "TypeScript",
	".tsx": "TypeScript", ".vue": "Vue", ".svelte": "Svelte", ".py": "Python", ".rb": "Ruby", ".php": "PHP", ".java": "Java",
	".kt": "Kotlin", ".kts": "Kotlin", ".scala": "Scala", ".cs": "C#", ".fs": "F#", ".rs": "Rust", ".c": "C", ".h": "C",
	".cpp": "C++", ".cc": "C++", ".hpp": "C++", ".swift": "Swift", ".m": "Objective-C", ".dart": "Dart", ".ex": "Elixir",
	".exs": "Elixir", ".erl": "Erlang", ".clj": "Clojure", ".lua": "Lua", ".r": "R", ".sql": "SQL", ".sh": "Shell",
	".bash": "Shell", ".ps1": "PowerShell", ".html": "HTML", ".css": "CSS", ".scss": "SCSS", ".less": "Less",
	".graphql": "GraphQL", ".gql": "GraphQL", ".proto": "Protocol Buffers", ".tf": "Terraform", ".yaml": "YAML", ".yml": "YAML",
	".json": "JSON", ".toml": "TOML", ".md": "Markdown",
}

// ignoredDirs are dependency, build and VCS directories that would distort the analysis.
var ignoredDirs = map[string]bool{"node_modules": true, "vendor": true, ".git": true, "dist": true, "build": true, ".next": true,
	".nuxt": true, ".venv": true, "venv": true, "__pycache__": true, "target": true, ".gradle": true, ".idea": true, ".vscode": true,
	"coverage": true, ".terraform": true, "bower_components": true, ".svn": true, ".hg": true, "out": true}

var binaryExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".ico": true, ".svg": false,
	".woff": true, ".woff2": true, ".ttf": true, ".eot": true, ".otf": true, ".zip": true, ".gz": true, ".tar": true, ".7z": true,
	".jar": true, ".war": true, ".class": true, ".exe": true, ".dll": true, ".so": true, ".dylib": true, ".pdf": true, ".mp4": true,
	".mp3": true, ".mov": true, ".psd": true, ".bin": true, ".wasm": true, ".pyc": true, ".lock": false}

// sensitiveFile matches files that commonly hold secrets. Their content is never read or stored.
var sensitiveFile = regexp.MustCompile(`(?i)(^|/)(\.env(\..+)?|id_rsa|id_ed25519|.*\.pem|.*\.key|.*\.p12|.*\.pfx|credentials(\.json)?|secrets?\.(ya?ml|json)|\.npmrc|\.pypirc|\.netrc|kubeconfig)$`)

// WantFile decides which archive members are read.
func WantFile(p string, size int64) bool {
	for _, part := range strings.Split(p, "/") {
		if ignoredDirs[part] {
			return false
		}
	}
	if sensitiveFile.MatchString(p) {
		return false
	}
	ext := strings.ToLower(path.Ext(p))
	if binaryExt[ext] {
		return false
	}
	base := path.Base(p)
	if _, ok := languageByExt[ext]; ok {
		return true
	}
	switch base {
	case "Dockerfile", "Makefile", "Procfile", "Gemfile", "Rakefile", "go.mod", "requirements.txt", "Pipfile", "pom.xml",
		"build.gradle", "Cargo.toml", "composer.json", "package.json", "pyproject.toml", "docker-compose.yml", "docker-compose.yaml",
		"compose.yml", "compose.yaml", "schema.prisma", "Jenkinsfile", ".gitlab-ci.yml", "README", "LICENSE":
		return true
	}
	return ext == ".prisma" || ext == ".gradle" || ext == ".xml" || ext == ".txt" || ext == ".ini" || ext == ".cfg" || ext == ".conf" || ext == ".env.example"
}

// ReadDirectory loads files from an allow-listed repository path without following symlinks.
func ReadDirectory(root string, maxFiles int, maxTotal, maxFile int64) ([]File, error) {
	var out []File
	var total int64
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if p != root && ignoredDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 || !d.Type().IsRegular() {
			out = append(out, File{Path: rel, Skipped: "link or special file"})
			return nil
		}
		if len(out) >= maxFiles {
			return fmt.Errorf("repository has more than %d files", maxFiles)
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		f := File{Path: rel, Size: info.Size()}
		switch {
		case info.Size() > maxFile:
			f.Skipped = "larger than the per-file analysis limit"
		case !WantFile(rel, info.Size()):
			f.Skipped = "not needed for analysis"
		default:
			data, err := os.ReadFile(p)
			if err != nil {
				f.Skipped = "unreadable"
				break
			}
			total += int64(len(data))
			if total > maxTotal {
				return fmt.Errorf("repository content exceeds %d MB", maxTotal>>20)
			}
			f.Content = data
		}
		out = append(out, f)
		return nil
	})
	return out, err
}

type depInfo struct {
	Manifest string `json:"manifest"`
	Name     string `json:"name"`
	Version  string `json:"version"`
	Dev      bool   `json:"dev,omitempty"`
}

var frameworkByDep = map[string]string{
	"react": "React", "next": "Next.js", "vue": "Vue.js", "nuxt": "Nuxt", "@angular/core": "Angular", "svelte": "Svelte",
	"@sveltejs/kit": "SvelteKit", "express": "Express", "fastify": "Fastify", "@nestjs/core": "NestJS", "koa": "Koa",
	"hono": "Hono", "prisma": "Prisma", "@prisma/client": "Prisma", "typeorm": "TypeORM", "sequelize": "Sequelize",
	"mongoose": "Mongoose", "graphql": "GraphQL", "@apollo/server": "Apollo Server", "vite": "Vite", "webpack": "webpack",
	"tailwindcss": "Tailwind CSS", "django": "Django", "flask": "Flask", "fastapi": "FastAPI", "sqlalchemy": "SQLAlchemy",
	"celery": "Celery", "rails": "Ruby on Rails", "sinatra": "Sinatra", "laravel/framework": "Laravel", "symfony/symfony": "Symfony",
	"github.com/gin-gonic/gin": "Gin", "github.com/go-chi/chi/v5": "chi", "github.com/labstack/echo/v4": "Echo",
	"github.com/gofiber/fiber/v2": "Fiber", "gorm.io/gorm": "GORM", "github.com/jackc/pgx/v5": "pgx (PostgreSQL)",
	"github.com/lib/pq": "lib/pq (PostgreSQL)", "github.com/go-sql-driver/mysql": "MySQL driver",
	"spring-boot-starter-web": "Spring Boot", "spring-boot-starter-data-jpa": "Spring Data JPA", "actix-web": "Actix Web",
	"axum": "Axum", "rocket": "Rocket", "diesel": "Diesel", "sqlx": "SQLx", "tokio": "Tokio", "stripe": "Stripe SDK",
	"pg": "node-postgres", "mysql2": "MySQL client", "redis": "Redis client", "ioredis": "Redis client", "psycopg2": "psycopg2 (PostgreSQL)",
	"psycopg": "psycopg (PostgreSQL)", "pymongo": "MongoDB driver", "boto3": "AWS SDK", "aws-sdk": "AWS SDK",
}

type routePattern struct {
	framework string
	re        *regexp.Regexp
	method    int // capture group index for the method, 0 when fixed
	path      int
	fixed     string
}

var routePatterns = []routePattern{
	{"Express/Koa/Fastify", regexp.MustCompile(`\b(?:app|router|server|fastify)\.(get|post|put|patch|delete|options|head)\(\s*['"\x60]([^'"\x60]+)['"\x60]`), 1, 2, ""},
	{"chi/Echo/Gin/Fiber", regexp.MustCompile(`\b(?:r|e|g|router|mux|api|app|group|v1)\.(Get|Post|Put|Patch|Delete|GET|POST|PUT|PATCH|DELETE)\(\s*"([^"]+)"`), 1, 2, ""},
	{"net/http", regexp.MustCompile(`\bHandleFunc\(\s*"([A-Z]+ )?([^"]+)"`), 1, 2, ""},
	{"Flask", regexp.MustCompile(`@\w+\.route\(\s*['"]([^'"]+)['"](?:[^)]*methods\s*=\s*\[([^\]]+)\])?`), 2, 1, ""},
	{"FastAPI", regexp.MustCompile(`@\w+\.(get|post|put|patch|delete)\(\s*['"]([^'"]+)['"]`), 1, 2, ""},
	{"Django", regexp.MustCompile(`\b(?:re_)?path\(\s*r?['"]([^'"]*)['"]\s*,`), 0, 1, "ANY"},
	{"Rails", regexp.MustCompile(`^\s*(get|post|put|patch|delete)\s+['"]([^'"]+)['"]`), 1, 2, ""},
	{"Rails resources", regexp.MustCompile(`^\s*resources?\s+:(\w+)`), 0, 1, "RESOURCE"},
	{"Laravel", regexp.MustCompile(`Route::(get|post|put|patch|delete|any)\(\s*['"]([^'"]+)['"]`), 1, 2, ""},
	{"Spring", regexp.MustCompile(`@(Get|Post|Put|Patch|Delete|Request)Mapping\(\s*(?:value\s*=\s*|path\s*=\s*)?"([^"]*)"`), 1, 2, ""},
	{"ASP.NET", regexp.MustCompile(`\[Http(Get|Post|Put|Patch|Delete)\(\s*"([^"]*)"\)\]`), 1, 2, ""},
	{"NestJS", regexp.MustCompile(`@(Get|Post|Put|Patch|Delete)\(\s*['"]([^'"]*)['"]\s*\)`), 1, 2, ""},
	{"Next.js API", regexp.MustCompile(`export\s+(?:async\s+)?function\s+(GET|POST|PUT|PATCH|DELETE)\s*\(`), 1, 0, ""},
}

type modelPattern struct {
	kind string
	re   *regexp.Regexp
}

var modelPatterns = []modelPattern{
	{"SQL table", regexp.MustCompile(`(?i)\bcreate\s+table\s+(?:if\s+not\s+exists\s+)?["\x60]?([\w.]+)["\x60]?`)},
	{"Prisma model", regexp.MustCompile(`(?m)^\s*model\s+(\w+)\s*\{`)},
	{"Django model", regexp.MustCompile(`class\s+(\w+)\((?:[\w.]*\.)?Model\)`)},
	{"SQLAlchemy model", regexp.MustCompile(`class\s+(\w+)\((?:Base|db\.Model|DeclarativeBase|SQLModel)[^)]*\)`)},
	{"ActiveRecord model", regexp.MustCompile(`class\s+(\w+)\s*<\s*(?:ApplicationRecord|ActiveRecord::Base)`)},
	{"TypeORM entity", regexp.MustCompile(`@Entity\([^)]*\)\s*(?:export\s+)?class\s+(\w+)`)},
	{"Mongoose schema", regexp.MustCompile(`(?:mongoose\.)?model\(\s*['"](\w+)['"]\s*,`)},
	{"JPA entity", regexp.MustCompile(`@Entity[\s\S]{0,200}?class\s+(\w+)`)},
	{"Eloquent model", regexp.MustCompile(`class\s+(\w+)\s+extends\s+Model\b`)},
}

var goStructRe = regexp.MustCompile(`type\s+(\w+)\s+struct\s*\{([^}]*)\}`)

var envPatterns = []*regexp.Regexp{
	regexp.MustCompile(`os\.(?:Getenv|LookupEnv)\(\s*"([A-Z][A-Z0-9_]{1,80})"`),
	regexp.MustCompile(`process\.env\.([A-Z][A-Z0-9_]{1,80})`),
	regexp.MustCompile(`process\.env\[\s*['"]([A-Z][A-Z0-9_]{1,80})['"]`),
	regexp.MustCompile(`os\.environ(?:\.get)?[\[(]\s*['"]([A-Z][A-Z0-9_]{1,80})['"]`),
	regexp.MustCompile(`os\.getenv\(\s*['"]([A-Z][A-Z0-9_]{1,80})['"]`),
	regexp.MustCompile(`ENV\[\s*['"]([A-Z][A-Z0-9_]{1,80})['"]`),
	regexp.MustCompile(`import\.meta\.env\.([A-Z][A-Z0-9_]{1,80})`),
	regexp.MustCompile(`env\(\s*['"]([A-Z][A-Z0-9_]{1,80})['"]`),
}

var importPatterns = map[string]*regexp.Regexp{
	"Go":         regexp.MustCompile(`"([\w./-]+)"`),
	"JavaScript": regexp.MustCompile(`(?:from\s+|require\(\s*|import\s*\(\s*)['"](\.{1,2}/[^'"]+)['"]`),
	"TypeScript": regexp.MustCompile(`(?:from\s+|require\(\s*|import\s*\(\s*)['"](\.{1,2}/[^'"]+|@/[^'"]+)['"]`),
	"Python":     regexp.MustCompile(`(?m)^\s*(?:from|import)\s+([\w.]+)`),
}

func lineOf(content string, idx int) int { return strings.Count(content[:idx], "\n") + 1 }

func (codebaseAnalyzer) Analyze(ctx context.Context, in Input) (Result, error) {
	b := newBuilder()
	files := in.Files
	if len(files) == 0 {
		b.unknown("coverage", "No source files were available", "The archive or repository path contained no readable files.")
		return Result{Findings: b.out, Stats: map[string]any{"files": 0}}, nil
	}
	langFiles, langLines := map[string]int{}, map[string]int{}
	topDirs := map[string]int{}
	var sensitive, skippedLarge []string
	analyzed := 0
	var goModule string
	for _, f := range files {
		if sensitiveFile.MatchString(f.Path) {
			sensitive = append(sensitive, f.Path)
			continue
		}
		if f.Skipped == "larger than the per-file analysis limit" {
			skippedLarge = append(skippedLarge, f.Path)
		}
		parts := strings.SplitN(f.Path, "/", 2)
		if len(parts) == 2 {
			topDirs[parts[0]]++
		} else {
			topDirs["(root)"]++
		}
		if lang, ok := languageByExt[strings.ToLower(path.Ext(f.Path))]; ok {
			langFiles[lang]++
			if f.Content != nil {
				langLines[lang] += strings.Count(string(f.Content), "\n") + 1
			}
		}
		if f.Content != nil {
			analyzed++
		}
		if path.Base(f.Path) == "go.mod" && goModule == "" {
			if m := regexp.MustCompile(`(?m)^module\s+(\S+)`).FindStringSubmatch(string(f.Content)); m != nil {
				goModule = m[1]
			}
		}
	}
	stats := map[string]any{"files": len(files), "files_analyzed": analyzed, "languages": langFiles}

	// Languages.
	langs := sortedByValue(langFiles)
	var ll []string
	for _, l := range langs {
		ll = append(ll, fmt.Sprintf("%s: %d files, %d lines", l, langFiles[l], langLines[l]))
	}
	if len(ll) > 0 {
		b.verified("language", "Languages used ("+plural(len(langs), "language")+")", "Counted by file extension, excluding dependency and build directories.",
			strings.Join(ll, "\n"), "file extensions", map[string]any{"files": langFiles, "lines": langLines})
	}
	// Structure.
	dirs := sortedByValue(topDirs)
	var dl []string
	for _, d := range dirs {
		dl = append(dl, fmt.Sprintf("%s/ (%d files)", d, topDirs[d]))
	}
	b.verified("structure", "Top-level layout ("+plural(len(dirs), "directory")+")", "", strings.Join(dl, "\n"), "/", map[string]any{"directories": topDirs})

	// Manifests and dependencies.
	var deps []depInfo
	for _, f := range files {
		if f.Content == nil {
			continue
		}
		deps = append(deps, parseManifest(f.Path, string(f.Content))...)
	}
	byManifest := map[string][]depInfo{}
	for _, d := range deps {
		byManifest[d.Manifest] = append(byManifest[d.Manifest], d)
	}
	for _, m := range sortedKeys(byManifest) {
		list := byManifest[m]
		var lines []string
		for _, d := range list {
			dev := ""
			if d.Dev {
				dev = " (dev)"
			}
			lines = append(lines, strings.TrimSpace(d.Name+" "+d.Version)+dev)
		}
		b.verified("dependency", "Dependencies declared in "+m+" ("+plural(len(list), "package")+")", "", strings.Join(lines, "\n"), m,
			map[string]any{"manifest": m, "dependencies": list})
	}
	frameworks := map[string][]string{}
	for _, d := range deps {
		if fw, ok := frameworkByDep[strings.ToLower(d.Name)]; ok {
			frameworks[fw] = append(frameworks[fw], d.Manifest)
		}
	}
	for _, fw := range sortedKeys(frameworks) {
		b.inferred("technology", fw+" is used", "Inferred from a declared dependency.", fw+" declared in "+strings.Join(uniq(frameworks[fw]), ", "),
			strings.Join(uniq(frameworks[fw]), ", "), "A declared dependency usually means the library is used, but it may be unused or only used in tooling.",
			0.85, map[string]any{"technology": fw})
	}

	// Routes, models, environment variables, imports.
	type route struct {
		Method, Path, File, Framework string
		Line                          int
	}
	var routes []route
	models := map[string][]string{}
	envVars := map[string][]string{}
	edges := map[string]map[string]bool{}
	for _, f := range files {
		if f.Content == nil {
			continue
		}
		content := string(f.Content)
		ext := strings.ToLower(path.Ext(f.Path))
		lang := languageByExt[ext]
		if lang != "" && lang != "Markdown" && lang != "JSON" && lang != "YAML" {
			for _, rp := range routePatterns {
				for _, m := range rp.re.FindAllStringSubmatchIndex(content, 200) {
					get := func(g int) string {
						if g == 0 || m[2*g] < 0 {
							return ""
						}
						return content[m[2*g]:m[2*g+1]]
					}
					method := strings.ToUpper(strings.TrimSpace(get(rp.method)))
					if rp.fixed != "" {
						method = rp.fixed
					}
					if rp.framework == "Flask" {
						method = strings.ToUpper(strings.Trim(strings.ReplaceAll(get(rp.method), " ", ""), `'"`))
						if method == "" {
							method = "GET"
						}
					}
					p := get(rp.path)
					if rp.framework == "Next.js API" {
						p = "/" + strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(f.Path, "src/"), "app/"), "/route"+ext)
					}
					if method == "" {
						method = "ANY"
					}
					routes = append(routes, route{method, p, f.Path, rp.framework, lineOf(content, m[0])})
				}
			}
			for _, mp := range modelPatterns {
				if mp.kind == "SQL table" && ext != ".sql" && !strings.Contains(content, "CREATE TABLE") && !strings.Contains(content, "create table") {
					continue
				}
				for _, m := range mp.re.FindAllStringSubmatch(content, 500) {
					models[m[1]] = append(models[m[1]], mp.kind+" in "+f.Path)
				}
			}
			if lang == "Go" {
				for _, m := range goStructRe.FindAllStringSubmatch(content, 500) {
					if strings.Contains(m[2], "gorm:") || strings.Contains(m[2], "db:\"") || strings.Contains(m[2], "bun:\"") {
						models[m[1]] = append(models[m[1]], "Go struct with database tags in "+f.Path)
					}
				}
			}
			for _, re := range envPatterns {
				for _, m := range re.FindAllStringSubmatch(content, 200) {
					envVars[m[1]] = append(envVars[m[1]], f.Path)
				}
			}
			if re, ok := importPatterns[lang]; ok {
				from := topDir(f.Path)
				for _, m := range re.FindAllStringSubmatch(content, 500) {
					target := ""
					switch lang {
					case "Go":
						if goModule != "" && strings.HasPrefix(m[1], goModule+"/") {
							target = topDir(strings.TrimPrefix(m[1], goModule+"/"))
						}
					case "Python":
						first := strings.Split(m[1], ".")[0]
						if _, ok := topDirs[first]; ok {
							target = first
						}
					default:
						if strings.HasPrefix(m[1], "@/") {
							target = "src"
						} else {
							target = topDir(path.Clean(path.Join(path.Dir(f.Path), m[1])))
						}
					}
					if target != "" && target != from && target != "." && target != ".." {
						if edges[from] == nil {
							edges[from] = map[string]bool{}
						}
						edges[from][target] = true
					}
				}
			}
		}
		if ext == ".sql" {
			continue
		}
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})
	seenRoute := map[string]bool{}
	for _, rt := range routes {
		key := rt.Method + " " + rt.Path
		if seenRoute[key] {
			continue
		}
		seenRoute[key] = true
		b.verified("route", key, "Route registered with "+rt.Framework+".", fmt.Sprintf("%s:%d", rt.File, rt.Line), fmt.Sprintf("%s:%d", rt.File, rt.Line),
			map[string]any{"method": rt.Method, "path": rt.Path, "file": rt.File, "line": rt.Line, "framework": rt.Framework})
	}
	stats["routes"] = len(seenRoute)
	for _, name := range sortedKeys(models) {
		srcs := uniq(models[name])
		b.verified("data_model", "Data model: "+name, "", strings.Join(srcs, "\n"), srcs[0], map[string]any{"name": name, "sources": srcs})
	}
	stats["data_models"] = len(models)
	if len(envVars) > 0 {
		names := sortedKeys(envVars)
		var lines []string
		for _, n := range names {
			lines = append(lines, n+" ("+strings.Join(uniq(envVars[n])[:min(3, len(uniq(envVars[n])))], ", ")+")")
		}
		b.verified("configuration", "Environment variables read ("+plural(len(names), "variable")+")", "Only names are recorded; values are never read.",
			strings.Join(lines, "\n"), "source code", map[string]any{"variables": names})
	}
	if len(edges) > 0 {
		var lines []string
		graph := map[string][]string{}
		for _, from := range sortedKeys(edges) {
			targets := sortedKeys(edges[from])
			graph[from] = targets
			for _, t := range targets {
				lines = append(lines, from+" → "+t)
			}
		}
		b.inferred("architecture", "Module dependencies between top-level directories", "Derived from import statements.",
			strings.Join(lines, "\n"), "import statements", "Static imports only; dynamic loading, dependency injection and runtime wiring are not captured.",
			0.8, map[string]any{"graph": graph})
	}

	// Entry points and infrastructure.
	for _, f := range files {
		base := path.Base(f.Path)
		content := string(f.Content)
		switch {
		case base == "Dockerfile" || strings.HasSuffix(base, ".Dockerfile"):
			from := regexp.MustCompile(`(?im)^FROM\s+(\S+)`).FindAllStringSubmatch(content, 10)
			var images []string
			for _, m := range from {
				images = append(images, m[1])
			}
			cmd := regexp.MustCompile(`(?im)^(?:CMD|ENTRYPOINT)\s+(.+)$`).FindStringSubmatch(content)
			ev := "Base images: " + strings.Join(images, ", ")
			if cmd != nil {
				ev += "\nStart command: " + clip(cmd[1], 200)
			}
			b.verified("infrastructure", "Container image definition: "+f.Path, "", ev, f.Path, map[string]any{"base_images": images})
		case base == "docker-compose.yml" || base == "docker-compose.yaml" || base == "compose.yml" || base == "compose.yaml":
			svcs := composeServices(content)
			b.verified("infrastructure", "Docker Compose services ("+plural(len(svcs), "service")+")", "", strings.Join(svcs, "\n"), f.Path, map[string]any{"services": svcs})
		case strings.HasPrefix(f.Path, ".github/workflows/") || base == ".gitlab-ci.yml" || base == "Jenkinsfile":
			b.verified("infrastructure", "CI pipeline: "+f.Path, "", clip(content, 1500), f.Path, nil)
		case base == "main.go" || base == "manage.py" || base == "wsgi.py" || base == "asgi.py" || base == "server.js" || base == "server.ts" || base == "Program.cs":
			b.inferred("entry_point", "Entry point: "+f.Path, "A conventional application entry point file.", clip(content, 600), f.Path,
				"Inferred from the file name convention.", 0.7, map[string]any{"file": f.Path})
		case base == "package.json":
			var pkg struct {
				Scripts map[string]string `json:"scripts"`
				Main    string            `json:"main"`
			}
			if json.Unmarshal(f.Content, &pkg) == nil && len(pkg.Scripts) > 0 {
				var lines []string
				for _, k := range sortedKeys(pkg.Scripts) {
					lines = append(lines, k+": "+clip(pkg.Scripts[k], 160))
				}
				b.verified("build", "npm scripts in "+f.Path, "Commands are recorded, never executed.", strings.Join(lines, "\n"), f.Path, map[string]any{"scripts": pkg.Scripts})
			}
		}
	}
	if len(sensitive) > 0 {
		b.verified("security", "Files that may contain secrets were skipped ("+plural(len(sensitive), "file")+")",
			"REA does not read or store the contents of these files. Consider removing secrets from the repository.",
			strings.Join(sensitive, "\n"), "file names", map[string]any{"files": sensitive})
	}
	if len(skippedLarge) > 0 {
		b.verified("coverage", "Large files were listed but not analyzed ("+plural(len(skippedLarge), "file")+")", "", strings.Join(skippedLarge[:min(50, len(skippedLarge))], "\n"), "file sizes", nil)
	}
	if len(seenRoute) == 0 {
		b.unknown("route", "No HTTP routes were recognised",
			"Routes may be declared in a framework REA does not recognise, generated at runtime, or the project may not expose an HTTP API.")
	}
	if len(models) == 0 {
		b.unknown("data_model", "No data models were recognised",
			"Persistence may use a library REA does not recognise, raw queries built at runtime, or an external service.")
	}
	b.unknown("runtime", "Runtime behaviour is not observed",
		"Static analysis does not execute the code, so performance, production configuration, data volumes and external service responses are unknown.")
	return Result{Findings: b.out, Stats: stats}, nil
}

func topDir(p string) string {
	p = strings.TrimPrefix(p, "./")
	if i := strings.IndexByte(p, '/'); i > 0 {
		return p[:i]
	}
	return "(root)"
}

func sortedByValue(m map[string]int) []string {
	keys := sortedKeys(m)
	sort.SliceStable(keys, func(i, j int) bool { return m[keys[i]] > m[keys[j]] })
	return keys
}

func composeServices(content string) []string {
	var out []string
	inServices := false
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			inServices = strings.HasPrefix(line, "services:")
			continue
		}
		if inServices {
			trim := strings.TrimLeft(line, " ")
			indent := len(line) - len(trim)
			if indent == 2 && strings.HasSuffix(strings.TrimSpace(trim), ":") {
				out = append(out, strings.TrimSuffix(strings.TrimSpace(trim), ":"))
			}
		}
	}
	return out
}

var (
	reqLineRe     = regexp.MustCompile(`^\s*([A-Za-z0-9_.\-\[\]]+)\s*([<>=!~]=?\s*[^;#\s]+)?`)
	gemRe         = regexp.MustCompile(`(?m)^\s*gem\s+['"]([^'"]+)['"](?:\s*,\s*['"]([^'"]+)['"])?`)
	gradleRe      = regexp.MustCompile(`(?m)^\s*(implementation|api|compileOnly|runtimeOnly|testImplementation)\s*\(?\s*['"]([^'"]+)['"]`)
	pomDepRe      = regexp.MustCompile(`(?s)<dependency>.*?<artifactId>([^<]+)</artifactId>(?:.*?<version>([^<]+)</version>)?.*?</dependency>`)
	tomlSectionRe = regexp.MustCompile(`^\s*\[([^\]]+)\]\s*$`)
	tomlDepRe     = regexp.MustCompile(`^\s*([A-Za-z0-9_\-]+)\s*=\s*(?:"([^"]+)"|\{.*?version\s*=\s*"([^"]+)".*\})`)
	goReqRe       = regexp.MustCompile(`(?m)^\s*([a-zA-Z0-9.\-_/]+\.[a-z]{2,}[^\s]*)\s+(v[0-9][^\s]*)(\s+//\s*indirect)?`)
)

// parseManifest extracts declared dependencies from common manifest formats.
func parseManifest(p, content string) []depInfo {
	base := path.Base(p)
	var out []depInfo
	switch base {
	case "package.json", "composer.json":
		var pkg map[string]json.RawMessage
		if json.Unmarshal([]byte(content), &pkg) != nil {
			return nil
		}
		for _, sec := range []struct {
			key string
			dev bool
		}{{"dependencies", false}, {"devDependencies", true}, {"peerDependencies", false}, {"require", false}, {"require-dev", true}} {
			var deps map[string]string
			if raw, ok := pkg[sec.key]; ok && json.Unmarshal(raw, &deps) == nil {
				for _, name := range sortedKeys(deps) {
					out = append(out, depInfo{p, name, deps[name], sec.dev})
				}
			}
		}
	case "go.mod":
		for _, m := range goReqRe.FindAllStringSubmatch(content, -1) {
			if strings.HasPrefix(strings.TrimSpace(m[0]), "module") || strings.HasPrefix(strings.TrimSpace(m[0]), "go ") {
				continue
			}
			out = append(out, depInfo{p, m[1], m[2], m[3] != ""})
		}
	case "requirements.txt", "requirements-dev.txt", "requirements.in":
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
				continue
			}
			if m := reqLineRe.FindStringSubmatch(line); m != nil {
				out = append(out, depInfo{p, strings.ToLower(m[1]), strings.TrimSpace(m[2]), strings.Contains(base, "dev")})
			}
		}
	case "pyproject.toml", "Cargo.toml", "Pipfile":
		section := ""
		inArray := false
		for _, line := range strings.Split(content, "\n") {
			if m := tomlSectionRe.FindStringSubmatch(line); m != nil {
				section = m[1]
				inArray = false
				continue
			}
			trim := strings.TrimSpace(line)
			if base == "pyproject.toml" && (strings.HasPrefix(trim, "dependencies") && strings.Contains(trim, "[")) {
				inArray = true
			}
			if inArray {
				for _, q := range regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(trim, -1) {
					if m := reqLineRe.FindStringSubmatch(q[1]); m != nil {
						out = append(out, depInfo{p, strings.ToLower(m[1]), strings.TrimSpace(m[2]), false})
					}
				}
				if strings.Contains(trim, "]") {
					inArray = false
				}
				continue
			}
			isDepSection := strings.HasSuffix(section, "dependencies") || section == "packages" || section == "dev-packages"
			if isDepSection {
				if m := tomlDepRe.FindStringSubmatch(line); m != nil && m[1] != "python" {
					v := m[2]
					if v == "" {
						v = m[3]
					}
					out = append(out, depInfo{p, m[1], v, strings.Contains(section, "dev")})
				}
			}
		}
	case "Gemfile":
		for _, m := range gemRe.FindAllStringSubmatch(content, -1) {
			out = append(out, depInfo{p, m[1], m[2], false})
		}
	case "pom.xml":
		for _, m := range pomDepRe.FindAllStringSubmatch(content, -1) {
			out = append(out, depInfo{p, strings.TrimSpace(m[1]), strings.TrimSpace(m[2]), false})
		}
	case "build.gradle", "build.gradle.kts":
		for _, m := range gradleRe.FindAllStringSubmatch(content, -1) {
			parts := strings.Split(m[2], ":")
			name, ver := m[2], ""
			if len(parts) >= 2 {
				name = parts[1]
			}
			if len(parts) >= 3 {
				ver = parts[2]
			}
			out = append(out, depInfo{p, name, ver, strings.HasPrefix(m[1], "test")})
		}
	}
	if len(out) > 1000 {
		out = out[:1000]
	}
	return out
}
