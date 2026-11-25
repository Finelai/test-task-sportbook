package checks

import (
	"errors"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/finelai/test-task-sportbook"

func TestGoImportBoundaries(t *testing.T) {
	for _, fixture := range []struct {
		name, file, imported string
		wantForbidden        bool
	}{
		{"cross-service", "services/bets/internal/app/app.go", "/services/line/api", true},
		{"domain-postgres", "services/wallet/internal/domain/wallet.go", "/services/wallet/internal/postgres", true},
		{"domain-http", "services/wallet/internal/domain/wallet.go", "/services/wallet/internal/httpapi/v1", true},
		{"domain-rpc", "services/wallet/internal/domain/wallet.go", "/services/wallet/internal/rpc", true},
		{"domain-app", "services/wallet/internal/domain/wallet.go", "/services/wallet/internal/app", true},
		{"domain-subpackage", "services/wallet/internal/domain/money/money.go", "/services/wallet/internal/domain", false},
		{"app-domain", "services/bets/internal/app/app.go", "/services/bets/internal/domain", false},
		{"shared-contract", "services/bets/internal/rpc/client.go", "/contracts/proto/line/v1", false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), fixture.file, "package fixture\nimport \""+modulePath+fixture.imported+"\"", parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			imported, err := strconv.Unquote(file.Imports[0].Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			got := forbiddenGoImport(fixture.file, imported)
			if got != fixture.wantForbidden {
				t.Errorf("%s imports %s: forbidden = %v, want %v", file.Name.Name, fixture.imported, got, fixture.wantForbidden)
			}
		})
	}
}

func TestRepositoryGoImportBoundaries(t *testing.T) {
	const root = "../.."
	for _, service := range []string{"line", "bets", "wallet"} {
		dir := filepath.Join(root, "services", service)
		if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			t.Fatal(err)
		}
		err := filepath.WalkDir(dir, func(file string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || filepath.Ext(file) != ".go" {
				return nil
			}
			relative, err := filepath.Rel(root, file)
			if err != nil {
				return err
			}
			files := token.NewFileSet()
			parsed, err := parser.ParseFile(files, file, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, spec := range parsed.Imports {
				imported, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					return err
				}
				if forbiddenGoImport(filepath.ToSlash(relative), imported) {
					t.Errorf("%s imports %q across an architecture boundary", files.Position(spec.Pos()), imported)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func forbiddenGoImport(file, imported string) bool {
	source := strings.Split(file, "/")
	targetPath, local := strings.CutPrefix(imported, modulePath+"/services/")
	if !local {
		return false
	}
	target := strings.Split(targetPath, "/")
	if source[1] != target[0] {
		return true
	}
	// A domain package can use other domain packages in its service.
	inDomain := len(source) >= 5 && source[2] == "internal" && source[3] == "domain"
	targetInDomain := len(target) >= 3 && target[1] == "internal" && target[2] == "domain"
	return inDomain && !targetInDomain
}
