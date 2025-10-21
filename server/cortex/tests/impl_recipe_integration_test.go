package tests

import (
    "io"
    "os"
    "path/filepath"
    "testing"

    fixtures "github.com/divisive-ai/vibethis/server/recipe-worker/test-fixtures"
)

func TestImplRecipeStubMode(t *testing.T) {
    tmp := t.TempDir()

    // Prepare recipe directory structure expected by the harness
    recipesDir := filepath.Join(tmp, "recipes")
    if err := os.MkdirAll(recipesDir, 0o755); err != nil {
        t.Fatalf("create recipes dir: %v", err)
    }

    // Copy recipe definition
    recipeSrc := filepath.Join("..", "recipes", "impl-recipe.yaml")
    recipeDst := filepath.Join(recipesDir, "impl-recipe.yaml")
    copyFile(t, recipeSrc, recipeDst)

    // Copy recipe test cases
    testSrc := filepath.Join("impl-recipe.test.yaml")
    testDst := filepath.Join(recipesDir, "impl-recipe.test.yaml")
    copyFile(t, testSrc, testDst)

    // Run harness against the temporary directory
    cwd, err := os.Getwd()
    if err != nil {
        t.Fatalf("getwd: %v", err)
    }
    if err := os.Chdir(tmp); err != nil {
        t.Fatalf("chdir: %v", err)
    }
    t.Cleanup(func() {
        _ = os.Chdir(cwd)
    })

    fixtures.RunTestOnAllRecipes("recipes/*.test.yaml", t)
}

func copyFile(t *testing.T, src, dst string) {
    t.Helper()
    in, err := os.Open(src)
    if err != nil {
        t.Fatalf("open %s: %v", src, err)
    }
    defer in.Close()

    out, err := os.Create(dst)
    if err != nil {
        t.Fatalf("create %s: %v", dst, err)
    }
    defer func() {
        cerr := out.Close()
        if cerr != nil {
            t.Fatalf("close %s: %v", dst, cerr)
        }
    }()

    if _, err := io.Copy(out, in); err != nil {
        t.Fatalf("copy %s -> %s: %v", src, dst, err)
    }
}
