package files

import (
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/robinmamie/Peerster/tools"
)

const chunkSize int = 1 << 13 // 8KB

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
	pathToExecutable, err := os.Executable()
	tools.Check(err)
	pathToFolder := filepath.Dir(pathToExecutable)
	pathToFile := pathToFolder + "/_SharedFiles/" + name
	fmt.Println(pathToFile)
	file, err := os.Open(pathToFile)
	tools.Check(err)

	// Compute metadata
	fileBytes, err := ioutil.ReadAll(file)
	tools.Check(err)
	fileSize := len(fileBytes)
	metaFile, metaHash := createMetaFile(fileBytes, fileSize)

	return &FileMetadata{
		FileName: name,
		FileSize: fileSize,
		MetaFile: metaFile,
		MetaHash: metaHash,
	}
}

func createMetaFile(file []byte, fileSize int) ([]byte, []byte) {
	// Compute number of chunks
	chunks := fileSize / chunkSize
	if fileSize%chunkSize != 0 {
		chunks++
	}
	sums := make([]byte, 0)
	for i := 0; i < chunks; i++ {
		endIndex := chunkSize * (i + 1)
		if endIndex > fileSize {
			endIndex = fileSize - 1
		}
		sum := sha256.Sum256(file[chunkSize*i : endIndex])
		//s := fmt.Sprintf("%x", sum)
		sums = append(sums, sum[:]...)
	}
	metaHash := sha256.Sum256(sums)
	return sums, metaHash[:]
}
