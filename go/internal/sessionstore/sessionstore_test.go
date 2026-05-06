package sessionstore_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/agusrdz/ctxgate/internal/sessionstore"
	"github.com/stretchr/testify/require"
)

func openTestStore(t *testing.T) *sessionstore.SessionStore {
	t.Helper()
	s, err := sessionstore.Open("test-session", t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

// --- session ID sanitization ---

func TestOpen_SanitizesSessionID(t *testing.T) {
	dir := t.TempDir()
	s, err := sessionstore.Open("bad/../session id!", dir)
	require.NoError(t, err)
	defer s.Close()

	// DB must be named "unknown.db" since the ID is invalid.
	_, err = os.Stat(filepath.Join(dir, "unknown.db"))
	require.NoError(t, err, "expected unknown.db for invalid session ID")
}

func TestOpen_ValidSessionID(t *testing.T) {
	dir := t.TempDir()
	s, err := sessionstore.Open("abc-123_XY", dir)
	require.NoError(t, err)
	defer s.Close()

	_, err = os.Stat(filepath.Join(dir, "abc-123_XY.db"))
	require.NoError(t, err, "expected abc-123_XY.db for valid session ID")
}

// --- file_reads CRUD ---

func TestFileEntry_RoundTrip(t *testing.T) {
	s := openTestStore(t)

	e := &sessionstore.FileEntry{
		FilePath:                   "/foo/bar.py",
		MtimeNs:                    1_000_000_000,
		SizeBytes:                  4096,
		RangesSeen:                 `[[0,100]]`,
		TokensEst:                  250,
		ReadCount:                  3,
		ContentHash:                "abc123",
		LastAccess:                 1_700_000_000.5,
		LastReplacementFingerprint: "fp1",
		LastReplacementType:        "structure",
		RepeatReplacementCount:     1,
		LastStructureReason:        "large file",
		LastStructureConfidence:    0.85,
	}

	require.NoError(t, s.UpsertFileEntry(e.FilePath, e))

	got, err := s.GetFileEntry(e.FilePath)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, e.FilePath, got.FilePath)
	require.Equal(t, e.MtimeNs, got.MtimeNs)
	require.Equal(t, e.SizeBytes, got.SizeBytes)
	require.Equal(t, e.RangesSeen, got.RangesSeen)
	require.Equal(t, e.TokensEst, got.TokensEst)
	require.Equal(t, e.ReadCount, got.ReadCount)
	require.Equal(t, e.ContentHash, got.ContentHash)
	require.InDelta(t, e.LastAccess, got.LastAccess, 1e-6)
	require.Equal(t, e.LastReplacementFingerprint, got.LastReplacementFingerprint)
	require.Equal(t, e.LastReplacementType, got.LastReplacementType)
	require.Equal(t, e.RepeatReplacementCount, got.RepeatReplacementCount)
	require.Equal(t, e.LastStructureReason, got.LastStructureReason)
	require.InDelta(t, e.LastStructureConfidence, got.LastStructureConfidence, 1e-9)
}

func TestFileEntry_NullContentHash(t *testing.T) {
	s := openTestStore(t)

	e := &sessionstore.FileEntry{
		FilePath:    "/no/hash.go",
		MtimeNs:    1,
		SizeBytes:  1,
		RangesSeen: "[]",
		LastAccess: 1.0,
	}
	require.NoError(t, s.UpsertFileEntry(e.FilePath, e))

	got, err := s.GetFileEntry(e.FilePath)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "", got.ContentHash)
}

func TestGetFileEntry_NotFound(t *testing.T) {
	s := openTestStore(t)
	got, err := s.GetFileEntry("/does/not/exist")
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestUpsertFileEntry_Overwrites(t *testing.T) {
	s := openTestStore(t)

	e := &sessionstore.FileEntry{FilePath: "/f.py", MtimeNs: 1, SizeBytes: 1, RangesSeen: "[]", TokensEst: 10, ReadCount: 1, LastAccess: 1.0}
	require.NoError(t, s.UpsertFileEntry(e.FilePath, e))

	e.TokensEst = 999
	e.ReadCount = 5
	require.NoError(t, s.UpsertFileEntry(e.FilePath, e))

	got, err := s.GetFileEntry(e.FilePath)
	require.NoError(t, err)
	require.Equal(t, 999, got.TokensEst)
	require.Equal(t, 5, got.ReadCount)
}

func TestDeleteFileEntry(t *testing.T) {
	s := openTestStore(t)

	e := &sessionstore.FileEntry{FilePath: "/del.py", MtimeNs: 1, SizeBytes: 1, RangesSeen: "[]", LastAccess: 1.0}
	require.NoError(t, s.UpsertFileEntry(e.FilePath, e))
	require.NoError(t, s.DeleteFileEntry(e.FilePath))

	got, err := s.GetFileEntry(e.FilePath)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestClearFileEntries(t *testing.T) {
	s := openTestStore(t)

	for i := range 5 {
		e := &sessionstore.FileEntry{
			FilePath: fmt.Sprintf("/file%d.py", i), MtimeNs: 1, SizeBytes: 1, RangesSeen: "[]", LastAccess: 1.0,
		}
		require.NoError(t, s.UpsertFileEntry(e.FilePath, e))
	}

	require.NoError(t, s.ClearFileEntries())

	all, err := s.GetAllFileEntries()
	require.NoError(t, err)
	require.Empty(t, all)
}

func TestGetAllFileEntries(t *testing.T) {
	s := openTestStore(t)

	paths := []string{"/a.py", "/b.go", "/c.ts"}
	for _, p := range paths {
		e := &sessionstore.FileEntry{FilePath: p, MtimeNs: 1, SizeBytes: 1, RangesSeen: "[]", LastAccess: 1.0}
		require.NoError(t, s.UpsertFileEntry(p, e))
	}

	all, err := s.GetAllFileEntries()
	require.NoError(t, err)
	require.Len(t, all, 3)
	for _, p := range paths {
		require.Contains(t, all, p)
	}
}

// --- cached_content CRUD ---

func TestCachedContent_RoundTrip(t *testing.T) {
	s := openTestStore(t)

	require.NoError(t, s.UpsertCachedContent("/foo.py", "print('hello')", "hash1"))

	got, err := s.GetCachedContent("/foo.py")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "/foo.py", got.FilePath)
	require.Equal(t, "print('hello')", got.Content)
	require.Equal(t, "hash1", got.ContentHash)
	require.Greater(t, got.CachedAt, float64(0))
}

func TestGetCachedContent_NotFound(t *testing.T) {
	s := openTestStore(t)
	got, err := s.GetCachedContent("/missing.py")
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestDeleteCachedContent(t *testing.T) {
	s := openTestStore(t)
	require.NoError(t, s.UpsertCachedContent("/del.py", "x", "h"))
	require.NoError(t, s.DeleteCachedContent("/del.py"))
	got, err := s.GetCachedContent("/del.py")
	require.NoError(t, err)
	require.Nil(t, got)
}

// --- GetRecentFileReads ---

func TestGetRecentFileReads(t *testing.T) {
	s := openTestStore(t)

	now := float64(time.Now().UnixNano()) / 1e9
	entries := []sessionstore.FileEntry{
		{FilePath: "/hot.py", MtimeNs: 1, SizeBytes: 1, RangesSeen: "[]", TokensEst: 500, ReadCount: 10, LastAccess: now},
		{FilePath: "/warm.py", MtimeNs: 1, SizeBytes: 1, RangesSeen: "[]", TokensEst: 200, ReadCount: 3, LastAccess: now - 1},
		{FilePath: "/cold.py", MtimeNs: 1, SizeBytes: 1, RangesSeen: "[]", TokensEst: 50, ReadCount: 1, LastAccess: now - 2},
	}
	for i := range entries {
		require.NoError(t, s.UpsertFileEntry(entries[i].FilePath, &entries[i]))
	}

	// minReadCount=2 should exclude /cold.py
	rows, err := s.GetRecentFileReads(10, 2)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	// ordered by last_access DESC
	require.Equal(t, "/hot.py", rows[0].FilePath)
	require.Equal(t, "/warm.py", rows[1].FilePath)
}

func TestGetRecentFileReads_Limit(t *testing.T) {
	s := openTestStore(t)

	now := float64(time.Now().UnixNano()) / 1e9
	for i := range 5 {
		e := &sessionstore.FileEntry{
			FilePath: fmt.Sprintf("/f%d.py", i), MtimeNs: 1, SizeBytes: 1, RangesSeen: "[]",
			ReadCount: 2, LastAccess: now - float64(i),
		}
		require.NoError(t, s.UpsertFileEntry(e.FilePath, e))
	}

	rows, err := s.GetRecentFileReads(3, 1)
	require.NoError(t, err)
	require.Len(t, rows, 3)
}

// --- CleanupOldStores ---

func TestCleanupOldStores_DeletesOld(t *testing.T) {
	dir := t.TempDir()

	// Create a fresh store (won't be deleted).
	s, err := sessionstore.Open("fresh", dir)
	require.NoError(t, err)
	s.Close()

	// Fake an old .db file by back-dating its mtime.
	oldDB := filepath.Join(dir, "old.db")
	require.NoError(t, os.WriteFile(oldDB, []byte{}, 0o600))
	old := time.Now().Add(-49 * time.Hour)
	require.NoError(t, os.Chtimes(oldDB, old, old))

	deleted, err := sessionstore.CleanupOldStores(dir, 48)
	require.NoError(t, err)
	require.Equal(t, 1, deleted)

	_, err = os.Stat(oldDB)
	require.True(t, os.IsNotExist(err))

	// fresh.db must survive.
	_, err = os.Stat(filepath.Join(dir, "fresh.db"))
	require.NoError(t, err)
}

func TestCleanupOldStores_MissingDir(t *testing.T) {
	deleted, err := sessionstore.CleanupOldStores(filepath.Join(t.TempDir(), "nonexistent"), 48)
	require.NoError(t, err)
	require.Equal(t, 0, deleted)
}

func TestCleanupOldStores_RemovesWALFiles(t *testing.T) {
	dir := t.TempDir()

	base := filepath.Join(dir, "old.db")
	for _, name := range []string{base, base + "-wal", base + "-shm"} {
		require.NoError(t, os.WriteFile(name, []byte{}, 0o600))
	}
	old := time.Now().Add(-49 * time.Hour)
	require.NoError(t, os.Chtimes(base, old, old))

	deleted, err := sessionstore.CleanupOldStores(dir, 48)
	require.NoError(t, err)
	require.Equal(t, 1, deleted)

	for _, name := range []string{base, base + "-wal", base + "-shm"} {
		_, err := os.Stat(name)
		require.True(t, os.IsNotExist(err), "expected %s to be deleted", name)
	}
}

// --- WAL concurrent-write stress test ---

func TestWALConcurrentWrites(t *testing.T) {
	dir := t.TempDir()

	// All goroutines share the same session store (same DB file).
	s, err := sessionstore.Open("stress", dir)
	require.NoError(t, err)
	defer s.Close()

	const goroutines = 5
	const writesEach = 20

	var wg sync.WaitGroup
	errs := make(chan error, goroutines*writesEach)

	for g := range goroutines {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := range writesEach {
				e := &sessionstore.FileEntry{
					FilePath:   fmt.Sprintf("/goroutine%d/file%d.py", g, i),
					MtimeNs:    int64(g*1000 + i),
					SizeBytes:  512,
					RangesSeen: "[]",
					TokensEst:  100,
					ReadCount:  1,
					LastAccess: float64(time.Now().UnixNano()) / 1e9,
				}
				if err := s.UpsertFileEntry(e.FilePath, e); err != nil {
					errs <- err
				}
			}
		}(g)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}

	all, err := s.GetAllFileEntries()
	require.NoError(t, err)
	require.Len(t, all, goroutines*writesEach)
}
