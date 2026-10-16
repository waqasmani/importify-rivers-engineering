package httpapi_test

import (
	"archive/zip"
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestFullExportExcludesSecrets(t *testing.T) {
	e := newEnv(t)
	e.owner.Do("POST", "/api/v1/companies", map[string]any{"name": "Exported Co"}).Must(t, 201)
	e.owner.Do("POST", "/api/v1/ai/providers", map[string]any{"name": "Claude", "kind": "anthropic", "model": "claude-opus-5-5", "api_key": "sk-ant-export-secret", "enabled": true}).Must(t, 201)
	res := e.owner.Do("GET", "/api/v1/admin/export", nil).Must(t, 200)
	zr, err := zip.NewReader(bytes.NewReader(res.Body), int64(len(res.Body)))
	if err != nil {
		t.Fatal(err)
	}
	all := ""
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
		rc, _ := f.Open()
		var b bytes.Buffer
		_, _ = b.ReadFrom(rc)
		rc.Close()
		all += b.String()
	}
	for _, want := range []string{"companies.jsonl", "products.jsonl", "analysis_findings.jsonl", "manifest.json"} {
		if !names[want] {
			t.Errorf("missing %s", want)
		}
	}
	if !strings.Contains(all, "Exported Co") {
		t.Error("export missing data")
	}
	for _, secret := range []string{"password_hash", "argon2id", "sk-ant-export-secret", "api_key_ciphertext", "token_hash", "csrf_token"} {
		if strings.Contains(all, secret) {
			t.Errorf("export contains %q", secret)
		}
	}
	mine := e.owner.Do("GET", "/api/v1/auth/me/export", nil).Must(t, 200)
	if !bytes.HasPrefix(mine.Body, []byte("PK")) {
		t.Fatal("personal export is not a zip")
	}
	viewer := e.userWithRole("viewer", "v@example.com")
	viewer.Do("GET", "/api/v1/admin/export", nil).Must(t, 403)
	viewer.Do("GET", "/api/v1/admin/backups", nil).Must(t, 403)
}

func TestBackupsWithPgDump(t *testing.T) {
	if _, err := exec.LookPath("pg_dump"); err != nil {
		t.Skip("pg_dump not installed")
	}
	e := newEnv(t)
	b := e.owner.Do("POST", "/api/v1/admin/backups", map[string]any{}).Must(t, 201)
	name := b.Str("name")
	if !strings.HasPrefix(name, "rea-") || b.Get("size_bytes").(float64) < 1000 {
		t.Fatalf("backup wrong: %s", b.Body)
	}
	list := e.owner.Do("GET", "/api/v1/admin/backups", nil).Must(t, 200)
	if len(list.Items()) != 1 {
		t.Fatalf("backup not listed: %s", list.Body)
	}
	dl := e.owner.Do("GET", "/api/v1/admin/backups/"+name, nil).Must(t, 200)
	if !bytes.HasPrefix(dl.Body, []byte("PGDMP")) {
		t.Fatal("download is not a pg_dump archive")
	}
	e.owner.Do("GET", "/api/v1/admin/backups/..%2F..%2Fetc%2Fpasswd", nil).Must(t, 404)
	e.owner.Do("DELETE", "/api/v1/admin/backups/"+name, nil).Must(t, 204)
}
