package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

const imageStateFileName = "liberty/cache/imageState.json"

var globalImageState *NamespaceImageStateMap
var globalImageStateLock *sync.Mutex

type ImageState struct {
	ImageLastPull  int    `json:"imageLastPull,omitempty"`
	LibertyVersion string `json:"libertyVersion,omitempty"`
}

func init() {
	globalImageStateLock = &sync.Mutex{}
	globalImageState = readOrCreateState()
}

type NamespaceImageStateMap struct {
	NamespaceImages map[string]*NamespaceImageState `json:"namespaceImages,omitempty"`
	LatestImage     *ImageState                     `json:"latestImage,omitempty"`
	ClusterLastPull int                             `json:"clusterLastPull"`
}

type NamespaceImageState struct {
	Images            map[string]*ImageState `json:"images,omitempty"` // maps image names to ImageStates
	NamespaceLastPull int                    `json:"namespaceLastPull,omitempty"`
}

// Deserializes a byte array into an NamespaceImageStateMap
func deserializeNamespaceImageStateMap(data []byte) (*NamespaceImageStateMap, error) {
	state := &NamespaceImageStateMap{}
	err := json.Unmarshal(data, state)
	if err != nil {
		return nil, err
	}
	return state, nil
}

func (s *NamespaceImageStateMap) SetLatestImage(lastPull int, version string) {
	if s.LatestImage == nil {
		s.LatestImage = &ImageState{}
	}
	if lastPull > s.LatestImage.ImageLastPull {
		s.LatestImage.ImageLastPull = lastPull
		s.LatestImage.LibertyVersion = version
	}
}

// Serializes an NamespaceImageStateMap into a byte array
func (s *NamespaceImageStateMap) serialize() ([]byte, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("Could not serialize NamespaceImageStateMap")
	}
	return data, nil
}

// Returns true and the last pull time if the image state has changed, false and -1 otherwise
func (s *NamespaceImageState) Set(imageName string, lastPull int, version string) (bool, int) {
	if s.Images == nil {
		s.Images = map[string]*ImageState{}
	}
	_, found := s.Images[imageName]
	if !found {
		s.Images[imageName] = &ImageState{ImageLastPull: lastPull, LibertyVersion: version}
		return true, lastPull
	} else {
		if s.Images[imageName].ImageLastPull != lastPull {
			s.Images[imageName].ImageLastPull = lastPull
			s.Images[imageName].LibertyVersion = version
			s.NamespaceLastPull = max(s.NamespaceLastPull, lastPull)
			return true, s.NamespaceLastPull
		}
	}
	return false, -1
}

func (s *NamespaceImageStateMap) Set(namespace string, imageName string, lastPull int, version string) bool {
	if s.NamespaceImages == nil {
		s.NamespaceImages = map[string]*NamespaceImageState{}
	}
	_, found := s.NamespaceImages[namespace]
	if !found {
		imageState := &NamespaceImageState{}
		imageState.Set(imageName, lastPull, version)
		s.NamespaceImages[namespace] = imageState
		return true
	}
	hasChanged, namespaceLastPull := s.NamespaceImages[namespace].Set(imageName, lastPull, version)
	if hasChanged {
		s.ClusterLastPull = max(s.ClusterLastPull, namespaceLastPull)
	}
	return hasChanged
}

func (s *NamespaceImageState) Get(imageName string) *ImageState {
	if s.Images == nil {
		return nil
	}
	val, found := s.Images[imageName]
	if !found {
		return &ImageState{}
	}
	return val
}

func (s *NamespaceImageStateMap) Get(namespace string) *NamespaceImageState {
	if s.NamespaceImages == nil {
		return nil
	}
	namespaceImageState, found := s.NamespaceImages[namespace]
	if !found {
		return &NamespaceImageState{}
	}
	return namespaceImageState
}

// Writes NamespaceImageStateMap to disk (support for persistence)
func writeStateToDisk(state *NamespaceImageStateMap) error {
	f, err := os.Create(imageStateFileName)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := globalImageState.serialize()
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err != nil {
		return err
	}
	return nil
}

// Reads NamespaceImageStateMap from disk (support for persistence)
func readStateFromDisk() (*NamespaceImageStateMap, error) {
	bytes, err := os.ReadFile(imageStateFileName)
	if err != nil {
		return nil, err
	}
	return deserializeNamespaceImageStateMap(bytes)
}

func (state *ImageState) GetImageLastPull() int {
	return state.ImageLastPull
}

func (state *ImageState) GetLibertyVersion() string {
	if state.LibertyVersion == "" {
		return NilLibertyVersion
	}
	return state.LibertyVersion
}

func (state *ImageState) GetImageDriftSeconds() int {
	return globalImageState.ClusterLastPull - state.ImageLastPull
}

func readOrCreateState() *NamespaceImageStateMap {
	if globalImageState == nil {
		state, err := readStateFromDisk()
		if err == nil {
			return state
		}
	}
	return &NamespaceImageStateMap{}
}

func SetLatestImageState(newLastPull int, newVersion string) error {
	globalImageStateLock.Lock()
	defer globalImageStateLock.Unlock()
	globalImageState.SetLatestImage(newLastPull, newVersion)
	globalImageState.ClusterLastPull = max(globalImageState.ClusterLastPull, newLastPull)
	return writeStateToDisk(globalImageState)
}

// Returns true if the image state was updated false otherwise
func SetImageState(namespace string, newImageName string, newLastPull int, newVersion string) bool {
	globalImageStateLock.Lock()
	defer globalImageStateLock.Unlock()
	if globalImageState.Set(namespace, newImageName, newLastPull, newVersion) {
		return writeStateToDisk(globalImageState) == nil
	}
	return false
}

func GetLatestImageState() *ImageState {
	globalImageStateLock.Lock()
	defer globalImageStateLock.Unlock()
	if globalImageState.LatestImage == nil {
		return &ImageState{}
	}
	return globalImageState.LatestImage
}

// Returns a pointer to the image state stored in persistence or creates a new state
func GetImageState(namespace string, imageName string) *ImageState {
	globalImageStateLock.Lock()
	defer globalImageStateLock.Unlock()
	img := globalImageState.Get(namespace).Get(imageName)
	if img != nil {
		return img
	}
	return &ImageState{}
}

func GetLatestImageLastPull() int {
	globalImageStateLock.Lock()
	defer globalImageStateLock.Unlock()
	if globalImageState.LatestImage == nil {
		globalImageState.LatestImage = &ImageState{}
	}
	return globalImageState.LatestImage.ImageLastPull
}

// Calculates the time drift in seconds between when imageName was last pulled against the most recently pulled image
func GetNamespaceLastPull(namespace string) int {
	globalImageStateLock.Lock()
	defer globalImageStateLock.Unlock()
	return globalImageState.Get(namespace).NamespaceLastPull
}
