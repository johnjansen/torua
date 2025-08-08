package storage

import (
	"errors"
	"sync"
)

// ErrKeyNotFound is returned when a key doesn't exist in the store
var ErrKeyNotFound = errors.New("key not found")

// Store defines the interface for key-value storage
// All implementations must be thread-safe for concurrent access
type Store interface {
	// Get retrieves a value by key
	// Returns ErrKeyNotFound if the key doesn't exist
	Get(key string) ([]byte, error)

	// Put stores a value with the given key
	// Overwrites any existing value for the key
	Put(key string, value []byte) error

	// Delete removes a key-value pair
	// No error if key doesn't exist
	Delete(key string) error

	// List returns all keys in the store
	// Order is not guaranteed
	List() []string

	// Stats returns storage statistics
	Stats() StoreStats
}

// StoreStats contains statistics about the store
type StoreStats struct {
	Keys  int // Number of keys
	Bytes int // Total size of all values in bytes
}

// MemoryStore implements Store interface with in-memory storage
// Uses sync.RWMutex for thread-safe concurrent access
type MemoryStore struct {
	mu   sync.RWMutex      // Protects concurrent access
	data map[string][]byte // Key-value storage
}

// NewMemoryStore creates a new in-memory store
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: make(map[string][]byte),
	}
}

// Get retrieves a value by key
// Returns a copy of the value to prevent external modification
func (m *MemoryStore) Get(key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	value, exists := m.data[key]
	if !exists {
		return nil, ErrKeyNotFound
	}

	// Return a copy to prevent external modification
	result := make([]byte, len(value))
	copy(result, value)
	return result, nil
}

// Put stores a value with the given key
// Makes a copy of the value to prevent external modification
func (m *MemoryStore) Put(key string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Make a copy to prevent external modification
	stored := make([]byte, len(value))
	copy(stored, value)
	m.data[key] = stored

	return nil
}

// Delete removes a key-value pair
// No error if key doesn't exist (idempotent)
func (m *MemoryStore) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.data, key)
	return nil
}

// List returns all keys in the store
// Returns a copy of the keys to prevent external modification
func (m *MemoryStore) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]string, 0, len(m.data))
	for key := range m.data {
		keys = append(keys, key)
	}
	return keys
}

// Stats returns storage statistics
func (m *MemoryStore) Stats() StoreStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	totalBytes := 0
	for _, value := range m.data {
		totalBytes += len(value)
	}

	return StoreStats{
		Keys:  len(m.data),
		Bytes: totalBytes,
	}
}
