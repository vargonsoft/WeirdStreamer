package handlers

import "sync"

// thumbnailHub fans thumbnail-ready events out to SSE clients, filtered by library access.
type thumbnailHub struct {
	mu      sync.RWMutex
	nextID  uint64
	clients map[uint64]*hubClient
}

type hubClient struct {
	group  string
	events chan string
}

func newThumbnailHub() *thumbnailHub {
	return &thumbnailHub{clients: make(map[uint64]*hubClient)}
}

// register adds a client for a user in group. IDs are never reused, so one client
// disconnecting can't remove another.
func (hub *thumbnailHub) register(group string) (uint64, <-chan string) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	hub.nextID++
	client := &hubClient{group: group, events: make(chan string, 32)}
	hub.clients[hub.nextID] = client
	return hub.nextID, client.events
}

func (hub *thumbnailHub) unregister(id uint64) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if client, ok := hub.clients[id]; ok {
		delete(hub.clients, id)
		close(client.events)
	}
}

// broadcast sends msg about filePath to every client whose group can access its library.
// Slow clients miss events rather than blocking the sender.
func (hub *thumbnailHub) broadcast(filePath, msg string) {
	lib, ok := libraryForPath(filePath)
	if !ok {
		return
	}
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	for _, client := range hub.clients {
		if !groupCanAccess(client.group, lib) {
			continue
		}
		select {
		case client.events <- msg:
		default:
		}
	}
}
