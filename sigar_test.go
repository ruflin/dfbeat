package main

import (
	"github.com/elastic/gosigar"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
	//"strconv"
	//"fmt"
)

func TestGetFileSystemStat(t *testing.T) {

	// Load sigar file system entries
	fslist := sigar.FileSystemList{}
	fslist.Get()
	fs := fslist.List

	// Check FileSystemStats object
	fsstat, err := GetFileSystemStat(fs[0])

	assert.NotNil(t, fsstat)
	assert.Nil(t, err)

	assert.True(t, len(fsstat.FileSystem) > 0)
	assert.True(t, fsstat.Size > 0)
	assert.True(t, fsstat.Available >= 0)
	assert.True(t, fsstat.Used >= 0)

	// Conversion to utf8 needed
	assert.Equal(t, "/", string([]rune(fsstat.Mounted)[0]))

	// Out is percentage
	assert.Equal(t, "%", string([]rune(fsstat.Use)[len(fsstat.Use)-1]))

	assert.True(t, time.Now().After(fsstat.ctime))

	// There are reserved blocks, so it can also be smaller
	assert.True(t, (fsstat.Available+fsstat.Used) <= fsstat.Size)
}

// checks that all objects are loaded
func TestGetFileSystemList(t *testing.T) {
	stats, err := GetFilesystemStatList()

	assert.NotNil(t, stats)
	assert.Nil(t, err)

	assert.True(t, len(stats) > 0)

	for _, fs := range stats {
		assert.True(t, time.Now().After(fs.ctime))
	}
}
