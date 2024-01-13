package web

import (
	"strings"
	"sync"
)

type Forwards struct {
	sync.Mutex
	entries map[string]string
}

func NewForwards() *Forwards {
	return &Forwards{
		entries: make(map[string]string),
	}
}

func (h *Forwards) Set(key string, protocol string, srcPort int32, ipv4 string, dstPort int32) {
	h.Lock()
	defer h.Unlock()

	protocol = strings.ToLower(protocol)
	if protocol != "tcp" && protocol != "udp" {
		panic("protocol must be either tcp or udp")
	}
	h.entries[key] = protocol + " " + ipv4 + ":" + string(srcPort) + " " + string(dstPort)
}

func (h *Forwards) Delete(key string) {
	h.Lock()
	defer h.Unlock()

	delete(h.entries, key)
}

func (h *Forwards) render() string {
	h.Lock()
	defer h.Unlock()

	var forwards string
	for _, entry := range h.entries {
		forwards += entry + "\n"
	}
	return forwards
}
