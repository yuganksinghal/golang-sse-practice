package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

// A Broker holds open client connections,
// listens for incoming events on its Notifier channel
// and broadcast event data to all registered connections
type Broker struct {

	// Events are pushed to this channel by the main events-gathering routine
	Notifier chan []byte

	// New client connections
	newClients chan chan []byte

	// Closed client connections
	closingClients chan chan []byte

	// Client coneections registry
	clients map[chan []byte]bool
}

func (broker *Broker) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// make sure the writer supports flushing.
	flusher, ok := rw.(http.Flusher)

	if !ok {
		http.Error(rw, "Streaming unsuported!", http.StatusInternalServerError)
		return
	}

	// event streaming headers
	rw.Header().Set("Content-Type", "text/event-stream")
	rw.Header().Set("Connection", "keep-alive")
	rw.Header().Set("Access-Control-Allow-Origin", "*") // Well this is more of a general Header, not related to streaming
	rw.Header().Set("Cache-Control", "no-cache")        // Well this is more of a general Header, not related to streaming

	// Each connection registers its own message channel with Broker's connections Registry (clients in the Broker Struct)
	messageChan := make(chan []byte)

	// Signal the broker that we have a new conection
	broker.newClients <- messageChan

	// Remove this client from the map of connected clients
	// when this handler exits
	defer func() {
		broker.closingClients <- messageChan
	}()

	// Listen to connection close and un-register messageChan
	notify := req.Context().Done()

	go func() {
		<-notify
		broker.closingClients <- messageChan
	}()

	// block waiting for messages broadcast on this coneection's messageChan
	for {
		// Write to the ResponseWriter
		// Server Sent Events compatible
		fmt.Fprintf(rw, "data: %s\n\n", <-messageChan)

		// Flush the data immediately instead of buffering it for later.
		flusher.Flush()
	}
}

func (broker *Broker) listen() {
	for {
		select {
		// New client connected
		// Register their message channel
		case s := <-broker.newClients:
			broker.clients[s] = true
			log.Printf("Client added, %d registered clients", len(broker.clients))

		// Closing Clients
		// stop sending them messages.
		case s := <-broker.closingClients:
			delete(broker.clients, s)
			log.Printf("removed client, %e registered clients", len(broker.clients))

		// new even from the outside
		// sned event to all connected clients
		case event := <-broker.Notifier:
			for clientMessageChan, _ := range broker.clients {
				clientMessageChan <- event
			}
		}
	}
}

// NewServer is a Broker factory
func NewServer() (broker *Broker) {
	// Instantiate a broker
	broker = &Broker{
		Notifier:       make(chan []byte, 1),
		newClients:     make(chan chan []byte),
		closingClients: make(chan chan []byte),
		clients:        make(map[chan []byte]bool),
	}

	// Set it running - listening and broadcasting events
	go broker.listen()

	return
}

func main() {
	broker := NewServer()

	go func() {
		for {
			time.Sleep(time.Second * 2)
			eventString := fmt.Sprintf("the time is %v", time.Now())
			log.Println("Receiving event")
			broker.Notifier <- []byte(eventString)
		}
	}()

	log.Fatal("HTTP server error: ", http.ListenAndServe("localhost:3000", broker))
}
