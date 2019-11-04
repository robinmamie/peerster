package files

import (
	"crypto/sha256"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/robinmamie/Peerster/tools"
)

// ChunkSize designates the size of a chunk in bytes.
const ChunkSize int = 1 << 13 // 8KB

// SHA256ByteSize designates the size of a SHA-256 hash in bytes.
const SHA256ByteSize int = 32

// FileMetadata describes all the necessary information about a file.
type FileMetadata struct {
	FileName string
	FileSize int
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
		FileSize: fileSize,
		MetaFile: metaFile,
		MetaHash: metaHash,
	}, chunks
}

func getFileData(name string) []byte {
	pathToExecutable, err := os.Executable()
	tools.Check(err)
	pathToFolder := filepath.Dir(pathToExecutable)
	// TODO We assume that the name complies with the specs (only file name).
	pathToFile := pathToFolder + "/_SharedFiles/" + name

	file, err := os.Open(pathToFile)
	tools.Check(err)
	fileBytes, err := ioutil.ReadAll(file)
	tools.Check(err)
	file.Close()
	return fileBytes
}

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
	//fmt.Println(tools.BytesToHexString(metaHash[:])) // Debug code to know hash
	return sums, metaHash[:], chunks
}

func getEndIndex(i int, fileSize int) int {
	endIndex := ChunkSize * (i + 1)
	// Check if chunk smaller than 8KB
	if endIndex > fileSize {
		// Not filesSize - 1, since [... : x] automatically infers "until, but not included, x"
		endIndex = fileSize
	}
	return endIndex
}

// ExtractCorrespondingData returns the corresponding data from the
// FileMetadata and the chunk number.
func (fileMeta FileMetadata) ExtractCorrespondingData(i int) []byte {
	// Import file
	file := getFileData(fileMeta.FileName)
	endIndex := getEndIndex(i, fileMeta.FileSize)
	return file[ChunkSize*i : endIndex]
}

// BuildFileFromChunks reconstructs a file using all data chunks, and returns the file size.
func BuildFileFromChunks(fileName string, chunks [][]byte) int {

	fileContents := make([]byte, 0)
	// TODO use metafile to iterate over hashes, and verify if contents are correct (2 loops)
	for _, chunk := range chunks {
		fileContents = append(fileContents, chunk...)
	}

	pathToExecutable, err := os.Executable()
	tools.Check(err)
	pathToFolder := filepath.Dir(pathToExecutable)
	// TODO We assume that the name complies with the specs (only file name).
	pathToFile := pathToFolder + "/_Downloads/" + fileName
	err = ioutil.WriteFile(pathToFile, fileContents, 0644)
	tools.Check(err)
	return len(fileContents)
}
