package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func FuzzResolveStaysInsideTheVault(f *testing.F) {
	for _, seed := range []string{
		"Inbox/a.md", "../a.md", "a/../../b.md", "/etc/passwd", "./a.md",
		"a/./b/../c.md", "..", ".", "a\x00b.md", `..\a.md`, "a//b.md", "....//a.md",
	} {
		f.Add(seed)
	}
	root := f.TempDir()
	v := New("Fuzz", root)
	f.Fuzz(func(t *testing.T, rel string) {
		clean, err := v.resolve(rel)
		if err != nil {
			return
		}
		joined := filepath.Join(root, clean)
		if joined != root && !strings.HasPrefix(joined, root+string(filepath.Separator)) {
			t.Fatalf("resolve(%q) = %q, which joins to %q outside %q", rel, clean, joined, root)
		}
	})
}

func FuzzReadNeverLeavesTheVault(f *testing.F) {
	for _, seed := range []string{"leak.md", "Out/secret.md", "Inbox/../leak.md", "alias.md", "Out/../Out/secret.md"} {
		f.Add(seed)
	}
	outside := f.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("outside-secret"), 0o644); err != nil {
		f.Fatal(err)
	}
	v := New("Fuzz", f.TempDir())
	if err := os.MkdirAll(filepath.Join(v.Root(), "Inbox"), 0o755); err != nil {
		f.Fatal(err)
	}
	for name, target := range map[string]string{
		"leak.md": filepath.Join(outside, "secret.md"),
		"Out":     outside,
	} {
		if err := os.Symlink(target, filepath.Join(v.Root(), name)); err != nil {
			f.Skipf("symlinks unavailable: %v", err)
		}
	}
	f.Fuzz(func(t *testing.T, rel string) {
		data, err := v.ReadAll(rel)
		if err == nil && strings.Contains(string(data), "outside-secret") {
			t.Fatalf("ReadAll(%q) returned a file from outside the vault", rel)
		}
	})
}
