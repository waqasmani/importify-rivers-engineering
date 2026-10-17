package migrate_test

import (
	"context"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/waqasmani/importify-rivers-engineering/db"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/migrate"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/testutil"
)

// destructive matches statements that lose data when run against a live database. They are only
// allowed in an Up section when the line before carries "-- rea:allow-destructive <reason>".
var destructive = regexp.MustCompile(`(?i)\b(DROP\s+(TABLE|COLUMN|SCHEMA|DATABASE|TYPE)|TRUNCATE|DELETE\s+FROM|ALTER\s+COLUMN\s+\S+\s+(SET\s+DATA\s+)?TYPE)\b`)

func migrationFiles(t *testing.T) []string {
	t.Helper()
	entries, err := fs.ReadDir(db.Migrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

// lintMigration returns convention violations for one migration file.
func lintMigration(name, src string, wantVersion int) []string {
	var problems []string
	m := regexp.MustCompile(`^(\d{5})_[a-z0-9_]+\.sql$`).FindStringSubmatch(name)
	if m == nil {
		return []string{name + ": file name must look like 00001_description.sql"}
	}
	if v, _ := strconv.Atoi(m[1]); v != wantVersion {
		problems = append(problems, name+": versions must be sequential")
	}
	up := strings.Index(src, "-- +goose Up")
	down := strings.Index(src, "-- +goose Down")
	if up < 0 || down < 0 || down < up {
		return append(problems, name+": needs a '-- +goose Up' section followed by a '-- +goose Down' section")
	}
	lines := strings.Split(src[up:down], "\n")
	inBody := false // inside a dollar-quoted function body, which runs later, not at migration time
	for j, line := range lines {
		code := line
		if k := strings.Index(code, "--"); k >= 0 {
			code = code[:k]
		}
		wasInBody := inBody
		if strings.Count(code, "$$")%2 == 1 {
			inBody = !inBody
		}
		if wasInBody || inBody || !destructive.MatchString(code) {
			continue
		}
		if j > 0 && strings.Contains(lines[j-1], "-- rea:allow-destructive") {
			continue
		}
		problems = append(problems, name+": destructive statement in Up section needs a '-- rea:allow-destructive <reason>' comment: "+strings.TrimSpace(line))
	}
	if strings.TrimSpace(strings.TrimPrefix(src[down:], "-- +goose Down")) == "" {
		problems = append(problems, name+": Down section is empty; every migration must be reversible")
	}
	return problems
}

// TestMigrationLint enforces the migration conventions without a database: sequential versions,
// both directions present, and no destructive statements in forward migrations.
func TestMigrationLint(t *testing.T) {
	names := migrationFiles(t)
	if len(names) == 0 {
		t.Fatal("no migrations embedded")
	}
	for i, name := range names {
		b, _ := fs.ReadFile(db.Migrations, "migrations/"+name)
		for _, p := range lintMigration(name, string(b), i+1) {
			t.Error(p)
		}
	}
}

func TestMigrationLintRejectsUnsafeFiles(t *testing.T) {
	cases := map[string]string{
		"drop column": "-- +goose Up\nALTER TABLE products DROP COLUMN sku;\n-- +goose Down\nSELECT 1;\n",
		"truncate":    "-- +goose Up\nTRUNCATE products;\n-- +goose Down\nSELECT 1;\n",
		"type change": "-- +goose Up\nALTER TABLE products ALTER COLUMN sku TYPE int USING 0;\n-- +goose Down\nSELECT 1;\n",
		"no down":     "-- +goose Up\nCREATE TABLE x (id int);\n-- +goose Down\n",
	}
	for label, src := range cases {
		if len(lintMigration("00009_bad.sql", src, 9)) == 0 {
			t.Errorf("%s: expected a lint error", label)
		}
	}
	ok := "-- +goose Up\n-- rea:allow-destructive column was never written\nALTER TABLE products DROP COLUMN legacy;\n-- +goose Down\nALTER TABLE products ADD COLUMN legacy text;\n"
	if p := lintMigration("00009_ok.sql", ok, 9); len(p) != 0 {
		t.Errorf("annotated statement should pass: %v", p)
	}
	if p := lintMigration("9_bad.sql", ok, 9); len(p) == 0 {
		t.Error("bad file name should fail")
	}
}

type snapshot map[string][]string

func schemaSnapshot(t *testing.T, ctx context.Context, url string) snapshot {
	t.Helper()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	queries := map[string]string{
		"columns": `SELECT table_name || '.' || column_name || ' ' || data_type || ' ' || is_nullable || ' ' || coalesce(column_default, '')
			FROM information_schema.columns WHERE table_schema = 'public' AND table_name <> 'goose_db_version'`,
		"indexes":     `SELECT indexdef FROM pg_indexes WHERE schemaname = 'public' AND tablename <> 'goose_db_version'`,
		"constraints": `SELECT conrelid::regclass::text || ' ' || pg_get_constraintdef(oid) FROM pg_constraint WHERE connamespace = 'public'::regnamespace AND conrelid::regclass::text <> 'goose_db_version'`,
		"functions":   `SELECT p.proname || '(' || pg_get_function_identity_arguments(p.oid) || ')' FROM pg_proc p WHERE p.pronamespace = 'public'::regnamespace AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.objid = p.oid AND d.deptype = 'e')`,
		"triggers":    `SELECT tgrelid::regclass::text || ' ' || tgname FROM pg_trigger WHERE NOT tgisinternal`,
		"seed":        `SELECT 'permissions ' || count(*) FROM permissions UNION ALL SELECT 'roles ' || count(*) FROM roles UNION ALL SELECT 'adapters ' || count(*) FROM source_adapters UNION ALL SELECT 'pipelines ' || count(*) FROM pipelines`,
	}
	out := snapshot{}
	for k, q := range queries {
		rows, err := conn.Query(ctx, q)
		if err != nil {
			if k == "seed" {
				continue // tables do not exist after a full rollback
			}
			t.Fatalf("%s: %v", k, err)
		}
		vals, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatalf("%s: %v", k, err)
		}
		sort.Strings(vals)
		out[k] = vals
	}
	return out
}

// TestMigrationsRollBackAndReapply proves every Down section reverses its Up section: rolling all
// migrations back leaves no application objects, and re-applying them reproduces the exact schema
// and seed data.
func TestMigrationsRollBackAndReapply(t *testing.T) {
	ctx := context.Background()
	url := testutil.NewDatabase(t)
	before := schemaSnapshot(t, ctx, url)
	if len(before["columns"]) == 0 {
		t.Fatal("expected a migrated database")
	}

	// Roll back one migration at a time so each Down section runs against the schema it expects.
	st, err := migrate.GetStatus(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	for v := st.Current; v > 0; v-- {
		got, err := migrate.Down(ctx, url)
		if err != nil {
			t.Fatalf("rolling back version %d: %v", v, err)
		}
		if got != v-1 {
			t.Fatalf("after rolling back %d the version is %d", v, got)
		}
	}
	empty := schemaSnapshot(t, ctx, url)
	for _, k := range []string{"columns", "indexes", "constraints", "functions", "triggers"} {
		if len(empty[k]) != 0 {
			t.Errorf("after full rollback %s remain: %v", k, empty[k])
		}
	}

	if _, err := migrate.Up(ctx, url); err != nil {
		t.Fatalf("re-applying migrations: %v", err)
	}
	after := schemaSnapshot(t, ctx, url)
	for k, vals := range before {
		if strings.Join(vals, "\n") != strings.Join(after[k], "\n") {
			t.Errorf("%s differ after rollback and re-apply\nbefore: %v\nafter:  %v", k, vals, after[k])
		}
	}
}
