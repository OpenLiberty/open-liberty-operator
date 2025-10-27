package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

var latestImageState *LatestImagePullState
var latestImageLock *sync.Mutex

const stateFileName = "liberty/output/image.json"

type LatestImagePullState struct {
	LastPull int    `json:"lastPull"`
	Version  string `json:"version"`
}

func init() {
	latestImageLock = &sync.Mutex{}
	latestImageState = &LatestImagePullState{}
}

func (state *LatestImagePullState) GetLastPull() int {
	latestImageLock.Lock()
	defer latestImageLock.Unlock()
	return state.LastPull
}

func (state *LatestImagePullState) GetVersion() string {
	latestImageLock.Lock()
	defer latestImageLock.Unlock()
	return state.Version
}

func (state *LatestImagePullState) Max(otherState *LatestImagePullState) (*LatestImagePullState, bool) {
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

// Returns true if the latest image state was updated false otherwise
func SetLatestImageState(newLastPull int, newVersion string) bool {
	latestImageLock.Lock()
	defer latestImageLock.Unlock()
	if latestImageState == nil {
		latestImageState = &LatestImagePullState{}
	}
	var hasChanged bool = false
	latestImageState, hasChanged = latestImageState.Max(&LatestImagePullState{LastPull: newLastPull, Version: newVersion})
	if hasChanged {
		return writeStateToDisk(latestImageState) == nil
	}
	return false
}

// Returns a pointer to the latest image pull state stored in persistence or creates a new state
func GetLatestImageState() *LatestImagePullState {
	latestImageLock.Lock()
	defer latestImageLock.Unlock()
	state, err := readStateFromDisk()
	if err != nil {
		// silently error, persistence is not being used
	}
	if latestImageState == nil {
		latestImageState = &LatestImagePullState{}
	}
	if err == nil && state.LastPull > latestImageState.LastPull {
		latestImageState = state
	}
	return latestImageState
}

// Deserializes a byte array into an ImagePullState
func deserializeImagePullState(data []byte) (*LatestImagePullState, error) {
	state := &LatestImagePullState{}
	err := json.Unmarshal(data, state)
	if err != nil {
		return nil, err
	}
	return state, nil
}

// Serializes an ImagePullState into a byte array
func (s *LatestImagePullState) serialize() ([]byte, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("Could not serialize ImagePullState")
	}
	return data, nil
}

// Writes LatestImagePullState to disk (support for persistence)
func writeStateToDisk(state *LatestImagePullState) error {
	f, err := os.Create(stateFileName)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := latestImageState.serialize()
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err != nil {
		return err
	}
	return nil
}

// Reads LatestImagePullState from disk (support for persistence)
func readStateFromDisk() (*LatestImagePullState, error) {
	bytes, err := os.ReadFile(stateFileName)
	if err != nil {
		return nil, err
	}
	return deserializeImagePullState(bytes)
}
