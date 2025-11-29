package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSandbox(t *testing.T) {
	tmpDir := t.TempDir()
	sandboxDir := filepath.Join(tmpDir, "sandbox")

	sb, err := NewSandbox(sandboxDir)
	require.NoError(t, err)
	require.NotNil(t, sb)

	// Verify directory was created
	info, err := os.Stat(sandboxDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// Verify BaseDir returns absolute path
	assert.True(t, filepath.IsAbs(sb.BaseDir()))
}

func TestSandbox_ResolvePath(t *testing.T) {
	sb := setupTestSandbox(t)

	tests := []struct {
		name        string
		path        string
		shouldError bool
	}{
		{"simple file", "test.txt", false},
		{"nested path", "subdir/test.txt", false},
		{"deep nesting", "a/b/c/d/test.txt", false},
		{"current dir", ".", false},
		{"parent escape attempt", "../escape.txt", true},
		{"nested parent escape", "subdir/../../escape.txt", true},
		{"absolute path escape", "/etc/passwd", true},
		{"hidden file", ".hidden", false},
		{"dot dot name", "..test", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, err := sb.ResolvePath(tt.path)
			if tt.shouldError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "escapes sandbox")
			} else {
				assert.NoError(t, err)
				assert.True(t, strings.HasPrefix(resolved, sb.BaseDir()))
			}
		})
	}
}

func TestSandbox_WriteAndReadFile(t *testing.T) {
	sb := setupTestSandbox(t)
	content := []byte("test content")

	// Write file
	err := sb.WriteFile("test.txt", content)
	require.NoError(t, err)

	// Read file
	data, err := sb.ReadFile("test.txt")
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestSandbox_WriteFile_CreatesParentDirs(t *testing.T) {
	sb := setupTestSandbox(t)
	content := []byte("nested content")

	err := sb.WriteFile("a/b/c/test.txt", content)
	require.NoError(t, err)

	// Verify file exists
	exists, err := sb.Exists("a/b/c/test.txt")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestSandbox_Exists(t *testing.T) {
	sb := setupTestSandbox(t)

	// File doesn't exist
	exists, err := sb.Exists("nonexistent.txt")
	require.NoError(t, err)
	assert.False(t, exists)

	// Create file
	err = sb.WriteFile("exists.txt", []byte("test"))
	require.NoError(t, err)

	// File exists
	exists, err = sb.Exists("exists.txt")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestSandbox_MkdirAll(t *testing.T) {
	sb := setupTestSandbox(t)

	err := sb.MkdirAll("a/b/c")
	require.NoError(t, err)

	exists, err := sb.Exists("a/b/c")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestSandbox_Remove(t *testing.T) {
	sb := setupTestSandbox(t)

	// Create file
	err := sb.WriteFile("to_remove.txt", []byte("test"))
	require.NoError(t, err)

	// Remove it
	err = sb.Remove("to_remove.txt")
	require.NoError(t, err)

	// Verify it's gone
	exists, err := sb.Exists("to_remove.txt")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestSandbox_RemoveAll(t *testing.T) {
	sb := setupTestSandbox(t)

	// Create nested structure
	err := sb.WriteFile("dir/subdir/file.txt", []byte("test"))
	require.NoError(t, err)

	// Remove all
	err = sb.RemoveAll("dir")
	require.NoError(t, err)

	// Verify it's gone
	exists, err := sb.Exists("dir")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestSandbox_RemoveAll_CannotRemoveBase(t *testing.T) {
	sb := setupTestSandbox(t)

	err := sb.RemoveAll(".")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot remove sandbox base directory")
}

func TestSandbox_Rename(t *testing.T) {
	sb := setupTestSandbox(t)

	// Create file
	content := []byte("rename test")
	err := sb.WriteFile("old.txt", content)
	require.NoError(t, err)

	// Rename it
	err = sb.Rename("old.txt", "new.txt")
	require.NoError(t, err)

	// Verify old is gone
	exists, err := sb.Exists("old.txt")
	require.NoError(t, err)
	assert.False(t, exists)

	// Verify new exists with same content
	data, err := sb.ReadFile("new.txt")
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestSandbox_AtomicWrite(t *testing.T) {
	sb := setupTestSandbox(t)
	content := []byte("atomic content")

	err := sb.AtomicWrite("atomic.txt", content)
	require.NoError(t, err)

	data, err := sb.ReadFile("atomic.txt")
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestSandbox_AtomicWriteReader(t *testing.T) {
	sb := setupTestSandbox(t)
	content := []byte("atomic reader content")
	reader := bytes.NewReader(content)

	err := sb.AtomicWriteReader("atomic_reader.txt", reader)
	require.NoError(t, err)

	data, err := sb.ReadFile("atomic_reader.txt")
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestSandbox_CreateTemp(t *testing.T) {
	sb := setupTestSandbox(t)

	file, err := sb.CreateTemp("", "test-*.tmp")
	require.NoError(t, err)
	defer os.Remove(file.Name())
	defer file.Close()

	// Verify file is within sandbox
	assert.True(t, strings.HasPrefix(file.Name(), sb.BaseDir()))
}

func TestSandbox_TempDir(t *testing.T) {
	sb := setupTestSandbox(t)

	tempDir, err := sb.TempDir()
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(tempDir, sb.BaseDir()))

	info, err := os.Stat(tempDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestSandbox_List(t *testing.T) {
	sb := setupTestSandbox(t)

	// Create some files
	err := sb.WriteFile("file1.txt", []byte("1"))
	require.NoError(t, err)
	err = sb.WriteFile("file2.txt", []byte("2"))
	require.NoError(t, err)
	err = sb.MkdirAll("subdir")
	require.NoError(t, err)

	entries, err := sb.List(".")
	require.NoError(t, err)
	assert.Len(t, entries, 3)
}

func TestSandbox_Walk(t *testing.T) {
	sb := setupTestSandbox(t)

	// Create structure
	err := sb.WriteFile("root.txt", []byte("root"))
	require.NoError(t, err)
	err = sb.WriteFile("dir/nested.txt", []byte("nested"))
	require.NoError(t, err)

	var paths []string
	err = sb.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		paths = append(paths, path)
		return nil
	})
	require.NoError(t, err)
	assert.Contains(t, paths, "root.txt")
	assert.Contains(t, paths, filepath.Join("dir", "nested.txt"))
}

func TestSandbox_Stat(t *testing.T) {
	sb := setupTestSandbox(t)

	content := []byte("stat test")
	err := sb.WriteFile("stat.txt", content)
	require.NoError(t, err)

	info, err := sb.Stat("stat.txt")
	require.NoError(t, err)
	assert.Equal(t, int64(len(content)), info.Size())
	assert.False(t, info.IsDir())
}

func TestSandbox_Size(t *testing.T) {
	sb := setupTestSandbox(t)

	content := []byte("size test content")
	err := sb.WriteFile("size.txt", content)
	require.NoError(t, err)

	size, err := sb.Size("size.txt")
	require.NoError(t, err)
	assert.Equal(t, int64(len(content)), size)
}

func TestSandbox_OpenFile(t *testing.T) {
	sb := setupTestSandbox(t)

	// Write using OpenFile
	file, err := sb.OpenFile("open.txt", os.O_CREATE|os.O_WRONLY, 0640)
	require.NoError(t, err)

	_, err = file.WriteString("open file test")
	require.NoError(t, err)
	file.Close()

	// Read back
	data, err := sb.ReadFile("open.txt")
	require.NoError(t, err)
	assert.Equal(t, "open file test", string(data))
}

func TestSandbox_SubSandbox(t *testing.T) {
	sb := setupTestSandbox(t)

	sub, err := sb.SubSandbox("subdir")
	require.NoError(t, err)

	// Write to subsandbox
	err = sub.WriteFile("test.txt", []byte("subsandbox"))
	require.NoError(t, err)

	// Verify it's in the right place
	data, err := sb.ReadFile("subdir/test.txt")
	require.NoError(t, err)
	assert.Equal(t, "subsandbox", string(data))
}

func TestSandbox_PathTraversalAttempts(t *testing.T) {
	sb := setupTestSandbox(t)

	// Various path traversal attempts that should be blocked on all platforms
	attacks := []string{
		"../../../etc/passwd",
		"subdir/../../../etc/passwd",
		"/absolute/path",
		"subdir/../../..",
		"subdir/./../../etc/passwd",
	}

	for _, attack := range attacks {
		t.Run(attack, func(t *testing.T) {
			_, err := sb.ResolvePath(attack)
			assert.Error(t, err, "path traversal should be blocked: %s", attack)
		})
	}
}

func setupTestSandbox(t *testing.T) *Sandbox {
	t.Helper()

	tmpDir := t.TempDir()
	sb, err := NewSandbox(tmpDir)
	require.NoError(t, err)

	return sb
}
