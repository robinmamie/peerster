package files

import (
	"crypto/sha256"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/robinmamie/Peerster/tools"
)

// ChunkSize designates the size of a chunk in bytes.
const ChunkSize int = 1 << 13 // 8KB

// SHA256ByteSize designates the size of a SHA-256 hash in bytes.
const SHA256ByteSize int = 32

// FileMetadata describes all the necessary information about a file.
type FileMetadata struct {
	FileName string
	FileSize int64
	MetaFile []byte
	MetaHash []byte
}

// NewFileMetadata creates the metadata of a given file
func NewFileMetadata(name string) (*FileMetadata, [][]byte) {
	// Import file
	file := getFileData(name)

	// Compute metadata
	fileSize := len(file)
	metaFile, metaHash, chunks := createMetaFile(file, fileSize)

	return &FileMetadata{
		FileName: name,
		FileSize: (int64)(fileSize),
		MetaFile: metaFile,
		MetaHash: metaHash,
	}, chunks
}

// getFileData returns the data of the file using its name. This assumes that
// the specs are respected (in _SharedFiles, only file name given).
func getFileData(name string) []byte {
	pathToExecutable, err := os.Executable()
	tools.Check(err)
	pathToFolder := filepath.Dir(pathToExecutable)
	// We assume that the name complies with the specs (only file name).
	pathToFile := pathToFolder + "/_SharedFiles/" + name

	file, err := os.Open(pathToFile)
	tools.Check(err)
	fileBytes, err := ioutil.ReadAll(file)
	tools.Check(err)
	file.Close()
	return fileBytes
}

// createMetaFile creates the entire metafile using the file data and its size.
// Returns in order the meta-file, the meta-hash and the chunk data.
func createMetaFile(file []byte, fileSize int) ([]byte, []byte, [][]byte) {
	// Compute number of chunks
	chunkNumber := fileSize / ChunkSize
	// Have to add 1 unless the final chunk is exactly full
	if fileSize%ChunkSize != 0 {
		chunkNumber++
	}
	sums := make([]byte, 0)
	chunks := make([][]byte, 0)
	for i := 0; i < chunkNumber; i++ {
		endIndex := getEndIndex(i, fileSize)
		chunk := file[ChunkSize*i : endIndex]
		chunks = append(chunks, chunk)
		sum := sha256.Sum256(chunk)
		sums = append(sums, sum[:]...)
	}
	metaHash := sha256.Sum256(sums)
	return sums, metaHash[:], chunks
}

// getEndIndex returns the last index (used for slicing) of a chunk, according
// to the size of the file.
func getEndIndex(i int, fileSize int) int {
	endIndex := ChunkSize * (i + 1)
	// Check if chunk smaller than 8KB
	if endIndex > fileSize {
		// Not filesSize - 1, since [... : x] automatically infers "until, but not included, x"
		endIndex = fileSize
	}
	return endIndex
}

// BuildFileFromChunks reconstructs a file using all data chunks, and returns the file size.
func BuildFileFromChunks(fileName string, metaFileList []string, chunks *sync.Map) bool {

	fileContents := make([]byte, 0)
	for _, hash := range metaFileList {
		if chunk, ok := chunks.Load(hash); ok {
			fileContents = append(fileContents, chunk.([]byte)...)
		} else {
			return false
		}
	}

	pathToExecutable, err := os.Executable()
	tools.Check(err)
	pathToFolder := filepath.Dir(pathToExecutable)
	// We assume that the name complies with the specs (only file name).
	pathToFile := pathToFolder + "/_Downloads/" + fileName
	err = ioutil.WriteFile(pathToFile, fileContents, 0644)
	tools.Check(err)
	return true
}
