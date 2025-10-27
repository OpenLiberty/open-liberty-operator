package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

const imageStateFileName = "liberty/cache/imageState.json"

var imageState *ImageStateMaxHeap
var imageStateLock *sync.Mutex

type ImageState struct {
	LastPull  int    `json:"lastPull"`
	ImageName string `json:"imageName"`
	Version   string `json:"version"`
}

func init() {
	imageStateLock = &sync.Mutex{}
	imageState = nil
}

type ImageStateMaxHeap struct {
	Data  []*ImageState  `json:"data"`
	Index map[string]int `json:"index"`
}

func (mh *ImageStateMaxHeap) swapIndex(i, j int) {
	mh.Index[mh.Data[i].ImageName], mh.Index[mh.Data[j].ImageName] = j, i
}

func (mh *ImageStateMaxHeap) swapData(i, j int) {
	mh.Data[i], mh.Data[j] = mh.Data[j], mh.Data[i]
}

func (mh *ImageStateMaxHeap) swap(i, j int) {
	mh.swapIndex(i, j)
	mh.swapData(i, j)
}

func (mh *ImageStateMaxHeap) Heapify(i int) {
	for {
		if i >= len(mh.Data) {
			break
		}
		l := 2 * i
		r := 2*i + 1
		largest := i

		if l < len(mh.Data) && mh.Data[l].LastPull > mh.Data[i].LastPull {
			largest = l
		}
		if r < len(mh.Data) && mh.Data[r].LastPull > mh.Data[i].LastPull {
			largest = r
		}
		if i == largest {
			break
		}
		mh.swap(i, largest)
		i = largest
	}
}

func (mh *ImageStateMaxHeap) HeapifyUp(i int) {
	for {
		p := (i - 1) / 2
		if i == p || mh.Data[p].LastPull > mh.Data[i].LastPull {
			return
		}
		mh.swap(i, p)
		i = p
	}
}

func (mh *ImageStateMaxHeap) BuildHeap(data []*ImageState) {
	mh.Data = data
	mh.Index = map[string]int{}
	for i, image := range mh.Data {
		mh.Index[image.ImageName] = i
	}
	n := len(mh.Data)
	for i := n - 1; i >= 0; i-- {
		mh.Heapify(i)
	}
}

func (mh *ImageStateMaxHeap) Add(val *ImageState) bool {
	if val == nil || val.Version == "" || val.ImageName == "" {
		return false
	}
	foundIndex, found := mh.Index[val.ImageName]
	if found {
		// found the index, so modify in-place
		var hasChanged bool = false
		mh.Data[foundIndex], hasChanged = mh.Data[foundIndex].Max(val)
		return hasChanged
	}
	mh.Data = append(mh.Data, val)
	mh.Index[val.ImageName] = max(len(mh.Index)-1, 0)
	mh.HeapifyUp(len(mh.Data) - 1)
	return true
}

func (mh *ImageStateMaxHeap) Remove(imageName string) *ImageState {
	if len(mh.Data) == 0 {
		return nil
	}
	imageIndex, found := mh.Index[imageName]
	if !found {
		return nil
	}

	// swap the found index with the ending element
	end := len(mh.Data) - 1
	deletedElement := mh.Data[imageIndex]
	mh.swap(imageIndex, end)
	mh.Data = mh.Data[:end]
	delete(mh.Index, imageName)
	// re-sort the newly placed element
	if imageIndex < len(mh.Data) {
		mh.Heapify(imageIndex)
		mh.HeapifyUp(imageIndex)
	}
	return deletedElement
}

func (mh *ImageStateMaxHeap) Peek() *ImageState {
	if len(mh.Data) == 0 {
		return nil
	}
	return mh.Data[0]
}

func (mh *ImageStateMaxHeap) Get(imageName string) *ImageState {
	if len(mh.Data) == 0 {
		return nil
	}
	imageIndex, found := mh.Index[imageName]
	if !found || imageIndex < 0 {
		return nil
	}
	if imageIndex < len(mh.Data) {
		return mh.Data[imageIndex]
	}
	return nil
}

// Deserializes a byte array into an ImageStateMaxHeap
func deserializeImageStateMaxHeap(data []byte) (*ImageStateMaxHeap, error) {
	state := &ImageStateMaxHeap{}
	err := json.Unmarshal(data, state)
	if err != nil {
		return nil, err
	}
	return state, nil
}

// Serializes an ImageStateMaxHeap into a byte array
func (s *ImageStateMaxHeap) serialize() ([]byte, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("Could not serialize ImageStateMaxHeap")
	}
	return data, nil
}

// Writes ImageStateMaxHeap to disk (support for persistence)
func writeStateToDisk(state *ImageStateMaxHeap) error {
	f, err := os.Create(imageStateFileName)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := imageState.serialize()
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err != nil {
		return err
	}
	return nil
}

// Reads ImageStateMaxHeap from disk (support for persistence)
func readStateFromDisk() (*ImageStateMaxHeap, error) {
	bytes, err := os.ReadFile(imageStateFileName)
	if err != nil {
		return nil, err
	}
	return deserializeImageStateMaxHeap(bytes)
}

func (state *ImageState) GetLastPull() int {
	imageStateLock.Lock()
	defer imageStateLock.Unlock()
	return state.LastPull
}

func (state *ImageState) GetVersion() string {
	imageStateLock.Lock()
	defer imageStateLock.Unlock()
	return state.Version
}

// Precondiiton: state.ImageName == otherState.ImageName
func (state *ImageState) Max(otherState *ImageState) (*ImageState, bool) {
	if otherState == nil {
		return state, false
	}
	if state.LastPull >= otherState.LastPull {
		return state, false
	}
	if CompareLibertyVersion(state.Version, otherState.Version) >= 0 {
		return state, false
	}
	return otherState, true
}

func readOrCreateState() {
	if imageState == nil {
		state, err := readStateFromDisk()
		if err == nil {
			imageState = state
		}
	} else {
		imageState = &ImageStateMaxHeap{}
		imageState.BuildHeap([]*ImageState{})
	}
}

// Returns true if the latest image state was updated false otherwise
func SetImageState(newImageName string, newLastPull int, newVersion string) bool {
	imageStateLock.Lock()
	defer imageStateLock.Unlock()
	readOrCreateState()
	if imageState.Add(&ImageState{ImageName: newImageName, LastPull: newLastPull, Version: newVersion}) {
		return writeStateToDisk(imageState) == nil
	}
	return false
}

// Returns a pointer to the latest image pull state stored in persistence or creates a new state
func GetImageState(imageName string) *ImageState {
	imageStateLock.Lock()
	defer imageStateLock.Unlock()
	readOrCreateState()
	top := imageState.Get(imageName)
	if top != nil {
		return top
	}
	return &ImageState{}
}

// Calculates the time drift in seconds between when imageName was last pulled against the most recently pulled image
func GetImageDriftSeconds(imageName string) int {
	imageStateLock.Lock()
	defer imageStateLock.Unlock()
	if imageState == nil {
		state, err := readStateFromDisk()
		if err == nil {
			imageState = state
		}
	} else {
		imageState = &ImageStateMaxHeap{}
		imageState.BuildHeap([]*ImageState{})
	}
	latest := imageState.Get(imageName)
	if latest != nil {
		top := imageState.Peek()
		if top != nil && top != latest {
			return top.LastPull - latest.LastPull
		}
	}
	return 0
}
