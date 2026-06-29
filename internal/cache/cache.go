package cache

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/sourcegraph/scip-cli-go/internal/project"
)

const (
	IndexDB         = "index.db"
	IndexDBNext     = "index.db.next"
	CacheSlugMaxLen = 48
	IndexLock       = ".index.lock"
	RootHashLen     = 12
)

func projectRootHash(projectRoot string) string {
	abs, _ := filepath.Abs(projectRoot)
	h := sha256.Sum256([]byte(abs))
	return fmt.Sprintf("%x", h)[:RootHashLen]
}

func IndexBuildLock(cacheDir string) (*os.File, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(cacheDir, IndexLock)
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func UnlockIndex(f *os.File) {
	if f != nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}
}

func IndexDBPath(cacheDir string, replace bool) string {
	if replace {
		return filepath.Join(cacheDir, IndexDBNext)
	}
	return filepath.Join(cacheDir, IndexDB)
}

func unlinkSQLiteSidecars(dbPath string) {
	for _, suffix := range []string{"-wal", "-shm"} {
		sidecar := dbPath + suffix
		_ = os.Remove(sidecar)
	}
}

func CleanupInProgressIndex(cacheDir string) {
	nextDB := IndexDBPath(cacheDir, true)
	_ = os.Remove(nextDB)
	unlinkSQLiteSidecars(nextDB)
}

func PromoteNextIndex(cacheDir string) error {
	nextDB := IndexDBPath(cacheDir, true)
	liveDB := IndexDBPath(cacheDir, false)

	if _, err := os.Stat(nextDB); os.IsNotExist(err) {
		return fmt.Errorf("index.db.next is missing")
	}

	unlinkSQLiteSidecars(liveDB)
	unlinkSQLiteSidecars(nextDB)

	return os.Rename(nextDB, liveDB)
}

func ProjectCacheSlug(projectRoot string) string {
	root, _ := filepath.Abs(projectRoot)
	parts := strings.Split(root, string(filepath.Separator))
	slugBase := parts[len(parts)-1]
	if slugBase == "" && len(parts) > 1 {
		slugBase = parts[len(parts)-2]
	}

	re := regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
	slug := re.ReplaceAllString(slugBase, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "project"
	}
	if len(slug) > CacheSlugMaxLen {
		slug = strings.TrimRight(slug[:CacheSlugMaxLen], "-")
	}

	digest := projectRootHash(root)[:6]
	return fmt.Sprintf("%s-%s", slug, digest)
}

func cacheBase() string {
	if v := os.Getenv("SCIP_CLI_CACHE"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "scip-cli")
}

func GetCacheDir(projectRoot string) string {
	return filepath.Join(cacheBase(), "projects", ProjectCacheSlug(projectRoot))
}

func FindDB(projectRoot string) string {
	root := projectRoot
	if root == "" {
		if resolved, ok := project.FindProjectRoot(""); ok {
			root = resolved
		} else {
			root, _ = os.Getwd()
		}
	}
	cache := IndexDBPath(GetCacheDir(root), false)
	if _, err := os.Stat(cache); err == nil {
		return cache
	}
	return ""
}
