package files

import (
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/robinmamie/Peerster/tools"
)

// ChunkSize designates the size of a chunk in bytes.
const ChunkSize int = 1 << 13 // 8KB

// SHA256Size designates the size of a SHA-256 hash in bytes.
const SHA256Size int = 32

// FileMetadata describes all the necessary information about a file.
type FileMetadata struct {
	FileName string
	FileSize int
	MetaFile []byte
	MetaHash []byte
}

// NewFileMetadata creates the metadata of a given file
func NewFileMetadata(name string) *FileMetadata {
	// Import file
	file := getFileData(name)

	// Compute metadata
	fileSize := len(file)
	metaFile, metaHash := createMetaFile(file, fileSize)

	return &FileMetadata{
		FileName: name,
		FileSize: fileSize,
		MetaFile: metaFile,
		MetaHash: metaHash,
	}
}

func getFileData(name string) []byte {
	pathToExecutable, err := os.Executable()
	tools.Check(err)
	pathToFolder := filepath.Dir(pathToExecutable)
	pathToFile := pathToFolder + "/_SharedFiles/" + name
	fmt.Println(pathToFile)
	file, err := os.Open(pathToFile)
	tools.Check(err)
	fileBytes, err := ioutil.ReadAll(file)
	tools.Check(err)
	file.Close()
	return fileBytes
}

func createMetaFile(file []byte, fileSize int) ([]byte, []byte) {
	// Compute number of chunks
	chunks := fileSize / ChunkSize
	if fileSize%ChunkSize != 0 {
		chunks++
	}
	sums := make([]byte, 0)
	for i := 0; i < chunks; i++ {
		endIndex := getEndIndex(i, fileSize)
		sum := sha256.Sum256(file[ChunkSize*i : endIndex])
		//s := fmt.Sprintf("%x", sum)
		sums = append(sums, sum[:]...)
	}
	metaHash := sha256.Sum256(sums)
	return sums, metaHash[:]
}

func getEndIndex(i int, fileSize int) int {
	endIndex := ChunkSize * (i + 1)
	if endIndex > fileSize {
		endIndex = fileSize - 1
	}
	return endIndex
}

/*
TODO !!
Finally, for performance, you should consider storing the chunks of a file separately after the
file has been indexed and completely downloaded, so if some specific chunk is requested,
your gossiper does not have to reparse the whole file.
*/

// ExtractCorrespondingData returns the corresponding data from the
// FileMetadata and the chunk number.
func (fileMeta FileMetadata) ExtractCorrespondingData(i int) []byte {
	// Import file
	// TODO here, we should rather save the chunks somewhere, so that we do not
	// have to reparse everything
	file := getFileData(fileMeta.FileName)
	endIndex := getEndIndex(i, fileMeta.FileSize)
	return file[ChunkSize*i : endIndex]
}
