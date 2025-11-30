package publish

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/pipeline/core"
	"github.com/jmylchreest/tvarr/internal/pipeline/stages/generatem3u"
	"github.com/jmylchreest/tvarr/internal/pipeline/stages/generatexmltv"
	"github.com/jmylchreest/tvarr/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStageID(t *testing.T) {
	assert.Equal(t, "publish", StageID)
}

func TestStageName(t *testing.T) {
	assert.Equal(t, "Publish", StageName)
}

func TestNew(t *testing.T) {
	sandbox, err := storage.NewSandbox(t.TempDir())
	require.NoError(t, err)

	stage := New(sandbox)
	assert.Equal(t, StageID, stage.ID())
	assert.Equal(t, StageName, stage.Name())
}

func TestNewConstructor(t *testing.T) {
	sandbox, err := storage.NewSandbox(t.TempDir())
	require.NoError(t, err)

	deps := &core.Dependencies{Sandbox: sandbox}
	constructor := NewConstructor()
	stage := constructor(deps)

	assert.NotNil(t, stage)
	assert.Equal(t, StageID, stage.ID())
}

func TestExecute_PublishM3U(t *testing.T) {
	tempDir := t.TempDir()
	sandbox, err := storage.NewSandbox(tempDir)
	require.NoError(t, err)

	stage := New(sandbox)

	// Create a source file to publish
	srcDir := filepath.Join(tempDir, "source")
	require.NoError(t, os.MkdirAll(srcDir, 0755))
	srcPath := filepath.Join(srcDir, "test.m3u")
	testContent := []byte("#EXTM3U\n#EXTINF:-1,StreamCast News HD\nhttp://stream.example.com/1")
	require.NoError(t, os.WriteFile(srcPath, testContent, 0644))

	// Create output directory
	outputDir := filepath.Join(tempDir, "output")
	require.NoError(t, os.MkdirAll(outputDir, 0755))

	// Create pipeline state
	proxy := &models.StreamProxy{}
	proxy.ID = models.NewULID()
	state := core.NewState(proxy)
	state.OutputDir = outputDir
	state.SetMetadata(generatem3u.MetadataKeyTempPath, srcPath)

	// Execute
	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	// Verify result
	assert.Equal(t, 1, result.RecordsProcessed)
	assert.Contains(t, result.Message, "Published 1 files")

	// Verify file was published
	destPath := filepath.Join(outputDir, proxy.ID.String()+".m3u")
	published, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, testContent, published)
}

func TestExecute_PublishXMLTV(t *testing.T) {
	tempDir := t.TempDir()
	sandbox, err := storage.NewSandbox(tempDir)
	require.NoError(t, err)

	stage := New(sandbox)

	// Create a source file to publish
	srcDir := filepath.Join(tempDir, "source")
	require.NoError(t, os.MkdirAll(srcDir, 0755))
	srcPath := filepath.Join(srcDir, "test.xml")
	testContent := []byte(`<?xml version="1.0"?><tv></tv>`)
	require.NoError(t, os.WriteFile(srcPath, testContent, 0644))

	// Create output directory
	outputDir := filepath.Join(tempDir, "output")
	require.NoError(t, os.MkdirAll(outputDir, 0755))

	// Create pipeline state
	proxy := &models.StreamProxy{}
	proxy.ID = models.NewULID()
	state := core.NewState(proxy)
	state.OutputDir = outputDir
	state.SetMetadata(generatexmltv.MetadataKeyTempPath, srcPath)

	// Execute
	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	// Verify result
	assert.Equal(t, 1, result.RecordsProcessed)

	// Verify file was published
	destPath := filepath.Join(outputDir, proxy.ID.String()+".xml")
	published, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, testContent, published)
}

func TestExecute_PublishBothFiles(t *testing.T) {
	tempDir := t.TempDir()
	sandbox, err := storage.NewSandbox(tempDir)
	require.NoError(t, err)

	stage := New(sandbox)

	// Create source files
	srcDir := filepath.Join(tempDir, "source")
	require.NoError(t, os.MkdirAll(srcDir, 0755))

	m3uPath := filepath.Join(srcDir, "test.m3u")
	require.NoError(t, os.WriteFile(m3uPath, []byte("#EXTM3U"), 0644))

	xmltvPath := filepath.Join(srcDir, "test.xml")
	require.NoError(t, os.WriteFile(xmltvPath, []byte("<tv/>"), 0644))

	// Create output directory
	outputDir := filepath.Join(tempDir, "output")
	require.NoError(t, os.MkdirAll(outputDir, 0755))

	// Create pipeline state
	proxy := &models.StreamProxy{}
	proxy.ID = models.NewULID()
	state := core.NewState(proxy)
	state.OutputDir = outputDir
	state.SetMetadata(generatem3u.MetadataKeyTempPath, m3uPath)
	state.SetMetadata(generatexmltv.MetadataKeyTempPath, xmltvPath)

	// Execute
	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	// Verify result
	assert.Equal(t, 2, result.RecordsProcessed)
	assert.Contains(t, result.Message, "Published 2 files")
	assert.Len(t, result.Artifacts, 2)

	// Verify files were published
	assert.FileExists(t, filepath.Join(outputDir, proxy.ID.String()+".m3u"))
	assert.FileExists(t, filepath.Join(outputDir, proxy.ID.String()+".xml"))
}

func TestExecute_NoFiles(t *testing.T) {
	tempDir := t.TempDir()
	sandbox, err := storage.NewSandbox(tempDir)
	require.NoError(t, err)

	stage := New(sandbox)

	// Create output directory
	outputDir := filepath.Join(tempDir, "output")
	require.NoError(t, os.MkdirAll(outputDir, 0755))

	// Create pipeline state with no metadata
	proxy := &models.StreamProxy{}
	proxy.ID = models.NewULID()
	state := core.NewState(proxy)
	state.OutputDir = outputDir

	// Execute
	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	// Verify result
	assert.Equal(t, 0, result.RecordsProcessed)
	assert.Contains(t, result.Message, "Published 0 files")
}

func TestExecute_CreatesOutputDir(t *testing.T) {
	tempDir := t.TempDir()
	sandbox, err := storage.NewSandbox(tempDir)
	require.NoError(t, err)

	stage := New(sandbox)

	// Create a source file
	srcDir := filepath.Join(tempDir, "source")
	require.NoError(t, os.MkdirAll(srcDir, 0755))
	srcPath := filepath.Join(srcDir, "test.m3u")
	require.NoError(t, os.WriteFile(srcPath, []byte("#EXTM3U"), 0644))

	// Output directory does NOT exist
	outputDir := filepath.Join(tempDir, "nonexistent", "output")

	// Create pipeline state
	proxy := &models.StreamProxy{}
	proxy.ID = models.NewULID()
	state := core.NewState(proxy)
	state.OutputDir = outputDir
	state.SetMetadata(generatem3u.MetadataKeyTempPath, srcPath)

	// Execute
	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	// Verify directory was created and file published
	assert.Equal(t, 1, result.RecordsProcessed)
	assert.DirExists(t, outputDir)
	assert.FileExists(t, filepath.Join(outputDir, proxy.ID.String()+".m3u"))
}

func TestExecute_ContextCancellation(t *testing.T) {
	tempDir := t.TempDir()
	sandbox, err := storage.NewSandbox(tempDir)
	require.NoError(t, err)

	stage := New(sandbox)

	// Create a source file
	srcDir := filepath.Join(tempDir, "source")
	require.NoError(t, os.MkdirAll(srcDir, 0755))
	srcPath := filepath.Join(srcDir, "test.m3u")
	require.NoError(t, os.WriteFile(srcPath, []byte("#EXTM3U"), 0644))

	outputDir := filepath.Join(tempDir, "output")
	require.NoError(t, os.MkdirAll(outputDir, 0755))

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Create pipeline state
	proxy := &models.StreamProxy{}
	proxy.ID = models.NewULID()
	state := core.NewState(proxy)
	state.OutputDir = outputDir
	state.SetMetadata(generatem3u.MetadataKeyTempPath, srcPath)

	// Execute with cancelled context
	_, err = stage.Execute(ctx, state)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled), "error should wrap context.Canceled")
}

func TestPublishFile_DirectRename(t *testing.T) {
	tempDir := t.TempDir()
	sandbox, err := storage.NewSandbox(tempDir)
	require.NoError(t, err)

	stage := New(sandbox)

	// Create a source file
	srcPath := filepath.Join(tempDir, "source.txt")
	testContent := []byte("test content")
	require.NoError(t, os.WriteFile(srcPath, testContent, 0644))

	// Publish to same directory (should use direct rename)
	destName := "dest.txt"
	err = stage.publishFile(context.Background(), srcPath, tempDir, destName)
	require.NoError(t, err)

	// Verify destination exists
	destPath := filepath.Join(tempDir, destName)
	published, err := os.ReadFile(destPath)
	require.NoError(t, err)
	assert.Equal(t, testContent, published)

	// Source should no longer exist (was renamed)
	_, err = os.Stat(srcPath)
	assert.True(t, os.IsNotExist(err))
}

func TestPublishFile_MissingSource(t *testing.T) {
	tempDir := t.TempDir()
	sandbox, err := storage.NewSandbox(tempDir)
	require.NoError(t, err)

	stage := New(sandbox)

	// Try to publish non-existent file
	err = stage.publishFile(context.Background(), "/nonexistent/file.txt", tempDir, "dest.txt")
	require.Error(t, err)
}

func TestCleanup(t *testing.T) {
	sandbox, err := storage.NewSandbox(t.TempDir())
	require.NoError(t, err)

	stage := New(sandbox)

	// Cleanup should be a no-op
	err = stage.Cleanup(context.Background())
	require.NoError(t, err)
}

func TestArtifacts(t *testing.T) {
	tempDir := t.TempDir()
	sandbox, err := storage.NewSandbox(tempDir)
	require.NoError(t, err)

	stage := New(sandbox)

	// Create source files
	srcDir := filepath.Join(tempDir, "source")
	require.NoError(t, os.MkdirAll(srcDir, 0755))

	m3uPath := filepath.Join(srcDir, "test.m3u")
	require.NoError(t, os.WriteFile(m3uPath, []byte("#EXTM3U"), 0644))

	xmltvPath := filepath.Join(srcDir, "test.xml")
	require.NoError(t, os.WriteFile(xmltvPath, []byte("<tv/>"), 0644))

	outputDir := filepath.Join(tempDir, "output")
	require.NoError(t, os.MkdirAll(outputDir, 0755))

	proxy := &models.StreamProxy{}
	proxy.ID = models.NewULID()
	state := core.NewState(proxy)
	state.OutputDir = outputDir
	state.SetMetadata(generatem3u.MetadataKeyTempPath, m3uPath)
	state.SetMetadata(generatexmltv.MetadataKeyTempPath, xmltvPath)

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	// Verify artifacts
	require.Len(t, result.Artifacts, 2)

	// Check M3U artifact
	m3uArtifact := result.Artifacts[0]
	assert.Equal(t, core.ArtifactTypeM3U, m3uArtifact.Type)
	assert.Equal(t, core.ProcessingStagePublished, m3uArtifact.Stage)
	assert.Contains(t, m3uArtifact.FilePath, ".m3u")

	// Check XMLTV artifact
	xmltvArtifact := result.Artifacts[1]
	assert.Equal(t, core.ArtifactTypeXMLTV, xmltvArtifact.Type)
	assert.Equal(t, core.ProcessingStagePublished, xmltvArtifact.Stage)
	assert.Contains(t, xmltvArtifact.FilePath, ".xml")
}
